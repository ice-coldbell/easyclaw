package keeper

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sort"
	"strings"
	"time"

	lpvault "github.com/coldbell/dex/backend/internal/anchor/lp_vault"
	orderengine "github.com/coldbell/dex/backend/internal/anchor/order_engine"
	"github.com/coldbell/dex/backend/internal/config"
	"github.com/coldbell/dex/backend/internal/dex"
	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"
)

const (
	priceScale = uint64(1_000_000)
	bpsDenom   = uint64(10_000)
)

var (
	pythPushOracleProgramID     = solana.MustPublicKeyFromBase58("pythWSnswVUd12oZpeFP8e9CVaEqJg25g1Vtc2biRsT")
	priceUpdateV2Discriminator  = [8]byte{34, 241, 35, 99, 157, 126, 244, 205}
	cancelOrderByExecutorDisc   = anchorInstructionDiscriminator("cancel_order_by_executor")
	errInvalidOracle            = errors.New("invalid oracle price update account")
	errStaleOracle              = errors.New("stale oracle")
	errUnexpectedOracleEncoding = errors.New("unexpected oracle payload encoding")
)

var errSkipOrder = errors.New("skip order")

type Service struct {
	cfg    config.KeeperConfig
	rpc    *rpc.Client
	signer solana.PrivateKey
	logger *slog.Logger
}

type runtimeAccounts struct {
	engineConfigKey solana.PublicKey
	engineConfig    *orderengine.EngineConfig
	globalConfig    *orderengine.GlobalConfig
	keeperSet       *orderengine.KeeperSet
}

type openOrder struct {
	pubkey solana.PublicKey
	order  *orderengine.Order
}

type executionQuote struct {
	fillPrice         uint64
	oraclePrice       uint64
	oracleConf        uint64
	oraclePublishTime int64
}

type oracleSnapshot struct {
	feedID      [32]byte
	price       uint64
	conf        uint64
	publishTime int64
}

func New(cfg config.KeeperConfig, logger *slog.Logger) (*Service, error) {
	signer, err := solana.PrivateKeyFromSolanaKeygenFile(cfg.KeypairPath)
	if err != nil {
		return nil, fmt.Errorf("load keypair %q: %w", cfg.KeypairPath, err)
	}

	orderengine.ProgramID = cfg.OrderEngineProgramID
	lpvault.ProgramID = cfg.LpVaultProgramID

	return &Service{
		cfg:    cfg,
		rpc:    rpc.New(cfg.RPCURL),
		signer: signer,
		logger: logger,
	}, nil
}

func (s *Service) Run(ctx context.Context) error {
	s.logger.Info("keeper started",
		"rpc", s.cfg.RPCURL,
		"commitment", s.cfg.Commitment,
		"executor", s.signer.PublicKey(),
		"order_engine_program", s.cfg.OrderEngineProgramID,
	)

	if err := s.tick(ctx); err != nil {
		s.logger.Error("keeper tick failed", "err", err)
	}

	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("keeper stopped")
			return nil
		case <-ticker.C:
			if err := s.tick(ctx); err != nil {
				s.logger.Error("keeper tick failed", "err", err)
			}
		}
	}
}

func (s *Service) tick(ctx context.Context) error {
	runtimeAccounts, err := s.loadRuntimeAccounts(ctx)
	if err != nil {
		return err
	}

	orders, err := s.fetchOpenOrders(ctx)
	if err != nil {
		return err
	}
	if len(orders) == 0 {
		return nil
	}

	now := s.getClusterUnixTime(ctx)
	sort.Slice(orders, func(i, j int) bool {
		iExpired := orders[i].order.ExpiresAt <= now
		jExpired := orders[j].order.ExpiresAt <= now
		if iExpired != jExpired {
			return !iExpired
		}
		return orders[i].order.CreatedAt < orders[j].order.CreatedAt
	})

	limit := s.cfg.MaxOrdersPerTick
	if limit > len(orders) {
		limit = len(orders)
	}

	executed := 0
	cancelled := 0
	skipped := 0
	for idx := 0; idx < limit; idx++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		candidate := orders[idx]
		if candidate.order.ExpiresAt <= now {
			expiredReason := fmt.Errorf("%w: order expired at %d", errSkipOrder, candidate.order.ExpiresAt)
			if cancelErr := s.cancelOrderByExecutor(ctx, runtimeAccounts, candidate, expiredReason); cancelErr != nil {
				skipped++
				s.logger.Warn("expired order cancel failed", "order", candidate.pubkey, "err", cancelErr)
				continue
			}
			cancelled++
			continue
		}

		err := s.processOrder(ctx, runtimeAccounts, candidate)
		if err == nil {
			executed++
			continue
		}
		if errors.Is(err, errSkipOrder) {
			skipped++
			s.logger.Warn("order skipped", "order", candidate.pubkey, "reason", err)
			continue
		} else {
			s.logger.Warn("order processing failed", "order", candidate.pubkey, "err", err)
		}

		if cancelErr := s.cancelOrderByExecutor(ctx, runtimeAccounts, candidate, err); cancelErr != nil {
			s.logger.Warn("order cancel-on-failure failed", "order", candidate.pubkey, "err", cancelErr)
			continue
		}
		cancelled++
	}

	s.logger.Info(
		"keeper tick complete",
		"open_orders",
		len(orders),
		"attempted",
		limit,
		"executed",
		executed,
		"skipped",
		skipped,
		"cancelled",
		cancelled,
	)
	return nil
}

