package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"gopkg.in/yaml.v3"
)

type KeeperOracleConfig struct {
	Price         uint64 `json:"price"`
	ConfBps       uint64 `json:"conf_bps"`
	PublishLagSec int64  `json:"publish_lag_sec"`
}

type LogConfig struct {
	Level    string
	Format   string
	Output   string
	FilePath string
}

type KeeperConfig struct {
	RPCURL                         string
	Commitment                     rpc.CommitmentType
	KeypairPath                    string
	PollInterval                   time.Duration
	MaxOrdersPerTick               int
	TxTimeout                      time.Duration
	SkipPreflight                  bool
	MaxRetries                     *uint
	ComputeUnitLimit               uint32
	ComputeUnitPriceMicroLamports  uint64
	DefaultOraclePrice             uint64
	DefaultOracleConfBps           uint64
	DefaultOraclePublishLagSec     int64
	OracleByMarket                 map[uint64]KeeperOracleConfig
	PythPriceUpdateAccountByMarket map[uint64]solana.PublicKey
	OrderEngineProgramID           solana.PublicKey
	MarketRegistryProgramID        solana.PublicKey
	LpVaultProgramID               solana.PublicKey
	Log                            LogConfig
}

type IndexerConfig struct {
	RPCURL                    string
	Commitment                rpc.CommitmentType
	PollInterval              time.Duration
	RPCMaxRetries             int
	RPCRetryBaseDelay         time.Duration
	RPCRetryMaxDelay          time.Duration
	DBDSN                     string
	OrderEngineProgramID      solana.PublicKey
	MarketRegistryProgramID   solana.PublicKey
	LpVaultProgramID          solana.PublicKey
	OrderbookSnapshotInterval time.Duration
	OrderbookRefreshInterval  time.Duration
	OrderbookTargets          []OrderbookTarget
	OrderbookDepth            int
	OrderbookRequestTimeout   time.Duration
	EnablePythPriceStream     bool
	PythStreamURL             string
	PythFeedID                string
	PythMarket                string
	PythReconnectInterval     time.Duration
	Log                       LogConfig
}

type OrderbookTarget struct {
	Exchange string
	Symbol   string
}

type APIServerConfig struct {
	ListenAddr     string
	DBDSN          string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	IdleTimeout    time.Duration
	AllowedOrigins []string
	Log            LogConfig
}

var (
	defaultOrderEngineProgramID    = solana.MustPublicKeyFromBase58("GpMobZUKPtEE1eiZQAADo2ecD54JXhNHPNts5kPGwLtb")
	defaultMarketRegistryProgramID = solana.MustPublicKeyFromBase58("BsA8fuyw8XqBMiUfpLbdiBwbKg8MZMHB1jdZzjs7c46q")
	defaultLPVaultProgramID        = solana.MustPublicKeyFromBase58("F8gkLV5nMaCG16PQAwkKKsTdWC2yuPektUXAFHQF4Cds")
	defaultPythStreamURL           = "https://hermes.pyth.network/v2/updates/price/stream"
	defaultPythBTCUSDFeedID        = "e62df6c8b4a85fe1a67db44dc12de5db330f7ac66b72dc658afedf0f4a415b43"
)

