package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	lpvault "github.com/coldbell/dex/backend/internal/anchor/lp_vault"
	orderengine "github.com/coldbell/dex/backend/internal/anchor/order_engine"
	"github.com/coldbell/dex/backend/internal/config"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

type Service struct {
	cfg                config.IndexerConfig
	rpc                *rpc.Client
	store              *Store
	logger             *slog.Logger
	orderbookCollector *orderbookCollector
}

func New(cfg config.IndexerConfig, logger *slog.Logger) (*Service, error) {
	store, err := NewStore(cfg.DBDSN)
	if err != nil {
		return nil, fmt.Errorf("init store: %w", err)
	}

	orderengine.ProgramID = cfg.OrderEngineProgramID
	lpvault.ProgramID = cfg.LpVaultProgramID

	return &Service{
		cfg:    cfg,
		rpc:    rpc.New(cfg.RPCURL),
		store:  store,
		logger: logger,
		orderbookCollector: newOrderbookCollector(
			cfg.OrderbookTargets,
			cfg.OrderbookDepth,
			cfg.OrderbookRequestTimeout,
			cfg.OrderbookRefreshInterval,
			logger,
		),
	}, nil
}

func (s *Service) Run(ctx context.Context) error {
	defer func() {
		if err := s.store.Close(); err != nil {
			s.logger.Error("failed to close store", "err", err)
		}
	}()

	s.logger.Info("indexer started",
		"rpc", s.cfg.RPCURL,
		"db_driver", "postgres",
		"commitment", s.cfg.Commitment,
	)
	if len(s.cfg.OrderbookTargets) > 0 {
		s.logger.Info(
			"orderbook snapshot collector enabled",
			"targets", len(s.cfg.OrderbookTargets),
			"interval", s.cfg.OrderbookSnapshotInterval.String(),
			"refresh_interval", s.cfg.OrderbookRefreshInterval.String(),
			"depth", s.cfg.OrderbookDepth,
		)
	}

	if err := s.syncOnce(ctx); err != nil {
		s.logger.Error("initial sync failed", "err", err)
	}
	s.initializeLocalOrderbook(ctx)
	if s.cfg.EnablePythPriceStream {
		go s.runPythPriceStream(ctx)
	}

	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()

	var snapshotTimer *time.Timer
	var snapshotTickC <-chan time.Time
	if len(s.cfg.OrderbookTargets) > 0 {
		snapshotTimer = time.NewTimer(nextOrderbookSnapshotDelay(s.cfg.OrderbookSnapshotInterval))
		snapshotTickC = snapshotTimer.C
		defer snapshotTimer.Stop()
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("indexer stopped")
			return nil
		case <-ticker.C:
			if err := s.syncOnce(ctx); err != nil {
				s.logger.Error("sync failed", "err", err)
			}
		case now := <-snapshotTickC:
			snapshotTime := previousSnapshotTime(now, s.cfg.OrderbookSnapshotInterval)
			if snapshotTime > 0 {
				if err := s.syncOrderbookSnapshots(ctx, snapshotTime); err != nil {
					s.logger.Error("orderbook snapshot sync failed", "err", err)
				}
			}
			snapshotTimer.Reset(nextOrderbookSnapshotDelay(s.cfg.OrderbookSnapshotInterval))
		}
	}
}

func (s *Service) initializeLocalOrderbook(ctx context.Context) {
	if s.orderbookCollector == nil || len(s.cfg.OrderbookTargets) == 0 {
		return
	}

	refreshErrors := s.orderbookCollector.Bootstrap(ctx)
	for target, err := range refreshErrors {
		if err != nil {
			s.logger.Warn(
				"orderbook initial refresh failed",
				"exchange", target.Exchange,
				"symbol", target.Symbol,
				"err", err,
			)
		}
	}

	s.orderbookCollector.Start(ctx)
}

func (s *Service) syncOrderbookSnapshots(ctx context.Context, snapshotTime int64) error {
	if s.orderbookCollector == nil || len(s.cfg.OrderbookTargets) == 0 {
		return nil
	}
	targetSnapshots := s.orderbookCollector.SnapshotAll(snapshotTime)

	if len(targetSnapshots) == 0 {
		return nil
	}

	return s.store.WithTx(ctx, func(tx *Tx) error {
		for _, snapshot := range targetSnapshots {
			if _, err := s.store.UpsertOrderbookSnapshotTx(ctx, tx, snapshot); err != nil {
				return fmt.Errorf("upsert orderbook snapshot for %s:%s: %w", snapshot.Exchange, snapshot.Symbol, err)
			}
		}
		return nil
	})
}

func nextOrderbookSnapshotDelay(interval time.Duration) time.Duration {
	normalizedInterval := normalizeOrderbookSnapshotInterval(interval)
	stepMinutes := int64(normalizedInterval / time.Minute)
	now := time.Now()
	currentMinute := now.Unix() / 60
	nextMinute := ((currentMinute / stepMinutes) + 1) * stepMinutes
	nextBoundary := time.Unix(nextMinute*60, 0)
	delay := time.Until(nextBoundary)
	if delay <= 0 {
		return time.Second
	}
	return delay
}