func (s *Service) cancelOrderByExecutor(
	ctx context.Context,
	runtime *runtimeAccounts,
	candidate openOrder,
	cause error,
) error {
	marketKey, _, err := dex.DeriveMarketPDA(runtime.engineConfig.RegistryProgram, candidate.order.MarketId)
	if err != nil {
		return fmt.Errorf("derive market PDA for cancel: %w", err)
	}

	cancelIx := newCancelOrderByExecutorInstruction(
		s.cfg.OrderEngineProgramID,
		s.signer.PublicKey(),
		runtime.engineConfigKey,
		runtime.engineConfig.RegistryProgram,
		runtime.engineConfig.RegistryGlobalConfig,
		runtime.engineConfig.KeeperSet,
		marketKey,
		candidate.order.UserMargin,
		candidate.pubkey,
	)

	instructions := make([]solana.Instruction, 0, 3)
	if s.cfg.ComputeUnitLimit > 0 {
		cuLimitIx, buildErr := computebudget.NewSetComputeUnitLimitInstruction(s.cfg.ComputeUnitLimit).ValidateAndBuild()
		if buildErr != nil {
			return fmt.Errorf("build compute unit limit instruction for cancel: %w", buildErr)
		}
		instructions = append(instructions, cuLimitIx)
	}
	if s.cfg.ComputeUnitPriceMicroLamports > 0 {
		cuPriceIx, buildErr := computebudget.NewSetComputeUnitPriceInstruction(s.cfg.ComputeUnitPriceMicroLamports).ValidateAndBuild()
		if buildErr != nil {
			return fmt.Errorf("build compute unit price instruction for cancel: %w", buildErr)
		}
		instructions = append(instructions, cuPriceIx)
	}
	instructions = append(instructions, cancelIx)

	txCtx, cancel := context.WithTimeout(ctx, s.cfg.TxTimeout)
	defer cancel()

	signature, err := s.sendTransaction(txCtx, instructions)
	if err != nil {
		return fmt.Errorf("send cancel_order_by_executor transaction: %w", err)
	}
	if err := s.waitForConfirmation(txCtx, signature); err != nil {
		return fmt.Errorf("confirm cancel_order_by_executor %s: %w", signature, err)
	}

	s.logger.Info(
		"order cancelled by executor",
		"order",
		candidate.pubkey,
		"market_id",
		candidate.order.MarketId,
		"reason",
		cause,
		"signature",
		signature,
	)

	return nil
}

func (s *Service) loadRuntimeAccounts(ctx context.Context) (*runtimeAccounts, error) {
	engineConfigKey, _, err := dex.DeriveEngineConfigPDA(s.cfg.OrderEngineProgramID)
	if err != nil {
		return nil, fmt.Errorf("derive engine config PDA: %w", err)
	}

	engineConfigResp, err := s.rpc.GetAccountInfoWithOpts(ctx, engineConfigKey, &rpc.GetAccountInfoOpts{Commitment: s.cfg.Commitment})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, fmt.Errorf("fetch engine config %s: %w (hint: check ORDER_ENGINE_PROGRAM_ID=%s and run bootstrap)", engineConfigKey, err, s.cfg.OrderEngineProgramID)
		}
		return nil, fmt.Errorf("fetch engine config %s: %w", engineConfigKey, err)
	}
	if engineConfigResp == nil || engineConfigResp.Value == nil {
		return nil, fmt.Errorf("engine config account %s not found (order_engine_program=%s, hint: deploy+bootstrap contracts on current RPC)", engineConfigKey, s.cfg.OrderEngineProgramID)
	}
	engineConfig, err := orderengine.ParseAccount_EngineConfig(engineConfigResp.Value.Data.GetBinary())
	if err != nil {
		return nil, fmt.Errorf("decode engine config %s: %w", engineConfigKey, err)
	}

	multiple, err := s.rpc.GetMultipleAccountsWithOpts(ctx,
		[]solana.PublicKey{engineConfig.RegistryGlobalConfig, engineConfig.KeeperSet},
		&rpc.GetMultipleAccountsOpts{Commitment: s.cfg.Commitment},
	)
	if err != nil {
		return nil, fmt.Errorf("fetch registry runtime accounts: %w", err)
	}
	if len(multiple.Value) != 2 || multiple.Value[0] == nil || multiple.Value[1] == nil {
		return nil, fmt.Errorf("registry runtime accounts missing")
	}

	globalConfig, err := orderengine.ParseAccount_GlobalConfig(multiple.Value[0].Data.GetBinary())
	if err != nil {
		return nil, fmt.Errorf("decode global config %s: %w", engineConfig.RegistryGlobalConfig, err)
	}
	keeperSet, err := orderengine.ParseAccount_KeeperSet(multiple.Value[1].Data.GetBinary())
	if err != nil {
		return nil, fmt.Errorf("decode keeper set %s: %w", engineConfig.KeeperSet, err)
	}

	executor := s.signer.PublicKey()
	if !executor.Equals(globalConfig.Multisig) && !containsPubkey(keeperSet.Keepers, executor) {
		return nil, fmt.Errorf("executor %s is not allowed in keeper set and not multisig", executor)
	}

	return &runtimeAccounts{
		engineConfigKey: engineConfigKey,
		engineConfig:    engineConfig,
		globalConfig:    globalConfig,
		keeperSet:       keeperSet,
	}, nil
}