func LoadKeeperConfig() (KeeperConfig, error) {
	if err := ensureRuntimeConfigLoaded(); err != nil {
		return KeeperConfig{}, err
	}

	keypairPath := envOrDefault("KEEPER_KEYPAIR_PATH", envOrDefault("SOLANA_KEYPAIR_PATH", "~/.config/solana/id.json"))
	keypairPath = maybeUseLocalSecretKeypair(keypairPath)
	expandedKeypair, err := expandHomePath(keypairPath)
	if err != nil {
		return KeeperConfig{}, fmt.Errorf("expand keypair path: %w", err)
	}

	pollInterval, err := envDuration("KEEPER_POLL_INTERVAL", 1500*time.Millisecond)
	if err != nil {
		return KeeperConfig{}, err
	}

	txTimeout, err := envDuration("KEEPER_TX_TIMEOUT", 30*time.Second)
	if err != nil {
		return KeeperConfig{}, err
	}

	maxOrders, err := envInt("KEEPER_MAX_ORDERS_PER_TICK", 10)
	if err != nil {
		return KeeperConfig{}, err
	}

	commitment, err := envCommitment("SOLANA_COMMITMENT", rpc.CommitmentConfirmed)
	if err != nil {
		return KeeperConfig{}, err
	}

	skipPreflight, err := envBool("KEEPER_SKIP_PREFLIGHT", false)
	if err != nil {
		return KeeperConfig{}, err
	}

	maxRetries, err := envOptionalUint("KEEPER_MAX_RETRIES")
	if err != nil {
		return KeeperConfig{}, err
	}

	cuLimit, err := envUint32("KEEPER_COMPUTE_UNIT_LIMIT", 0)
	if err != nil {
		return KeeperConfig{}, err
	}

	cuPrice, err := envUint64("KEEPER_COMPUTE_UNIT_PRICE_MICRO_LAMPORTS", 0)
	if err != nil {
		return KeeperConfig{}, err
	}

	defaultOraclePrice, err := envUint64("KEEPER_DEFAULT_ORACLE_PRICE", 0)
	if err != nil {
		return KeeperConfig{}, err
	}

	defaultOracleConfBps, err := envUint64("KEEPER_DEFAULT_ORACLE_CONF_BPS", 10)
	if err != nil {
		return KeeperConfig{}, err
	}

	defaultOracleLag, err := envInt64("KEEPER_DEFAULT_ORACLE_PUBLISH_LAG_SEC", 1)
	if err != nil {
		return KeeperConfig{}, err
	}

	oracleByMarket, err := parseOracleMap(envOrDefault("KEEPER_ORACLE_PRICES_JSON", ""))
	if err != nil {
		return KeeperConfig{}, err
	}
	pythPriceUpdateAccountByMarket, err := parsePubkeyMap(envOrDefault("KEEPER_PYTH_PRICE_UPDATE_ACCOUNTS_JSON", ""))
	if err != nil {
		return KeeperConfig{}, err
	}

	orderEngineProgramID, err := envPubkey("ORDER_ENGINE_PROGRAM_ID", defaultOrderEngineProgramID)
	if err != nil {
		return KeeperConfig{}, err
	}
	marketRegistryProgramID, err := envPubkey("MARKET_REGISTRY_PROGRAM_ID", defaultMarketRegistryProgramID)
	if err != nil {
		return KeeperConfig{}, err
	}
	lpVaultProgramID, err := envPubkey("LP_VAULT_PROGRAM_ID", defaultLPVaultProgramID)
	if err != nil {
		return KeeperConfig{}, err
	}

	return KeeperConfig{
		RPCURL:                         envOrDefault("SOLANA_RPC_URL", "http://127.0.0.1:8899"),
		Commitment:                     commitment,
		KeypairPath:                    expandedKeypair,
		PollInterval:                   pollInterval,
		MaxOrdersPerTick:               maxOrders,
		TxTimeout:                      txTimeout,
		SkipPreflight:                  skipPreflight,
		MaxRetries:                     maxRetries,
		ComputeUnitLimit:               cuLimit,
		ComputeUnitPriceMicroLamports:  cuPrice,
		DefaultOraclePrice:             defaultOraclePrice,
		DefaultOracleConfBps:           defaultOracleConfBps,
		DefaultOraclePublishLagSec:     defaultOracleLag,
		OracleByMarket:                 oracleByMarket,
		PythPriceUpdateAccountByMarket: pythPriceUpdateAccountByMarket,
		OrderEngineProgramID:           orderEngineProgramID,
		MarketRegistryProgramID:        marketRegistryProgramID,
		LpVaultProgramID:               lpVaultProgramID,
		Log:                            buildLogConfig("KEEPER", "keeper"),
	}, nil
}