func previousSnapshotTime(now time.Time, interval time.Duration) int64 {
	normalizedInterval := normalizeOrderbookSnapshotInterval(interval)
	stepMinutes := int64(normalizedInterval / time.Minute)
	currentMinute := now.Unix() / 60
	closeMinute := (currentMinute / stepMinutes) * stepMinutes
	previousMinute := closeMinute - stepMinutes
	return previousMinute * 60
}

func normalizeOrderbookSnapshotInterval(interval time.Duration) time.Duration {
	if interval < time.Minute {
		return time.Minute
	}
	wholeMinutes := int64(interval / time.Minute)
	if wholeMinutes <= 0 {
		return time.Minute
	}
	return time.Duration(wholeMinutes) * time.Minute
}

func (s *Service) syncOnce(ctx context.Context) error {
	slot, err := s.rpc.GetSlot(ctx, s.cfg.Commitment)
	if err != nil {
		return fmt.Errorf("get slot: %w", err)
	}

	stats := map[string]int{}

	err = s.store.WithTx(ctx, func(tx *Tx) error {
		if err := s.syncOrderEngine(ctx, tx, slot, stats); err != nil {
			return err
		}
		if err := s.syncMarketRegistry(ctx, tx, slot, stats); err != nil {
			return err
		}
		if err := s.syncLPVault(ctx, tx, slot, stats); err != nil {
			return err
		}
		return s.store.UpsertSyncStateTx(ctx, tx, slot)
	})
	if err != nil {
		return err
	}

	s.logger.Info(
		"sync complete",
		"slot", slot,
		"orders", stats["orders"],
		"positions", stats["positions"],
		"lp_positions", stats["lp_positions"],
		"resources", stats["resources"],
	)

	return nil
}

func (s *Service) syncOrderEngine(ctx context.Context, tx *Tx, slot uint64, stats map[string]int) error {
	programID := s.cfg.OrderEngineProgramID

	if err := s.scanAndStore(ctx, tx, slot, programID, "EngineConfig", orderengine.Account_EngineConfig,
		func(item *rpc.KeyedAccount) error {
			payload, err := orderengine.ParseAccount_EngineConfig(item.Account.Data.GetBinary())
			if err != nil {
				return err
			}
			stats["resources"]++
			return s.store.UpsertResourceTx(ctx, tx, item.Pubkey, programID, "EngineConfig", item.Account.Owner, item.Account.Lamports, slot, payload)
		}); err != nil {
		return err
	}

	if err := s.scanAndStore(ctx, tx, slot, programID, "UserMargin", orderengine.Account_UserMargin,
		func(item *rpc.KeyedAccount) error {
			payload, err := orderengine.ParseAccount_UserMargin(item.Account.Data.GetBinary())
			if err != nil {
				return err
			}
			stats["resources"]++
			return s.store.UpsertResourceTx(ctx, tx, item.Pubkey, programID, "UserMargin", item.Account.Owner, item.Account.Lamports, slot, payload)
		}); err != nil {
		return err
	}

	if err := s.scanAndStore(ctx, tx, slot, programID, "MarketFundingState", orderengine.Account_MarketFundingState,
		func(item *rpc.KeyedAccount) error {
			payload, err := orderengine.ParseAccount_MarketFundingState(item.Account.Data.GetBinary())
			if err != nil {
				return err
			}
			stats["resources"]++
			return s.store.UpsertResourceTx(ctx, tx, item.Pubkey, programID, "MarketFundingState", item.Account.Owner, item.Account.Lamports, slot, payload)
		}); err != nil {
		return err
	}

	if err := s.scanAndStore(ctx, tx, slot, programID, "Order", orderengine.Account_Order,
		func(item *rpc.KeyedAccount) error {
			payload, err := orderengine.ParseAccount_Order(item.Account.Data.GetBinary())
			if err != nil {
				return err
			}
			stats["orders"]++
			return s.store.UpsertOrderTx(ctx, tx, item.Pubkey, slot, payload)
		}); err != nil {
		return err
	}

	if err := s.scanAndStore(ctx, tx, slot, programID, "UserMarketPosition", orderengine.Account_UserMarketPosition,
		func(item *rpc.KeyedAccount) error {
			payload, err := orderengine.ParseAccount_UserMarketPosition(item.Account.Data.GetBinary())
			if err != nil {
				return err
			}
			stats["positions"]++
			return s.store.UpsertPositionTx(ctx, tx, item.Pubkey, slot, payload)
		}); err != nil {
		return err
	}

	return nil
}