func (s *Service) fetchOpenOrders(ctx context.Context) ([]openOrder, error) {
	orders, err := s.rpc.GetProgramAccountsWithOpts(ctx, s.cfg.OrderEngineProgramID, &rpc.GetProgramAccountsOpts{
		Commitment: s.cfg.Commitment,
		Filters: []rpc.RPCFilter{
			{Memcmp: &rpc.RPCFilterMemcmp{Offset: 0, Bytes: solana.Base58(orderengine.Account_Order[:])}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("getProgramAccounts orders: %w", err)
	}

	openOrders := make([]openOrder, 0, len(orders))
	for _, item := range orders {
		if item == nil || item.Account == nil {
			continue
		}
		order, err := orderengine.ParseAccount_Order(item.Account.Data.GetBinary())
		if err != nil {
			s.logger.Warn("failed to parse order account", "pubkey", item.Pubkey, "err", err)
			continue
		}
		if order.Status != orderengine.OrderStatus_Open {
			continue
		}
		openOrders = append(openOrders, openOrder{pubkey: item.Pubkey, order: order})
	}

	return openOrders, nil
}

func (s *Service) processOrder(ctx context.Context, runtime *runtimeAccounts, candidate openOrder) error {
	now := s.getClusterUnixTime(ctx)
	if candidate.order.ExpiresAt <= now {
		return fmt.Errorf("%w: order expired at %d", errSkipOrder, candidate.order.ExpiresAt)
	}

	marketKey, _, err := dex.DeriveMarketPDA(runtime.engineConfig.RegistryProgram, candidate.order.MarketId)
	if err != nil {
		return fmt.Errorf("derive market PDA: %w", err)
	}
	fundingKey, _, err := dex.DeriveFundingPDA(s.cfg.OrderEngineProgramID, candidate.order.MarketId)
	if err != nil {
		return fmt.Errorf("derive funding PDA: %w", err)
	}
	userPositionKey, _, err := dex.DeriveUserMarketPositionPDA(s.cfg.OrderEngineProgramID, candidate.order.UserMargin, candidate.order.MarketId)
	if err != nil {
		return fmt.Errorf("derive user market position PDA: %w", err)
	}
	keeperRebateKey, _, err := dex.DeriveKeeperRebatePDA(runtime.engineConfig.LpVaultProgram, runtime.engineConfig.LpPool, s.signer.PublicKey())
	if err != nil {
		return fmt.Errorf("derive keeper rebate PDA: %w", err)
	}

	oracleByMarket, oracleFallbackExists := s.cfg.OracleByMarket[candidate.order.MarketId]
	oraclePriceUpdateKey, hasOracleAccount := s.cfg.PythPriceUpdateAccountByMarket[candidate.order.MarketId]
	effectiveOraclePriceUpdateKey := oraclePriceUpdateKey
	if !hasOracleAccount && !oracleFallbackExists {
		return fmt.Errorf("%w: missing oracle price update account for market %d", errSkipOrder, candidate.order.MarketId)
	}
	if !hasOracleAccount {
		effectiveOraclePriceUpdateKey = solana.SystemProgramID
	}

	accountKeys := []solana.PublicKey{
		marketKey,
		fundingKey,
		candidate.order.UserMargin,
		userPositionKey,
		keeperRebateKey,
		oraclePriceUpdateKey,
	}
	fetched, err := s.rpc.GetMultipleAccountsWithOpts(ctx, accountKeys, &rpc.GetMultipleAccountsOpts{Commitment: s.cfg.Commitment})
	if err != nil {
		return fmt.Errorf("fetch order runtime accounts: %w", err)
	}
	if len(fetched.Value) != len(accountKeys) {
		return fmt.Errorf("unexpected account count for order %s", candidate.pubkey)
	}
	for i, acc := range fetched.Value {
		if acc == nil {
			if i == 5 && oracleFallbackExists {
				continue
			}
			return fmt.Errorf("%w: missing required account %s", errSkipOrder, accountKeys[i])
		}
	}

	market, err := orderengine.ParseAccount_Market(fetched.Value[0].Data.GetBinary())
	if err != nil {
		return fmt.Errorf("decode market %s: %w", marketKey, err)
	}
	funding, err := orderengine.ParseAccount_MarketFundingState(fetched.Value[1].Data.GetBinary())
	if err != nil {
		return fmt.Errorf("decode funding state %s: %w", fundingKey, err)
	}
	margin, err := orderengine.ParseAccount_UserMargin(fetched.Value[2].Data.GetBinary())
	if err != nil {
		return fmt.Errorf("decode margin %s: %w", candidate.order.UserMargin, err)
	}
	if !margin.Owner.Equals(candidate.order.User) {
		return fmt.Errorf("%w: margin owner mismatch", errSkipOrder)
	}

	if _, err := orderengine.ParseAccount_UserMarketPosition(fetched.Value[3].Data.GetBinary()); err != nil {
		return fmt.Errorf("decode user position %s: %w", userPositionKey, err)
	}
	if _, err := orderengine.ParseAccount_KeeperRebate(fetched.Value[4].Data.GetBinary()); err != nil {
		return fmt.Errorf("%w: keeper rebate account %s missing or invalid: %v", errSkipOrder, keeperRebateKey, err)
	}
	oracle, err := func() (*oracleSnapshot, error) {
		account := fetched.Value[5]
		if account == nil {
			if !oracleFallbackExists {
				return nil, fmt.Errorf("%w: missing oracle price update account for market %d", errSkipOrder, candidate.order.MarketId)
			}
			effectiveOraclePriceUpdateKey = solana.SystemProgramID
			conf, err := fallbackOracleConfForMarket(oracleByMarket, now)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", errSkipOrder, err)
			}
			return &oracleSnapshot{
				feedID:      [32]byte{},
				price:       oracleByMarket.Price,
				conf:        conf,
				publishTime: fallbackPublishTime(oracleByMarket, now),
			}, nil
		}

		decoded, decodeErr := decodePythPriceUpdateAccount(account, now)
		if decodeErr != nil {
			if !oracleFallbackExists {
				return nil, fmt.Errorf("%w: %v", errSkipOrder, decodeErr)
			}
			effectiveOraclePriceUpdateKey = solana.SystemProgramID
			conf, confErr := fallbackOracleConfForMarket(oracleByMarket, now)
			if confErr != nil {
				return nil, fmt.Errorf("%w: %v", errSkipOrder, confErr)
			}
			return &oracleSnapshot{
				feedID:      [32]byte{},
				price:       oracleByMarket.Price,
				conf:        conf,
				publishTime: fallbackPublishTime(oracleByMarket, now),
			}, nil
		}
		return decoded, nil
	}()
	if err != nil {
		return err
	}
	if oracle == nil {
		return fmt.Errorf("%w: no oracle snapshot for market %d", errSkipOrder, candidate.order.MarketId)
	}

	if oracleFallbackExists {
		oracle.feedID = market.PythFeed
	}

	if !bytes.Equal(oracle.feedID[:], market.PythFeed[:]) {
		return fmt.Errorf("%w: oracle feed id mismatch for market %d", errSkipOrder, candidate.order.MarketId)
	}
	maxStaleness := int64(market.PricingParams.MaxOracleStalenessSec)
	if maxStaleness < 0 || now < oracle.publishTime || now-oracle.publishTime > maxStaleness {
		return fmt.Errorf("%w: %w", errSkipOrder, errStaleOracle)
	}

	quote, err := s.buildExecutionQuote(candidate.order, market, funding, oracle)
	if err != nil {
		return err
	}

	executeIx, err := orderengine.NewExecuteOrderInstruction(
		quote.fillPrice,
		quote.oraclePrice,
		quote.oracleConf,
		quote.oraclePublishTime,
		s.signer.PublicKey(),
		runtime.engineConfigKey,
		runtime.engineConfig.RegistryProgram,
		runtime.engineConfig.RegistryGlobalConfig,
		runtime.engineConfig.KeeperSet,
		candidate.pubkey,
		marketKey,
		effectiveOraclePriceUpdateKey,
		fundingKey,
		candidate.order.UserMargin,
		userPositionKey,
		runtime.engineConfig.EngineAuthority,
		runtime.engineConfig.CollateralVault,
		runtime.engineConfig.LpVaultProgram,
		runtime.engineConfig.LpPool,
		runtime.engineConfig.LpLiquidityVault,
		runtime.engineConfig.LpInsuranceVault,
		runtime.engineConfig.LpProtocolFeeVault,
		keeperRebateKey,
		solana.TokenProgramID,
	)
	if err != nil {
		return fmt.Errorf("build execute_order instruction: %w", err)
	}

	instructions := make([]solana.Instruction, 0, 3)
	if s.cfg.ComputeUnitLimit > 0 {
		cuLimitIx, err := computebudget.NewSetComputeUnitLimitInstruction(s.cfg.ComputeUnitLimit).ValidateAndBuild()
		if err != nil {
			return fmt.Errorf("build compute unit limit instruction: %w", err)
		}
		instructions = append(instructions, cuLimitIx)
	}
	if s.cfg.ComputeUnitPriceMicroLamports > 0 {
		cuPriceIx, err := computebudget.NewSetComputeUnitPriceInstruction(s.cfg.ComputeUnitPriceMicroLamports).ValidateAndBuild()
		if err != nil {
			return fmt.Errorf("build compute unit price instruction: %w", err)
		}
		instructions = append(instructions, cuPriceIx)
	}
	instructions = append(instructions, executeIx)

	txCtx, cancel := context.WithTimeout(ctx, s.cfg.TxTimeout)
	defer cancel()

	signature, err := s.sendTransaction(txCtx, instructions)
	if err != nil {
		return fmt.Errorf("send transaction: %w", err)
	}
	if err := s.waitForConfirmation(txCtx, signature); err != nil {
		return fmt.Errorf("wait confirmation %s: %w", signature, err)
	}

	s.logger.Info("order executed",
		"order", candidate.pubkey,
		"market_id", candidate.order.MarketId,
		"side", candidate.order.Side.String(),
		"margin", candidate.order.Margin,
		"fill_price", quote.fillPrice,
		"oracle_price", quote.oraclePrice,
		"signature", signature,
	)

	return nil
}

func (s *Service) getClusterUnixTime(ctx context.Context) int64 {
	slot, err := s.rpc.GetSlot(ctx, s.cfg.Commitment)
	if err != nil {
		s.logger.Warn("using local clock because getSlot failed", "err", err)
		return time.Now().Unix()
	}

	blockTime, err := s.rpc.GetBlockTime(ctx, slot)
	if err != nil || blockTime == nil {
		s.logger.Warn("using local clock because getBlockTime unavailable", "slot", slot, "err", err)
		return time.Now().Unix()
	}

	return int64(*blockTime)
}

func fallbackOracleConfForMarket(cfg config.KeeperOracleConfig, now int64) (uint64, error) {
	if cfg.Price == 0 {
		return 0, fmt.Errorf("fallback oracle price is zero for market")
	}
	if cfg.ConfBps > bpsDenom {
		return 0, fmt.Errorf("fallback oracle conf bps exceeds basis-point max")
	}
	conf, err := mulDivFloor(cfg.ConfBps, cfg.Price, bpsDenom)
	if err != nil {
		return 0, err
	}
	if conf == 0 && cfg.ConfBps > 0 {
		return 1, nil
	}
	if cfg.PublishLagSec <= 0 {
		return conf, nil
	}
	return conf, nil
}

func fallbackPublishTime(cfg config.KeeperOracleConfig, now int64) int64 {
	publishTime := now - cfg.PublishLagSec
	if publishTime < 0 {
		return 0
	}
	return publishTime
}

func (s *Service) buildExecutionQuote(order *orderengine.Order, market *orderengine.Market, funding *orderengine.MarketFundingState, oracle *oracleSnapshot) (*executionQuote, error) {
	if oracle.price == 0 {
		return nil, fmt.Errorf("%w: oracle price is zero for market %d", errSkipOrder, order.MarketId)
	}

	maxConfBps := uint64(market.PricingParams.MaxConfBps)
	if maxConfBps == 0 {
		return nil, fmt.Errorf("%w: market %d max_conf_bps is zero", errSkipOrder, order.MarketId)
	}
	confBps, err := mulDivFloor(oracle.conf, bpsDenom, oracle.price)
	if err != nil {
		return nil, fmt.Errorf("%w: oracle conf calc failed for market %d: %v", errSkipOrder, order.MarketId, err)
	}
	if confBps > maxConfBps {
		return nil, fmt.Errorf("%w: oracle confidence too wide for market %d (conf_bps=%d, max=%d)", errSkipOrder, order.MarketId, confBps, maxConfBps)
	}

	fillPrice, err := calculateFillPrice(order, market, funding, oracle.price)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errSkipOrder, err)
	}

	return &executionQuote{
		fillPrice:         fillPrice,
		oraclePrice:       oracle.price,
		oracleConf:        oracle.conf,
		oraclePublishTime: oracle.publishTime,
	}, nil
}

