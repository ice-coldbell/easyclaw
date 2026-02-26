# DEX Backend (Go)

`easyclaw-backend` contains three Go processes:

- `keeper`: watches open orders and submits `execute_order` transactions.
- `indexer`: indexes positions/orders/resources into PostgreSQL.
- `api-server`: serves indexed data for frontend consumers.

## Components

- `cmd/keeper`: keeper process entrypoint
- `cmd/indexer`: indexer process entrypoint
- `cmd/api-server`: HTTP API server entrypoint
- `internal/anchor/order_engine`: generated from `easyclaw-contract/target/idl/order_engine.json`
- `internal/anchor/lp_vault`: generated from `easyclaw-contract/target/idl/lp_vault.json`
- `internal/keeper`: order execution loop
- `internal/indexer`: state indexing loop + PostgreSQL store
- `internal/apiserver`: REST handlers for indexed data

## Prerequisites

- Go 1.26.0
- Solana RPC endpoint
- Keeper wallet keypair file (for `keeper`)

## Configuration (`config-{phase}.yaml`)

All services load configuration in this order:

1. Environment variable
2. YAML config file
3. Built-in default

Default config file path:

- `config/config-${CONFIG_PHASE}.yaml`
- `CONFIG_PHASE` default: `local`

Optional override:

- `CONFIG_FILE=/absolute/or/relative/path/to/config.yaml`

Included examples:

- `config/config-local.yaml`
- `config/config-docker.yaml`

## Run Locally

`cmd/indexer`, `cmd/keeper`, and `cmd/api-server` import `godotenv/autoload`, so local `.env` is still supported.

```bash
cd easyclaw-backend

# build
go build ./...

# defaults to CONFIG_PHASE=local -> config/config-local.yaml
go run ./cmd/indexer
go run ./cmd/api-server
go run ./cmd/keeper
```

If you want a different phase/file:

```bash
CONFIG_PHASE=docker go run ./cmd/api-server
# or
CONFIG_FILE=./config/config-docker.yaml go run ./cmd/api-server
```

## Logging Behavior

- Local (`config-local.yaml`): logs to console + `.local/log/{process}.log`
- Docker (`config-docker.yaml`): logs to files under `.docker/{service}/{service}.log`

Supported log keys (global + service override):

- `LOG_LEVEL`, `LOG_FORMAT`, `LOG_OUTPUT`, `LOG_FILE`
- `API_SERVER_LOG_LEVEL`, `API_SERVER_LOG_FORMAT`, `API_SERVER_LOG_OUTPUT`, `API_SERVER_LOG_FILE`
- `INDEXER_LOG_LEVEL`, `INDEXER_LOG_FORMAT`, `INDEXER_LOG_OUTPUT`, `INDEXER_LOG_FILE`
- `KEEPER_LOG_LEVEL`, `KEEPER_LOG_FORMAT`, `KEEPER_LOG_OUTPUT`, `KEEPER_LOG_FILE`

`LOG_OUTPUT` values: `console|file|both`

## Docker Compose Split (`infra` / `app`)

- `docker-compose.infra.yml`: `postgres`, `redis`
- `docker-compose.yml`: `api-server`, `indexer`, `keeper`

Run infra first:

```bash
cd easyclaw-backend
docker compose -f docker-compose.infra.yml up -d
```

Then run app services:

```bash
docker compose up --build -d
```

Or run all in one command:

```bash
docker compose -f docker-compose.infra.yml -f docker-compose.yml up --build -d
```

Important:

- `keeper` mounts your host keypair from `${HOME}/.config/solana/id.json`
- Docker config assumes Solana RPC is reachable at `http://host.docker.internal:8899`

Handy commands:

```bash
docker compose logs -f api-server indexer keeper
cat .docker/api-server/api-server.log
cat .docker/indexer/indexer.log
cat .docker/keeper/keeper.log
```

## Key Environment Variables

Shared:

- `SOLANA_RPC_URL` (default: `http://127.0.0.1:8899`)
- `SOLANA_COMMITMENT` (`processed|confirmed|finalized`, default: `confirmed`)
- `ORDER_ENGINE_PROGRAM_ID` (default: `GpMobZUKPtEE1eiZQAADo2ecD54JXhNHPNts5kPGwLtb`)
- `MARKET_REGISTRY_PROGRAM_ID` (default: `BsA8fuyw8XqBMiUfpLbdiBwbKg8MZMHB1jdZzjs7c46q`)
- `LP_VAULT_PROGRAM_ID` (default: `F8gkLV5nMaCG16PQAwkKKsTdWC2yuPektUXAFHQF4Cds`)

Keeper:

- `KEEPER_KEYPAIR_PATH` (default: `~/.config/solana/id.json`)
- If `KEEPER_KEYPAIR_PATH` resolves to default `~/.config/solana/id.json` and `../.local/secret/deployer-wallet.json` exists, backend prefers that localnet keypair automatically.
- `KEEPER_POLL_INTERVAL` (default: `1500ms`)
- `KEEPER_MAX_ORDERS_PER_TICK` (default: `10`)
- `KEEPER_TX_TIMEOUT` (default: `30s`)
- `KEEPER_SKIP_PREFLIGHT` (default: `false`)
- `KEEPER_MAX_RETRIES` (optional)
- `KEEPER_COMPUTE_UNIT_LIMIT` (default: `0`, disabled)
- `KEEPER_COMPUTE_UNIT_PRICE_MICRO_LAMPORTS` (default: `0`, disabled)
- `KEEPER_PYTH_PRICE_UPDATE_ACCOUNTS_JSON` market-id keyed Pyth `PriceUpdateV2` account map
- `KEEPER_ORACLE_PRICES_JSON` legacy option
- `KEEPER_DEFAULT_ORACLE_PRICE` legacy option
- `KEEPER_DEFAULT_ORACLE_CONF_BPS` legacy option
- `KEEPER_DEFAULT_ORACLE_PUBLISH_LAG_SEC` legacy option