func (s *Service) syncMarketRegistry(ctx context.Context, tx *Tx, slot uint64, stats map[string]int) error {
	programID := s.cfg.MarketRegistryProgramID

	if err := s.scanAndStore(ctx, tx, slot, programID, "GlobalConfig", orderengine.Account_GlobalConfig,
		func(item *rpc.KeyedAccount) error {
			payload, err := orderengine.ParseAccount_GlobalConfig(item.Account.Data.GetBinary())
			if err != nil {
				return err
			}
			stats["resources"]++
			return s.store.UpsertResourceTx(ctx, tx, item.Pubkey, programID, "GlobalConfig", item.Account.Owner, item.Account.Lamports, slot, payload)
		}); err != nil {
		return err
	}

	if err := s.scanAndStore(ctx, tx, slot, programID, "KeeperSet", orderengine.Account_KeeperSet,
		func(item *rpc.KeyedAccount) error {
			payload, err := orderengine.ParseAccount_KeeperSet(item.Account.Data.GetBinary())
			if err != nil {
				return err
			}
			stats["resources"]++
			return s.store.UpsertResourceTx(ctx, tx, item.Pubkey, programID, "KeeperSet", item.Account.Owner, item.Account.Lamports, slot, payload)
		}); err != nil {
		return err
	}

	if err := s.scanAndStore(ctx, tx, slot, programID, "Market", orderengine.Account_Market,
		func(item *rpc.KeyedAccount) error {
			payload, err := orderengine.ParseAccount_Market(item.Account.Data.GetBinary())
			if err != nil {
				return err
			}
			stats["resources"]++
			return s.store.UpsertResourceTx(ctx, tx, item.Pubkey, programID, "Market", item.Account.Owner, item.Account.Lamports, slot, payload)
		}); err != nil {
		return err
	}

	return nil
}

func (s *Service) syncLPVault(ctx context.Context, tx *Tx, slot uint64, stats map[string]int) error {
	programID := s.cfg.LpVaultProgramID

	if err := s.scanAndStore(ctx, tx, slot, programID, "Pool", lpvault.Account_Pool,
		func(item *rpc.KeyedAccount) error {
			payload, err := lpvault.ParseAccount_Pool(item.Account.Data.GetBinary())
			if err != nil {
				return err
			}
			stats["resources"]++
			return s.store.UpsertResourceTx(ctx, tx, item.Pubkey, programID, "Pool", item.Account.Owner, item.Account.Lamports, slot, payload)
		}); err != nil {
		return err
	}

	if err := s.scanAndStore(ctx, tx, slot, programID, "KeeperRebate", lpvault.Account_KeeperRebate,
		func(item *rpc.KeyedAccount) error {
			payload, err := lpvault.ParseAccount_KeeperRebate(item.Account.Data.GetBinary())
			if err != nil {
				return err
			}
			stats["resources"]++
			return s.store.UpsertResourceTx(ctx, tx, item.Pubkey, programID, "KeeperRebate", item.Account.Owner, item.Account.Lamports, slot, payload)
		}); err != nil {
		return err
	}

	if err := s.scanAndStore(ctx, tx, slot, programID, "LpPosition", lpvault.Account_LpPosition,
		func(item *rpc.KeyedAccount) error {
			payload, err := lpvault.ParseAccount_LpPosition(item.Account.Data.GetBinary())
			if err != nil {
				return err
			}
			stats["lp_positions"]++
			return s.store.UpsertLPPositionTx(ctx, tx, item.Pubkey, slot, payload)
		}); err != nil {
		return err
	}

	if err := s.scanAndStore(ctx, tx, slot, programID, "WithdrawRequest", lpvault.Account_WithdrawRequest,
		func(item *rpc.KeyedAccount) error {
			payload, err := lpvault.ParseAccount_WithdrawRequest(item.Account.Data.GetBinary())
			if err != nil {
				return err
			}
			stats["resources"]++
			return s.store.UpsertResourceTx(ctx, tx, item.Pubkey, programID, "WithdrawRequest", item.Account.Owner, item.Account.Lamports, slot, payload)
		}); err != nil {
		return err
	}

	return nil
}

func (s *Service) scanAndStore(
	ctx context.Context,
	tx *Tx,
	slot uint64,
	programID solana.PublicKey,
	accountType string,
	discriminator [8]byte,
	handler func(item *rpc.KeyedAccount) error,
) error {
	accounts, err := s.rpc.GetProgramAccountsWithOpts(ctx, programID, &rpc.GetProgramAccountsOpts{
		Commitment: s.cfg.Commitment,
		Filters: []rpc.RPCFilter{
			{Memcmp: &rpc.RPCFilterMemcmp{Offset: 0, Bytes: solana.Base58(discriminator[:])}},
		},
	})
	if err != nil {
		return fmt.Errorf("scan %s accounts for program %s: %w", accountType, programID, err)
	}

	for _, item := range accounts {
		if item == nil || item.Account == nil {
			continue
		}
		if err := handler(item); err != nil {
			s.logger.Warn("failed to index account",
				"program", programID,
				"account_type", accountType,
				"pubkey", item.Pubkey,
				"slot", slot,
				"err", err,
			)
		}
	}
	return nil
}