func decodePythPriceUpdateAccount(account *rpc.Account, now int64) (*oracleSnapshot, error) {
	if account == nil {
		return nil, errInvalidOracle
	}
	if !account.Owner.Equals(pythPushOracleProgramID) {
		return nil, fmt.Errorf("%w: owner mismatch (%s)", errInvalidOracle, account.Owner)
	}

	data := account.Data.GetBinary()
	if len(data) < len(priceUpdateV2Discriminator) {
		return nil, fmt.Errorf("%w: payload too short", errInvalidOracle)
	}
	if !bytes.Equal(data[:8], priceUpdateV2Discriminator[:]) {
		return nil, fmt.Errorf("%w: discriminator mismatch", errInvalidOracle)
	}

	offset := 8
	if len(data) < offset+32 {
		return nil, fmt.Errorf("%w: missing write authority", errInvalidOracle)
	}
	offset += 32 // write_authority

	if len(data) < offset+1 {
		return nil, fmt.Errorf("%w: missing verification level", errInvalidOracle)
	}
	verificationVariant := data[offset]
	offset++
	switch verificationVariant {
	case 1: // Full
		// no payload
	case 0: // Partial { num_signatures: u8 }
		if len(data) < offset+1 {
			return nil, fmt.Errorf("%w: missing partial signature count", errInvalidOracle)
		}
		return nil, fmt.Errorf("%w: verification level is partial", errInvalidOracle)
	default:
		return nil, fmt.Errorf("%w: unknown verification level %d", errInvalidOracle, verificationVariant)
	}

	feedID, offset, err := readFixed32(data, offset)
	if err != nil {
		return nil, err
	}
	price, offset, err := readI64(data, offset)
	if err != nil {
		return nil, err
	}
	conf, offset, err := readU64(data, offset)
	if err != nil {
		return nil, err
	}
	exponent, offset, err := readI32(data, offset)
	if err != nil {
		return nil, err
	}
	publishTime, offset, err := readI64(data, offset)
	if err != nil {
		return nil, err
	}
	_, offset, err = readI64(data, offset) // prev_publish_time
	if err != nil {
		return nil, err
	}
	_, offset, err = readI64(data, offset) // ema_price
	if err != nil {
		return nil, err
	}
	_, offset, err = readU64(data, offset) // ema_conf
	if err != nil {
		return nil, err
	}
	_, offset, err = readU64(data, offset) // posted_slot
	if err != nil {
		return nil, err
	}
	if offset != len(data) {
		return nil, fmt.Errorf("%w: trailing bytes in payload", errUnexpectedOracleEncoding)
	}

	enginePrice, err := scaleSignedPriceToEngine(price, exponent)
	if err != nil {
		return nil, err
	}
	engineConf, err := scaleConfidenceToEngine(conf, exponent)
	if err != nil {
		return nil, err
	}
	if publishTime < 0 || publishTime > now {
		return nil, fmt.Errorf("%w: invalid publish time %d", errInvalidOracle, publishTime)
	}

	return &oracleSnapshot{
		feedID:      feedID,
		price:       enginePrice,
		conf:        engineConf,
		publishTime: publishTime,
	}, nil
}