func LoadIndexerConfig() (IndexerConfig, error) {
	if err := ensureRuntimeConfigLoaded(); err != nil {
		return IndexerConfig{}, err
	}

	pollInterval, err := envDuration("INDEXER_POLL_INTERVAL", 2*time.Second)
	if err != nil {
		return IndexerConfig{}, err
	}
	rpcMaxRetries, err := envInt("INDEXER_RPC_MAX_RETRIES", 6)
	if err != nil {
		return IndexerConfig{}, err
	}
	rpcRetryBaseDelay, err := envDuration("INDEXER_RPC_RETRY_BASE_DELAY", time.Second)
	if err != nil {
		return IndexerConfig{}, err
	}
	rpcRetryMaxDelay, err := envDuration("INDEXER_RPC_RETRY_MAX_DELAY", 20*time.Second)
	if err != nil {
		return IndexerConfig{}, err
	}
	if rpcRetryMaxDelay < rpcRetryBaseDelay {
		return IndexerConfig{}, fmt.Errorf("invalid INDEXER_RPC_RETRY_MAX_DELAY: must be >= INDEXER_RPC_RETRY_BASE_DELAY")
	}

	commitment, err := envCommitment("SOLANA_COMMITMENT", rpc.CommitmentConfirmed)
	if err != nil {
		return IndexerConfig{}, err
	}

	dbDSN := envOrDefault("INDEXER_DB_DSN", "postgres://postgres:postgres@127.0.0.1:5432/dex?sslmode=disable")

	orderEngineProgramID, err := envPubkey("ORDER_ENGINE_PROGRAM_ID", defaultOrderEngineProgramID)
	if err != nil {
		return IndexerConfig{}, err
	}
	marketRegistryProgramID, err := envPubkey("MARKET_REGISTRY_PROGRAM_ID", defaultMarketRegistryProgramID)
	if err != nil {
		return IndexerConfig{}, err
	}
	lpVaultProgramID, err := envPubkey("LP_VAULT_PROGRAM_ID", defaultLPVaultProgramID)
	if err != nil {
		return IndexerConfig{}, err
	}

	orderbookSnapshotInterval, err := envDuration("INDEXER_ORDERBOOK_SNAPSHOT_INTERVAL", time.Minute)
	if err != nil {
		return IndexerConfig{}, err
	}
	orderbookRefreshInterval, err := envDuration("INDEXER_ORDERBOOK_REFRESH_INTERVAL", time.Second)
	if err != nil {
		return IndexerConfig{}, err
	}
	orderbookDepth, err := envInt("INDEXER_ORDERBOOK_DEPTH", 100)
	if err != nil {
		return IndexerConfig{}, err
	}
	orderbookRequestTimeout, err := envDuration("INDEXER_ORDERBOOK_REQUEST_TIMEOUT", 10*time.Second)
	if err != nil {
		return IndexerConfig{}, err
	}
	orderbookTargets, err := parseOrderbookTargets(envOrDefault("INDEXER_ORDERBOOK_TARGETS", ""))
	if err != nil {
		return IndexerConfig{}, err
	}
	enablePythPriceStream, err := envBool("INDEXER_ENABLE_PYTH_PRICE_STREAM", true)
	if err != nil {
		return IndexerConfig{}, err
	}
	pythReconnectInterval, err := envDuration("INDEXER_PYTH_RECONNECT_INTERVAL", 3*time.Second)
	if err != nil {
		return IndexerConfig{}, err
	}

	return IndexerConfig{
		RPCURL:                    envOrDefault("SOLANA_RPC_URL", "http://127.0.0.1:8899"),
		Commitment:                commitment,
		PollInterval:              pollInterval,
		RPCMaxRetries:             rpcMaxRetries,
		RPCRetryBaseDelay:         rpcRetryBaseDelay,
		RPCRetryMaxDelay:          rpcRetryMaxDelay,
		DBDSN:                     dbDSN,
		OrderEngineProgramID:      orderEngineProgramID,
		MarketRegistryProgramID:   marketRegistryProgramID,
		LpVaultProgramID:          lpVaultProgramID,
		OrderbookSnapshotInterval: orderbookSnapshotInterval,
		OrderbookRefreshInterval:  orderbookRefreshInterval,
		OrderbookTargets:          orderbookTargets,
		OrderbookDepth:            orderbookDepth,
		OrderbookRequestTimeout:   orderbookRequestTimeout,
		EnablePythPriceStream:     enablePythPriceStream,
		PythStreamURL:             envOrDefault("INDEXER_PYTH_STREAM_URL", defaultPythStreamURL),
		PythFeedID:                strings.ToLower(strings.TrimSpace(envOrDefault("INDEXER_PYTH_FEED_ID", defaultPythBTCUSDFeedID))),
		PythMarket:                strings.ToUpper(strings.TrimSpace(envOrDefault("INDEXER_PYTH_MARKET", "BTCUSDT"))),
		PythReconnectInterval:     pythReconnectInterval,
		Log:                       buildLogConfig("INDEXER", "indexer"),
	}, nil
}