Indexer:

- `INDEXER_DB_DSN` (default: `postgres://postgres:postgres@127.0.0.1:5432/dex?sslmode=disable`)
- `INDEXER_POLL_INTERVAL` (default: `2s`)
- `INDEXER_ORDERBOOK_SNAPSHOT_INTERVAL` (default: `1m`, minute-aligned)
- `INDEXER_ORDERBOOK_REFRESH_INTERVAL` (default: `1s`)
- `INDEXER_ORDERBOOK_TARGETS` (default: `binance:BTCUSDT,okx:BTC-USDT,coinbase:BTC-USD,bybit:BTCUSDT`)
- `INDEXER_ORDERBOOK_DEPTH` (default: `100`)
- `INDEXER_ORDERBOOK_REQUEST_TIMEOUT` (default: `10s`)
- `INDEXER_ENABLE_PYTH_PRICE_STREAM` (default: `true`)
- `INDEXER_PYTH_STREAM_URL` (default: `https://hermes.pyth.network/v2/updates/price/stream`)
- `INDEXER_PYTH_FEED_ID` (default: `e62df6c8b4a85fe1a67db44dc12de5db330f7ac66b72dc658afedf0f4a415b43`, BTC/USD)
- `INDEXER_PYTH_MARKET` (default: `BTCUSDT`)
- `INDEXER_PYTH_RECONNECT_INTERVAL` (default: `3s`)

API server:

- `API_SERVER_LISTEN_ADDR` (default: `:8080`)
- `API_SERVER_DB_DSN` (default: `INDEXER_DB_DSN`)
- `API_SERVER_ALLOWED_ORIGINS` (default: `*`, comma-separated list supported)
- `API_SERVER_READ_TIMEOUT` (default: `10s`)
- `API_SERVER_WRITE_TIMEOUT` (default: `15s`)
- `API_SERVER_IDLE_TIMEOUT` (default: `60s`)

Note:

- Redis is in the infra compose for completeness (`redis://redis:6379/0` in docker config).
- Current backend code path does not require Redis for startup yet.

## API Endpoints

- `GET /healthz`
- `GET /api/v1/positions?user_margin=&market_id=&limit=&offset=`
- `GET /api/v1/orders?user_margin=&user_pubkey=&market_id=&status=&limit=&offset=`
- `GET /api/v1/fills?user_margin=&user_pubkey=&market_id=&limit=&offset=`
- `GET /api/v1/position-history?user_margin=&market_id=&limit=&offset=`
- `GET /api/v1/orderbook-heatmap?exchange=&symbol=&from=&to=&limit=&offset=`
- `GET /api/v1/orderbook-heatmap-aggregated?symbol_key=&from=&to=&limit=&offset=`

- `POST /v1/auth/challenge`
- `POST /v1/auth/verify-signature`
- `POST /v1/auth/session/refresh`

- `GET /v1/agents`
- `POST /v1/agents`
- `GET /v1/agents/:agentId`
- `GET /v1/agents/:agentId/owner-binding`
- `POST /v1/agents/:agentId/owner-binding/rebind`
- `POST /v1/agents/:agentId/sessions`
- `DELETE /v1/agents/:agentId/sessions/:sessionId`
- `GET /v1/agents/:agentId/risk`
- `PATCH /v1/agents/:agentId/risk`
- `POST /v1/safety/kill-switch`

- `GET /v1/strategy/templates`
- `POST /v1/strategies`
- `GET /v1/strategies/:strategyId`
- `PATCH /v1/strategies/:strategyId`
- `POST /v1/strategies/:strategyId/publish`

- `GET /v1/portfolio?period=7d|30d|all`
- `GET /v1/portfolio/agents/:agentId?period=7d|30d|all`
- `GET /v1/chart/candles?market=BTCUSDT&timeframe=1m|5m|15m|1h|4h|1d&limit=120`
- `GET /v1/trades?agent_id=&from=&to=&limit=&offset=`
- `GET /v1/leaderboard?metric=win_rate|pnl_pct&period=all_time|30d|7d&min_trades=20`
- `GET /v1/system/status`

- `GET /ws` (websocket, subscribe message: `{"type":"subscribe","channel":"..."}`)
  - realtime price: `market.price.BTCUSDT`
  - candles snapshot compatibility channel: `chart.ticks.BTCUSDT`

## Regenerate go-anchor clients

```bash
cd easyclaw-backend
./scripts/generate-anchor-clients.sh ../easyclaw-contract/target/idl
```

## PostgreSQL Tables

- `positions`: `UserMarketPosition`
- `lp_positions`: `LpPosition`
- `orders`: `Order`
- `fills`: executed-order based fill history
- `position_history`: position snapshot change history
- `resources`: engine/market/keeper/vault related resources
- `sync_state`: last synced slot
- `exchange_orderbook_snapshots`: exchange orderbook snapshots by minute (`levels_json` 포함)
- `market_price_ticks`: Pyth stream price ticks (BTC 등) for chart/WS payloads