func readFixed32(data []byte, offset int) ([32]byte, int, error) {
	if len(data) < offset+32 {
		return [32]byte{}, offset, fmt.Errorf("%w: truncated feed id", errInvalidOracle)
	}
	var out [32]byte
	copy(out[:], data[offset:offset+32])
	return out, offset + 32, nil
}

func readU64(data []byte, offset int) (uint64, int, error) {
	if len(data) < offset+8 {
		return 0, offset, fmt.Errorf("%w: truncated u64 field", errInvalidOracle)
	}
	value := binary.LittleEndian.Uint64(data[offset : offset+8])
	return value, offset + 8, nil
}

func readI64(data []byte, offset int) (int64, int, error) {
	u, next, err := readU64(data, offset)
	if err != nil {
		return 0, offset, err
	}
	return int64(u), next, nil
}

func readI32(data []byte, offset int) (int32, int, error) {
	if len(data) < offset+4 {
		return 0, offset, fmt.Errorf("%w: truncated i32 field", errInvalidOracle)
	}
	value := binary.LittleEndian.Uint32(data[offset : offset+4])
	return int32(value), offset + 4, nil
}

func scaleSignedPriceToEngine(price int64, exponent int32) (uint64, error) {
	if price <= 0 {
		return 0, fmt.Errorf("%w: non-positive oracle price", errInvalidOracle)
	}
	base := new(big.Int).SetInt64(price)
	scaled, err := scaleUnsignedToEngine(base, exponent, false)
	if err != nil {
		return 0, err
	}
	if scaled.Sign() <= 0 || !scaled.IsUint64() {
		return 0, fmt.Errorf("%w: scaled oracle price overflow", errInvalidOracle)
	}
	return scaled.Uint64(), nil
}