func LoadAPIServerConfig() (APIServerConfig, error) {
	if err := ensureRuntimeConfigLoaded(); err != nil {
		return APIServerConfig{}, err
	}

	dbDSN := envOrDefault("API_SERVER_DB_DSN", envOrDefault("INDEXER_DB_DSN", "postgres://postgres:postgres@127.0.0.1:5432/dex?sslmode=disable"))

	readTimeout, err := envDuration("API_SERVER_READ_TIMEOUT", 10*time.Second)
	if err != nil {
		return APIServerConfig{}, err
	}
	writeTimeout, err := envDuration("API_SERVER_WRITE_TIMEOUT", 15*time.Second)
	if err != nil {
		return APIServerConfig{}, err
	}
	idleTimeout, err := envDuration("API_SERVER_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		return APIServerConfig{}, err
	}

	allowedOrigins := parseCSVEnv(
		envOrDefault("API_SERVER_ALLOWED_ORIGINS", "*"),
		[]string{"*"},
	)

	return APIServerConfig{
		ListenAddr:     envOrDefault("API_SERVER_LISTEN_ADDR", ":8080"),
		DBDSN:          dbDSN,
		ReadTimeout:    readTimeout,
		WriteTimeout:   writeTimeout,
		IdleTimeout:    idleTimeout,
		AllowedOrigins: allowedOrigins,
		Log:            buildLogConfig("API_SERVER", "api-server"),
	}, nil
}

type ConfigSource struct {
	Phase  string
	Path   string
	Loaded bool
}

func CurrentConfigSource() (ConfigSource, error) {
	if err := ensureRuntimeConfigLoaded(); err != nil {
		return ConfigSource{}, err
	}
	return ConfigSource{
		Phase:  runtimeConfigPhase,
		Path:   runtimeConfigPath,
		Loaded: runtimeConfigLoaded,
	}, nil
}

func parseOracleMap(raw string) (map[uint64]KeeperOracleConfig, error) {
	out := make(map[uint64]KeeperOracleConfig)
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}

	var temp map[string]KeeperOracleConfig
	if err := json.Unmarshal([]byte(raw), &temp); err != nil {
		return nil, fmt.Errorf("parse KEEPER_ORACLE_PRICES_JSON: %w", err)
	}

	for key, value := range temp {
		marketID, err := strconv.ParseUint(key, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid market id %q in KEEPER_ORACLE_PRICES_JSON: %w", key, err)
		}
		out[marketID] = value
	}

	return out, nil
}

func parsePubkeyMap(raw string) (map[uint64]solana.PublicKey, error) {
	out := make(map[uint64]solana.PublicKey)
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}

	var temp map[string]string
	if err := json.Unmarshal([]byte(raw), &temp); err != nil {
		return nil, fmt.Errorf("parse KEEPER_PYTH_PRICE_UPDATE_ACCOUNTS_JSON: %w", err)
	}

	for key, value := range temp {
		marketID, err := strconv.ParseUint(key, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid market id %q in KEEPER_PYTH_PRICE_UPDATE_ACCOUNTS_JSON: %w", key, err)
		}

		pubkey, err := solana.PublicKeyFromBase58(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid pubkey for market %d in KEEPER_PYTH_PRICE_UPDATE_ACCOUNTS_JSON: %w", marketID, err)
		}
		out[marketID] = pubkey
	}

	return out, nil
}