func scaleConfidenceToEngine(conf uint64, exponent int32) (uint64, error) {
	base := new(big.Int).SetUint64(conf)
	scaled, err := scaleUnsignedToEngine(base, exponent, true)
	if err != nil {
		return 0, err
	}
	if scaled.Sign() < 0 || !scaled.IsUint64() {
		return 0, fmt.Errorf("%w: scaled oracle confidence overflow", errInvalidOracle)
	}
	return scaled.Uint64(), nil
}

func scaleUnsignedToEngine(value *big.Int, exponent int32, ceil bool) (*big.Int, error) {
	// Match on-chain guardrail (`checked_pow` on u128) and avoid unbounded big.Int growth.
	if exponent > 38 || exponent < -38 {
		return nil, fmt.Errorf("%w: unsupported oracle exponent %d", errInvalidOracle, exponent)
	}
	tenPow := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(absInt32(exponent))), nil)
	priceScaleBig := new(big.Int).SetUint64(priceScale)

	if exponent >= 0 {
		out := new(big.Int).Mul(value, tenPow)
		out.Mul(out, priceScaleBig)
		return out, nil
	}

	numerator := new(big.Int).Mul(value, priceScaleBig)
	if ceil {
		numerator.Add(numerator, new(big.Int).Sub(tenPow, big.NewInt(1)))
	}
	out := new(big.Int).Div(numerator, tenPow)
	return out, nil
}

func absInt32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func (s *Service) sendTransaction(ctx context.Context, instructions []solana.Instruction) (solana.Signature, error) {
	recent, err := s.rpc.GetLatestBlockhash(ctx, s.cfg.Commitment)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("get latest blockhash: %w", err)
	}

	tx, err := solana.NewTransaction(
		instructions,
		recent.Value.Blockhash,
		solana.TransactionPayer(s.signer.PublicKey()),
	)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("build transaction: %w", err)
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if s.signer.PublicKey().Equals(key) {
			return &s.signer
		}
		return nil
	})
	if err != nil {
		return solana.Signature{}, fmt.Errorf("sign transaction: %w", err)
	}

	opts := rpc.TransactionOpts{
		SkipPreflight:       s.cfg.SkipPreflight,
		PreflightCommitment: s.cfg.Commitment,
	}
	if s.cfg.MaxRetries != nil {
		retries := *s.cfg.MaxRetries
		opts.MaxRetries = &retries
	}

	sig, err := s.rpc.SendTransactionWithOpts(ctx, tx, opts)
	if err != nil {
		return solana.Signature{}, err
	}
	return sig, nil
}

func newCancelOrderByExecutorInstruction(
	programID solana.PublicKey,
	executor solana.PublicKey,
	engineConfig solana.PublicKey,
	marketRegistryProgram solana.PublicKey,
	globalConfig solana.PublicKey,
	keeperSet solana.PublicKey,
	market solana.PublicKey,
	userMargin solana.PublicKey,
	order solana.PublicKey,
) solana.Instruction {
	data := make([]byte, len(cancelOrderByExecutorDisc))
	copy(data, cancelOrderByExecutorDisc[:])

	accounts := solana.AccountMetaSlice{
		solana.NewAccountMeta(executor, false, true),
		solana.NewAccountMeta(engineConfig, false, false),
		solana.NewAccountMeta(marketRegistryProgram, false, false),
		solana.NewAccountMeta(globalConfig, false, false),
		solana.NewAccountMeta(keeperSet, false, false),
		solana.NewAccountMeta(market, false, false),
		solana.NewAccountMeta(userMargin, true, false),
		solana.NewAccountMeta(order, true, false),
	}

	return solana.NewInstruction(programID, accounts, data)
}

func anchorInstructionDiscriminator(ixName string) [8]byte {
	hash := sha256.Sum256([]byte("global:" + ixName))
	var out [8]byte
	copy(out[:], hash[:8])
	return out
}

func (s *Service) waitForConfirmation(ctx context.Context, sig solana.Signature) error {
	ticker := time.NewTicker(700 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			result, err := s.rpc.GetSignatureStatuses(ctx, true, sig)
			if err != nil {
				continue
			}
			if len(result.Value) == 0 || result.Value[0] == nil {
				continue
			}
			status := result.Value[0]
			if status.Err != nil {
				return fmt.Errorf("transaction failed: %v", status.Err)
			}
			if status.ConfirmationStatus == rpc.ConfirmationStatusConfirmed ||
				status.ConfirmationStatus == rpc.ConfirmationStatusFinalized {
				return nil
			}
		}
	}
}

type projectedFill struct {
	notional  uint64
	oi        uint64
	skew      *big.Int
	impactBps uint64
	upper     uint64
	lower     uint64
}