func parseOrderbookTargets(raw string) ([]OrderbookTarget, error) {
	raw = strings.TrimSpace(raw)
	parts := parseCSVEnv(raw, nil)
	if len(parts) == 0 {
		return defaultOrderbookTargets(), nil
	}

	out := make([]OrderbookTarget, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		rawPair := strings.Split(part, ":")
		if len(rawPair) != 2 {
			return nil, fmt.Errorf("invalid INDEXER_ORDERBOOK_TARGETS entry %q, expected exchange:symbol", part)
		}
		exchange := strings.ToLower(strings.TrimSpace(rawPair[0]))
		symbol := strings.TrimSpace(rawPair[1])
		if exchange == "" || symbol == "" {
			return nil, fmt.Errorf("invalid INDEXER_ORDERBOOK_TARGETS entry %q, exchange and symbol are required", part)
		}

		key := exchange + ":" + symbol
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, OrderbookTarget{
			Exchange: exchange,
			Symbol:   symbol,
		})
	}

	if len(out) == 0 {
		return defaultOrderbookTargets(), nil
	}

	return out, nil
}

func defaultOrderbookTargets() []OrderbookTarget {
	return []OrderbookTarget{
		{Exchange: "binance", Symbol: "BTCUSDT"},
		{Exchange: "okx", Symbol: "BTC-USDT"},
		{Exchange: "coinbase", Symbol: "BTC-USD"},
		{Exchange: "bybit", Symbol: "BTCUSDT"},
	}
}

func buildLogConfig(prefix string, serviceName string) LogConfig {
	level := envOrDefault(prefix+"_LOG_LEVEL", envOrDefault("LOG_LEVEL", "info"))
	format := envOrDefault(prefix+"_LOG_FORMAT", envOrDefault("LOG_FORMAT", "text"))
	output := envOrDefault(prefix+"_LOG_OUTPUT", envOrDefault("LOG_OUTPUT", "console"))
	filePath := envOrDefault(prefix+"_LOG_FILE", envOrDefault("LOG_FILE", filepath.Join(".docker", serviceName, serviceName+".log")))

	return LogConfig{
		Level:    level,
		Format:   format,
		Output:   output,
		FilePath: filePath,
	}
}

func envPubkey(key string, fallback solana.PublicKey) (solana.PublicKey, error) {
	raw := strings.TrimSpace(valueForKey(key))
	if raw == "" {
		return fallback, nil
	}
	pk, err := solana.PublicKeyFromBase58(raw)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("invalid %s: %w", key, err)
	}
	return pk, nil
}

func envCommitment(key string, fallback rpc.CommitmentType) (rpc.CommitmentType, error) {
	raw := strings.TrimSpace(valueForKey(key))
	if raw == "" {
		return fallback, nil
	}
	switch strings.ToLower(raw) {
	case string(rpc.CommitmentProcessed):
		return rpc.CommitmentProcessed, nil
	case string(rpc.CommitmentConfirmed):
		return rpc.CommitmentConfirmed, nil
	case string(rpc.CommitmentFinalized):
		return rpc.CommitmentFinalized, nil
	default:
		return "", fmt.Errorf("invalid %s: %q (expected processed|confirmed|finalized)", key, raw)
	}
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(valueForKey(key))
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid %s: must be > 0", key)
	}
	return d, nil
}