func calculateFillPrice(
	order *orderengine.Order,
	market *orderengine.Market,
	funding *orderengine.MarketFundingState,
	oraclePrice uint64,
) (uint64, error) {
	if oraclePrice == 0 {
		return 0, fmt.Errorf("oracle price must be > 0")
	}

	if order.ReduceOnly {
		var fill uint64
		switch order.OrderType {
		case orderengine.OrderType_Limit:
			fill = order.Price
		default:
			fill = oraclePrice
		}
		notional := order.Margin
		if notional == 0 {
			return 0, fmt.Errorf("notional resolved to zero")
		}
		if notional > market.RiskParams.MaxTradeNotional {
			return 0, fmt.Errorf("notional exceeds max_trade_notional")
		}
		return fill, nil
	}

	fill := oraclePrice
	for i := 0; i < 8; i++ {
		projection, err := projectFill(order, market, funding, oraclePrice, fill)
		if err != nil {
			return 0, err
		}

		nextFill := fill
		switch order.Side {
		case orderengine.Side_Buy:
			if fill < projection.upper {
				nextFill = projection.upper
			}
		case orderengine.Side_Sell:
			if fill > projection.lower {
				nextFill = projection.lower
			}
		default:
			return 0, fmt.Errorf("unknown side")
		}

		if nextFill == fill {
			break
		}
		fill = nextFill
	}

	projection, err := projectFill(order, market, funding, oraclePrice, fill)
	if err != nil {
		return 0, err
	}
	if projection.notional > market.RiskParams.MaxTradeNotional {
		return 0, fmt.Errorf("notional exceeds max_trade_notional")
	}
	if projection.oi > market.RiskParams.OiCap {
		return 0, fmt.Errorf("projected open interest exceeds oi_cap")
	}
	if absBigInt(projection.skew).Cmp(new(big.Int).SetUint64(market.RiskParams.SkewCap)) > 0 {
		return 0, fmt.Errorf("projected skew exceeds skew_cap")
	}

	maxDeviationBps := uint64(market.PricingParams.MaxFillDeviationBps)
	side := order.Side
	switch side {
	case orderengine.Side_Buy:
		maxFill, err := mulDivFloor(oraclePrice, bpsDenom+maxDeviationBps, bpsDenom)
		if err != nil {
			return 0, err
		}
		if fill < projection.upper || fill > maxFill {
			return 0, fmt.Errorf("buy fill outside oracle guardrail")
		}
		if order.OrderType == orderengine.OrderType_Limit && fill > order.Price {
			return 0, fmt.Errorf("buy fill violates limit")
		}
	case orderengine.Side_Sell:
		if maxDeviationBps > bpsDenom {
			return 0, fmt.Errorf("invalid max deviation bps")
		}
		minFill, err := mulDivFloor(oraclePrice, bpsDenom-maxDeviationBps, bpsDenom)
		if err != nil {
			return 0, err
		}
		if fill > projection.lower || fill < minFill {
			return 0, fmt.Errorf("sell fill outside oracle guardrail")
		}
		if order.OrderType == orderengine.OrderType_Limit && fill < order.Price {
			return 0, fmt.Errorf("sell fill violates limit")
		}
	default:
		return 0, fmt.Errorf("unknown side")
	}

	if fill == 0 {
		return 0, fmt.Errorf("fill resolved to zero")
	}
	return fill, nil
}

func projectFill(
	order *orderengine.Order,
	market *orderengine.Market,
	funding *orderengine.MarketFundingState,
	oraclePrice uint64,
	fill uint64,
) (*projectedFill, error) {
	_ = fill
	notional := order.Margin
	if notional == 0 {
		return nil, fmt.Errorf("notional resolved to zero")
	}
	projectedOI := funding.OpenInterest + notional
	if projectedOI == 0 {
		return nil, fmt.Errorf("projected open interest is zero")
	}

	projectedSkew := new(big.Int).Set(funding.Skew.BigInt())
	notionalBig := new(big.Int).SetUint64(notional)
	switch order.Side {
	case orderengine.Side_Buy:
		projectedSkew.Add(projectedSkew, notionalBig)
	case orderengine.Side_Sell:
		projectedSkew.Sub(projectedSkew, notionalBig)
	default:
		return nil, fmt.Errorf("unknown side")
	}

	absSkew := absBigInt(projectedSkew)
	skewRatioBpsBig := new(big.Int).Mul(absSkew, new(big.Int).SetUint64(bpsDenom))
	skewRatioBpsBig.Div(skewRatioBpsBig, new(big.Int).SetUint64(projectedOI))
	if !skewRatioBpsBig.IsUint64() {
		return nil, fmt.Errorf("skew ratio overflow")
	}
	skewRatioBps := skewRatioBpsBig.Uint64()

	skewImpactBps, err := mulDivFloor(uint64(market.PricingParams.SkewCoeffBps), skewRatioBps, bpsDenom)
	if err != nil {
		return nil, err
	}
	impactBps := uint64(market.PricingParams.BaseSpreadBps) + skewImpactBps
	if impactBps >= bpsDenom {
		return nil, fmt.Errorf("impact bps too large")
	}

	upper, err := mulDivFloor(oraclePrice, bpsDenom+impactBps, bpsDenom)
	if err != nil {
		return nil, err
	}
	lower, err := mulDivFloor(oraclePrice, bpsDenom-impactBps, bpsDenom)
	if err != nil {
		return nil, err
	}

	return &projectedFill{
		notional:  notional,
		oi:        projectedOI,
		skew:      projectedSkew,
		impactBps: impactBps,
		upper:     upper,
		lower:     lower,
	}, nil
}

func mulDivFloor(a, b, denominator uint64) (uint64, error) {
	if denominator == 0 {
		return 0, fmt.Errorf("division by zero")
	}
	left := new(big.Int).SetUint64(a)
	right := new(big.Int).SetUint64(b)
	left.Mul(left, right)
	left.Div(left, new(big.Int).SetUint64(denominator))
	if left.Sign() < 0 || !left.IsUint64() {
		return 0, fmt.Errorf("mulDiv overflow")
	}
	return left.Uint64(), nil
}

func absBigInt(v *big.Int) *big.Int {
	if v.Sign() >= 0 {
		return new(big.Int).Set(v)
	}
	return new(big.Int).Neg(v)
}

func containsPubkey(slice []solana.PublicKey, target solana.PublicKey) bool {
	for _, item := range slice {
		if item.Equals(target) {
			return true
		}
	}
	return false
}