func envInt(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(valueForKey(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	if v <= 0 {
		return 0, fmt.Errorf("invalid %s: must be > 0", key)
	}
	return v, nil
}

func envInt64(key string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(valueForKey(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return v, nil
}

func envUint64(key string, fallback uint64) (uint64, error) {
	raw := strings.TrimSpace(valueForKey(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return v, nil
}

func envUint32(key string, fallback uint32) (uint32, error) {
	raw := strings.TrimSpace(valueForKey(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return uint32(v), nil
}

func envOptionalUint(key string) (*uint, error) {
	raw := strings.TrimSpace(valueForKey(key))
	if raw == "" {
		return nil, nil
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", key, err)
	}
	out := uint(v)
	return &out, nil
}

func envBool(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(valueForKey(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("invalid %s: %w", key, err)
	}
	return v, nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(valueForKey(key)); value != "" {
		return value
	}
	return fallback
}

func parseCSVEnv(raw string, fallback []string) []string {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func expandHomePath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return homeDir, nil
		}
		return filepath.Join(homeDir, strings.TrimPrefix(path, "~/")), nil
	}
	return path, nil
}

var (
	runtimeConfigOnce   sync.Once
	runtimeConfigErr    error
	runtimeConfigValues map[string]string
	runtimeConfigLoaded bool
	runtimeConfigPath   string
	runtimeConfigPhase  string
)

func ensureRuntimeConfigLoaded() error {
	runtimeConfigOnce.Do(func() {
		runtimeConfigValues = make(map[string]string)

		phase := strings.TrimSpace(os.Getenv("CONFIG_PHASE"))
		if phase == "" {
			phase = "local"
		}
		runtimeConfigPhase = phase

		configPath := strings.TrimSpace(os.Getenv("CONFIG_FILE"))
		explicitPath := configPath != ""
		if configPath == "" {
			configPath = filepath.Join("config", "config-"+phase+".yaml")
		}

		body, err := os.ReadFile(configPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) && !explicitPath {
				return
			}
			runtimeConfigErr = fmt.Errorf("read config file %q: %w", configPath, err)
			return
		}

		raw := make(map[string]any)
		if err := yaml.Unmarshal(body, &raw); err != nil {
			runtimeConfigErr = fmt.Errorf("parse config file %q: %w", configPath, err)
			return
		}

		flattened, err := flattenConfig(raw)
		if err != nil {
			runtimeConfigErr = fmt.Errorf("flatten config file %q: %w", configPath, err)
			return
		}

		runtimeConfigValues = flattened
		runtimeConfigLoaded = true
		if absPath, err := filepath.Abs(configPath); err == nil {
			runtimeConfigPath = absPath
		} else {
			runtimeConfigPath = configPath
		}
	})
	return runtimeConfigErr
}

func flattenConfig(raw map[string]any) (map[string]string, error) {
	out := make(map[string]string)
	for key, value := range raw {
		segment := normalizeKeySegment(key)
		if segment == "" {
			continue
		}
		if err := flattenConfigValue(segment, value, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func flattenConfigValue(prefix string, value any, out map[string]string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			segment := normalizeKeySegment(key)
			if segment == "" {
				continue
			}
			if err := flattenConfigValue(prefix+"_"+segment, child, out); err != nil {
				return err
			}
		}
		return nil
	case map[any]any:
		for keyAny, child := range typed {
			keyText, ok := keyAny.(string)
			if !ok {
				return fmt.Errorf("unsupported map key type %T under %q", keyAny, prefix)
			}
			segment := normalizeKeySegment(keyText)
			if segment == "" {
				continue
			}
			if err := flattenConfigValue(prefix+"_"+segment, child, out); err != nil {
				return err
			}
		}
		return nil
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			switch scalar := item.(type) {
			case string:
				if strings.TrimSpace(scalar) == "" {
					continue
				}
				parts = append(parts, strings.TrimSpace(scalar))
			case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
				parts = append(parts, fmt.Sprint(scalar))
			default:
				return fmt.Errorf("unsupported list item type %T under %q", item, prefix)
			}
		}
		out[prefix] = strings.Join(parts, ",")
		return nil
	case nil:
		return nil
	default:
		out[prefix] = fmt.Sprint(typed)
		return nil
	}
}

func normalizeKeySegment(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(raw))
	lastUnderscore := false

	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToUpper(r))
			lastUnderscore = false
			continue
		}
		if !lastUnderscore && b.Len() > 0 {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}

	return strings.Trim(b.String(), "_")
}

func valueForKey(key string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}

	if err := ensureRuntimeConfigLoaded(); err != nil {
		return ""
	}

	if value := strings.TrimSpace(runtimeConfigValues[key]); value != "" {
		return value
	}
	return ""
}

func maybeUseLocalSecretKeypair(current string) string {
	expandedCurrent, err := expandHomePath(current)
	if err != nil {
		return current
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return current
	}
	defaultHomePath := filepath.Join(homeDir, ".config", "solana", "id.json")
	if filepath.Clean(expandedCurrent) != filepath.Clean(defaultHomePath) {
		return current
	}

	for _, candidate := range []string{
		"../.local/secret/deployer-wallet.json",
		".local/secret/deployer-wallet.json",
	} {
		absoluteCandidate, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(absoluteCandidate)
		if err != nil {
			continue
		}
		if info.IsDir() {
			continue
		}
		return absoluteCandidate
	}

	return current
}
