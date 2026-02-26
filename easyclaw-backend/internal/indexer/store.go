package indexer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	lpvault "github.com/coldbell/dex/backend/internal/anchor/lp_vault"
	orderengine "github.com/coldbell/dex/backend/internal/anchor/order_engine"
	"github.com/gagliardetto/solana-go"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Store struct {
	db *DB
}

type DB struct {
	raw *sql.DB
}

type Tx struct {
	raw *sql.Tx
}

func (db *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return db.raw.ExecContext(ctx, rebindPostgresPlaceholders(query), args...)
}

func (db *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return db.raw.QueryContext(ctx, rebindPostgresPlaceholders(query), args...)
}

func (db *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return db.raw.QueryRowContext(ctx, rebindPostgresPlaceholders(query), args...)
}

func (db *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	tx, err := db.raw.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{raw: tx}, nil
}

func (db *DB) Close() error {
	return db.raw.Close()
}

func (tx *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return tx.raw.ExecContext(ctx, rebindPostgresPlaceholders(query), args...)
}

func (tx *Tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return tx.raw.QueryContext(ctx, rebindPostgresPlaceholders(query), args...)
}

func (tx *Tx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return tx.raw.QueryRowContext(ctx, rebindPostgresPlaceholders(query), args...)
}

func (tx *Tx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return tx.raw.PrepareContext(ctx, rebindPostgresPlaceholders(query))
}

func (tx *Tx) Commit() error {
	return tx.raw.Commit()
}

func (tx *Tx) Rollback() error {
	return tx.raw.Rollback()
}

func rebindPostgresPlaceholders(query string) string {
	var out strings.Builder
	out.Grow(len(query) + 16)

	arg := 1
	inSingleQuote := false
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if ch == '\'' {
			out.WriteByte(ch)
			if inSingleQuote {
				// SQL escape: two single quotes inside a string literal.
				if i+1 < len(query) && query[i+1] == '\'' {
					out.WriteByte(query[i+1])
					i++
					continue
				}
				inSingleQuote = false
			} else {
				inSingleQuote = true
			}
			continue
		}

		if ch == '?' && !inSingleQuote {
			out.WriteByte('$')
			out.WriteString(strconv.Itoa(arg))
			arg++
			continue
		}

		out.WriteByte(ch)
	}

	return out.String()
}

func NewStore(dbDSN string) (*Store, error) {
	db, err := sql.Open("pgx", dbDSN)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetConnMaxIdleTime(30 * time.Second)
	db.SetMaxIdleConns(4)
	db.SetMaxOpenConns(16)

	pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	store := &Store{db: &DB{raw: db}}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.ensureOrderbookTables(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) WithTx(ctx context.Context, fn func(*Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS sync_state (
			id BIGINT PRIMARY KEY CHECK (id = 1),
			last_slot BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS positions (
			pubkey TEXT PRIMARY KEY,
			user_margin TEXT NOT NULL,
			market_id BIGINT NOT NULL,
			long_qty TEXT NOT NULL,
			long_entry_notional TEXT NOT NULL,
			short_qty TEXT NOT NULL,
			short_entry_notional TEXT NOT NULL,
			last_funding_index_long TEXT NOT NULL,
			last_funding_index_short TEXT NOT NULL,
			raw_json TEXT NOT NULL,
			slot BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_positions_user_market ON positions(user_margin, market_id);`,
		`CREATE TABLE IF NOT EXISTS lp_positions (
			pubkey TEXT PRIMARY KEY,
			owner TEXT NOT NULL,
			pool TEXT NOT NULL,
			shares TEXT NOT NULL,
			pending_shares TEXT NOT NULL,
			withdraw_nonce TEXT NOT NULL,
			raw_json TEXT NOT NULL,
			slot BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_lp_positions_owner_pool ON lp_positions(owner, pool);`,
		`CREATE TABLE IF NOT EXISTS orders (
			pubkey TEXT PRIMARY KEY,
			user_margin TEXT NOT NULL,
			user_pubkey TEXT NOT NULL,
			market_id BIGINT NOT NULL,
			side TEXT NOT NULL,
			order_type TEXT NOT NULL,
			reduce_only INTEGER NOT NULL,
			margin TEXT NOT NULL,
			price TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			client_order_id TEXT NOT NULL,
			raw_json TEXT NOT NULL,
			slot BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_orders_market_status ON orders(market_id, status);`,
		`CREATE TABLE IF NOT EXISTS fills (
			id BIGSERIAL PRIMARY KEY,
			order_pubkey TEXT NOT NULL UNIQUE,
			user_margin TEXT NOT NULL,
			user_pubkey TEXT NOT NULL,
			market_id BIGINT NOT NULL,
			side TEXT NOT NULL,
			order_type TEXT NOT NULL,
			reduce_only INTEGER NOT NULL,
			margin TEXT NOT NULL,
			price TEXT NOT NULL,
			client_order_id TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			executed_slot BIGINT NOT NULL,
			executed_at BIGINT NOT NULL,
			raw_json TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_fills_user_market_time ON fills(user_margin, market_id, executed_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_fills_user_pubkey_time ON fills(user_pubkey, executed_at DESC);`,
		`CREATE TABLE IF NOT EXISTS position_history (
			id BIGSERIAL PRIMARY KEY,
			position_pubkey TEXT NOT NULL,
			user_margin TEXT NOT NULL,
			market_id BIGINT NOT NULL,
			event_type TEXT NOT NULL,
			prev_long_qty TEXT NOT NULL,
			prev_long_entry_notional TEXT NOT NULL,
			prev_short_qty TEXT NOT NULL,
			prev_short_entry_notional TEXT NOT NULL,
			prev_last_funding_index_long TEXT NOT NULL,
			prev_last_funding_index_short TEXT NOT NULL,
			next_long_qty TEXT NOT NULL,
			next_long_entry_notional TEXT NOT NULL,
			next_short_qty TEXT NOT NULL,
			next_short_entry_notional TEXT NOT NULL,
			next_last_funding_index_long TEXT NOT NULL,
			next_last_funding_index_short TEXT NOT NULL,
			slot BIGINT NOT NULL,
			recorded_at BIGINT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_position_history_user_market_time ON position_history(user_margin, market_id, recorded_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_position_history_position_slot ON position_history(position_pubkey, slot DESC);`,
		`INSERT INTO position_history (
			position_pubkey, user_margin, market_id, event_type,
			prev_long_qty, prev_long_entry_notional, prev_short_qty, prev_short_entry_notional,
			prev_last_funding_index_long, prev_last_funding_index_short,
			next_long_qty, next_long_entry_notional, next_short_qty, next_short_entry_notional,
			next_last_funding_index_long, next_last_funding_index_short,
			slot, recorded_at
		)
		SELECT
			p.pubkey, p.user_margin, p.market_id, 'snapshot',
			p.long_qty, p.long_entry_notional, p.short_qty, p.short_entry_notional,
			p.last_funding_index_long, p.last_funding_index_short,
			p.long_qty, p.long_entry_notional, p.short_qty, p.short_entry_notional,
			p.last_funding_index_long, p.last_funding_index_short,
			p.slot, p.updated_at
		FROM positions p
		WHERE NOT EXISTS (
			SELECT 1 FROM position_history h WHERE h.position_pubkey = p.pubkey
		);`,
		`CREATE TABLE IF NOT EXISTS resources (
			pubkey TEXT PRIMARY KEY,
			program_id TEXT NOT NULL,
			account_type TEXT NOT NULL,
			owner TEXT NOT NULL,
			lamports BIGINT NOT NULL,
			raw_json TEXT NOT NULL,
			slot BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_resources_program_type ON resources(program_id, account_type);`,
		`CREATE TABLE IF NOT EXISTS market_price_ticks (
			id BIGSERIAL PRIMARY KEY,
			market TEXT NOT NULL,
			source TEXT NOT NULL,
			feed_id TEXT NOT NULL,
			slot BIGINT NOT NULL,
			publish_time BIGINT NOT NULL,
			price DOUBLE PRECISION NOT NULL,
			conf DOUBLE PRECISION NOT NULL,
			expo INTEGER NOT NULL,
			received_at BIGINT NOT NULL,
			raw_json TEXT NOT NULL
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_market_price_ticks_dedupe ON market_price_ticks(market, source, publish_time, slot);`,
		`CREATE INDEX IF NOT EXISTS idx_market_price_ticks_market_time ON market_price_ticks(market, publish_time DESC, slot DESC, id DESC);`,
		`CREATE TABLE IF NOT EXISTS auth_challenges (
			id TEXT PRIMARY KEY,
			wallet_pubkey TEXT NOT NULL,
			intent TEXT NOT NULL,
			message TEXT NOT NULL,
			created_at BIGINT NOT NULL,
			expires_at BIGINT NOT NULL,
			used_at BIGINT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_auth_challenges_wallet_created ON auth_challenges(wallet_pubkey, created_at DESC);`,
		`CREATE TABLE IF NOT EXISTS auth_sessions (
			token_hash TEXT PRIMARY KEY,
			wallet_pubkey TEXT NOT NULL,
			created_at BIGINT NOT NULL,
			expires_at BIGINT NOT NULL,
			refreshed_at BIGINT NOT NULL,
			revoked_at BIGINT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_auth_sessions_wallet_exp ON auth_sessions(wallet_pubkey, expires_at DESC);`,
		`CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			strategy_id TEXT NOT NULL,
			status TEXT NOT NULL,
			owner_pubkey TEXT NOT NULL,
			risk_profile_json TEXT NOT NULL,
			bound_at BIGINT,
			session_expires_at BIGINT,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_agents_owner ON agents(owner_pubkey);`,
		`CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);`,
		`CREATE TABLE IF NOT EXISTS agent_sessions (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
			mode TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at BIGINT NOT NULL,
			stopped_at BIGINT,
			created_by TEXT NOT NULL,
			created_at BIGINT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_sessions_agent_status ON agent_sessions(agent_id, status, started_at DESC);`,
		`CREATE TABLE IF NOT EXISTS strategy_templates (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL,
			risk_level TEXT NOT NULL,
			rules_json TEXT NOT NULL,
			created_at BIGINT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS strategies (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			entry_rules_json TEXT NOT NULL,
			exit_rules_json TEXT NOT NULL,
			risk_defaults_json TEXT NOT NULL,
			owner_pubkey TEXT NOT NULL,
			is_published INTEGER NOT NULL DEFAULT 0,
			published_at BIGINT,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_strategies_owner ON strategies(owner_pubkey, updated_at DESC);`,
	}

	for _, query := range ddl {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	if err := s.renameLegacyOrderColumns(ctx); err != nil {
		return err
	}

	if err := s.backfillExecutedFills(ctx); err != nil {
		return err
	}
	if err := s.seedStrategyTemplates(ctx); err != nil {
		return err
	}

	return nil
}

func (s *Store) seedStrategyTemplates(ctx context.Context) error {
	now := time.Now().Unix()
	templates := []struct {
		id          string
		name        string
		description string
		riskLevel   string
		rules       string
	}{
		{
			id:          "preset-momentum",
			name:        "Momentum Trend",
			description: "RSI/MACD based trend follow entries with ATR stop.",
			riskLevel:   "medium",
			rules:       `{"entry":{"indicators":["rsi","macd"],"condition":"trend_follow"},"exit":{"stop":"atr","take_profit":"dynamic"},"timeframe":"15m"}`,
		},
		{
			id:          "preset-mean-reversion",
			name:        "Mean Reversion",
			description: "Bollinger band deviation entries with mid-band exits.",
			riskLevel:   "low",
			rules:       `{"entry":{"indicator":"bollinger","condition":"band_deviation"},"exit":{"target":"middle_band","stop":"fixed"},"timeframe":"5m"}`,
		},
		{
			id:          "preset-breakout",
			name:        "Volatility Breakout",
			description: "Breakout continuation with volume confirmation.",
			riskLevel:   "high",
			rules:       `{"entry":{"indicator":"bollinger","condition":"breakout","confirm":"volume"},"exit":{"stop":"trailing","take_profit":"rr_2"},"timeframe":"1h"}`,
		},
	}

	for _, template := range templates {
		if _, err := s.db.ExecContext(
			ctx,
			`INSERT INTO strategy_templates (id, name, description, risk_level, rules_json, created_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(id) DO UPDATE SET
			   name = excluded.name,
			   description = excluded.description,
			   risk_level = excluded.risk_level,
			   rules_json = excluded.rules_json`,
			template.id,
			template.name,
			template.description,
			template.riskLevel,
			template.rules,
			now,
		); err != nil {
			return fmt.Errorf("seed strategy template %s: %w", template.id, err)
		}
	}
	return nil
}

func (s *Store) renameLegacyOrderColumns(ctx context.Context) error {
	renames := []struct {
		table string
		from  string
		to    string
	}{
		{table: "orders", from: "qty", to: "margin"},
		{table: "orders", from: "limit_price", to: "price"},
		{table: "fills", from: "qty", to: "margin"},
		{table: "fills", from: "limit_price", to: "price"},
	}

	for _, rename := range renames {
		hasFrom, err := s.hasColumn(ctx, rename.table, rename.from)
		if err != nil {
			return err
		}
		if !hasFrom {
			continue
		}

		hasTo, err := s.hasColumn(ctx, rename.table, rename.to)
		if err != nil {
			return err
		}
		if hasTo {
			continue
		}

		query := fmt.Sprintf(
			"ALTER TABLE %s RENAME COLUMN %s TO %s",
			rename.table,
			rename.from,
			rename.to,
		)
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf(
				"rename legacy column %s.%s to %s: %w",
				rename.table,
				rename.from,
				rename.to,
				err,
			)
		}
	}

	return nil
}

func (s *Store) hasColumn(ctx context.Context, table, column string) (bool, error) {
	row := s.db.QueryRowContext(
		ctx,
		`
		SELECT 1
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = ?
		  AND column_name = ?
		LIMIT 1
		`,
		table,
		column,
	)
	var one int
	err := row.Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return one == 1, nil
}

func (s *Store) backfillExecutedFills(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO fills (
			order_pubkey, user_margin, user_pubkey, market_id, side, order_type,
			reduce_only, margin, price, client_order_id, created_at, expires_at,
			executed_slot, executed_at, raw_json
		)
		SELECT
			o.pubkey, o.user_margin, o.user_pubkey, o.market_id, o.side, o.order_type,
			o.reduce_only, o.margin, o.price, o.client_order_id, o.created_at, o.expires_at,
			o.slot, o.updated_at, o.raw_json
		FROM orders o
		WHERE o.status = 'Executed'
		  AND NOT EXISTS (
			SELECT 1 FROM fills f WHERE f.order_pubkey = o.pubkey
		  )
	`)
	if err != nil {
		return fmt.Errorf("backfill executed fills: %w", err)
	}

	return nil
}

func (s *Store) UpsertSyncStateTx(ctx context.Context, tx *Tx, slot uint64) error {
	now := time.Now().Unix()
	_, err := tx.ExecContext(ctx, `
		INSERT INTO sync_state (id, last_slot, updated_at)
		VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			last_slot = excluded.last_slot,
			updated_at = excluded.updated_at
	`, int64(slot), now)
	return err
}

func (s *Store) UpsertPositionTx(ctx context.Context, tx *Tx, pubkey solana.PublicKey, slot uint64, position *orderengine.UserMarketPosition) error {
	raw, err := json.Marshal(position)
	if err != nil {
		return err
	}

	pubkeyText := pubkey.String()
	nextSnapshot := positionHistorySnapshot{
		LongQty:               strconv.FormatUint(position.LongQty, 10),
		LongEntryNotional:     position.LongEntryNotional.String(),
		ShortQty:              strconv.FormatUint(position.ShortQty, 10),
		ShortEntryNotional:    position.ShortEntryNotional.String(),
		LastFundingIndexLong:  position.LastFundingIndexLong.String(),
		LastFundingIndexShort: position.LastFundingIndexShort.String(),
	}
	prevSnapshot, err := s.getPositionHistorySnapshotTx(ctx, tx, pubkeyText)
	if err != nil {
		return err
	}

	now := time.Now().Unix()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO positions (
			pubkey, user_margin, market_id, long_qty, long_entry_notional, short_qty,
			short_entry_notional, last_funding_index_long, last_funding_index_short,
			raw_json, slot, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(pubkey) DO UPDATE SET
			user_margin = excluded.user_margin,
			market_id = excluded.market_id,
			long_qty = excluded.long_qty,
			long_entry_notional = excluded.long_entry_notional,
			short_qty = excluded.short_qty,
			short_entry_notional = excluded.short_entry_notional,
			last_funding_index_long = excluded.last_funding_index_long,
			last_funding_index_short = excluded.last_funding_index_short,
			raw_json = excluded.raw_json,
			slot = excluded.slot,
			updated_at = excluded.updated_at
	`,
		pubkeyText,
		position.UserMargin.String(),
		int64(position.MarketId),
		nextSnapshot.LongQty,
		nextSnapshot.LongEntryNotional,
		nextSnapshot.ShortQty,
		nextSnapshot.ShortEntryNotional,
		nextSnapshot.LastFundingIndexLong,
		nextSnapshot.LastFundingIndexShort,
		string(raw),
		int64(slot),
		now,
	)
	if err != nil {
		return err
	}

	if prevSnapshot == nil {
		return s.insertPositionHistoryTx(
			ctx,
			tx,
			pubkeyText,
			position.UserMargin.String(),
			position.MarketId,
			"snapshot",
			zeroPositionHistorySnapshot(),
			nextSnapshot,
			slot,
			now,
		)
	}
	if snapshotsEqual(*prevSnapshot, nextSnapshot) {
		return nil
	}

	return s.insertPositionHistoryTx(
		ctx,
		tx,
		pubkeyText,
		position.UserMargin.String(),
		position.MarketId,
		"update",
		*prevSnapshot,
		nextSnapshot,
		slot,
		now,
	)
}

func (s *Store) UpsertLPPositionTx(ctx context.Context, tx *Tx, pubkey solana.PublicKey, slot uint64, position *lpvault.LpPosition) error {
	raw, err := json.Marshal(position)
	if err != nil {
		return err
	}
	now := time.Now().Unix()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO lp_positions (
			pubkey, owner, pool, shares, pending_shares, withdraw_nonce,
			raw_json, slot, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(pubkey) DO UPDATE SET
			owner = excluded.owner,
			pool = excluded.pool,
			shares = excluded.shares,
			pending_shares = excluded.pending_shares,
			withdraw_nonce = excluded.withdraw_nonce,
			raw_json = excluded.raw_json,
			slot = excluded.slot,
			updated_at = excluded.updated_at
	`,
		pubkey.String(),
		position.Owner.String(),
		position.Pool.String(),
		position.Shares.String(),
		position.PendingShares.String(),
		strconv.FormatUint(position.WithdrawNonce, 10),
		string(raw),
		int64(slot),
		now,
	)
	return err
}

func (s *Store) UpsertOrderTx(ctx context.Context, tx *Tx, pubkey solana.PublicKey, slot uint64, order *orderengine.Order) error {
	raw, err := json.Marshal(order)
	if err != nil {
		return err
	}

	pubkeyText := pubkey.String()
	prevOrderStatus, err := s.getOrderStatusTx(ctx, tx, pubkeyText)
	if err != nil {
		return err
	}

	now := time.Now().Unix()
	orderStatus := order.Status.String()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO orders (
			pubkey, user_margin, user_pubkey, market_id, side, order_type,
			reduce_only, margin, price, status, created_at, expires_at,
			client_order_id, raw_json, slot, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(pubkey) DO UPDATE SET
			user_margin = excluded.user_margin,
			user_pubkey = excluded.user_pubkey,
			market_id = excluded.market_id,
			side = excluded.side,
			order_type = excluded.order_type,
			reduce_only = excluded.reduce_only,
			margin = excluded.margin,
			price = excluded.price,
			status = excluded.status,
			created_at = excluded.created_at,
			expires_at = excluded.expires_at,
			client_order_id = excluded.client_order_id,
			raw_json = excluded.raw_json,
			slot = excluded.slot,
			updated_at = excluded.updated_at
	`,
		pubkeyText,
		order.UserMargin.String(),
		order.User.String(),
		int64(order.MarketId),
		order.Side.String(),
		order.OrderType.String(),
		boolToInt(order.ReduceOnly),
		strconv.FormatUint(order.Margin, 10),
		strconv.FormatUint(order.Price, 10),
		orderStatus,
		order.CreatedAt,
		order.ExpiresAt,
		strconv.FormatUint(order.ClientOrderId, 10),
		string(raw),
		int64(slot),
		now,
	)
	if err != nil {
		return err
	}

	if orderStatus != orderengine.OrderStatus_Executed.String() {
		return nil
	}
	if prevOrderStatus != nil && *prevOrderStatus == orderStatus {
		return nil
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO fills (
			order_pubkey, user_margin, user_pubkey, market_id, side, order_type,
			reduce_only, margin, price, client_order_id, created_at, expires_at,
			executed_slot, executed_at, raw_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(order_pubkey) DO NOTHING
	`,
		pubkeyText,
		order.UserMargin.String(),
		order.User.String(),
		int64(order.MarketId),
		order.Side.String(),
		order.OrderType.String(),
		boolToInt(order.ReduceOnly),
		strconv.FormatUint(order.Margin, 10),
		strconv.FormatUint(order.Price, 10),
		strconv.FormatUint(order.ClientOrderId, 10),
		order.CreatedAt,
		order.ExpiresAt,
		int64(slot),
		now,
		string(raw),
	)
	return err
}

func (s *Store) UpsertResourceTx(
	ctx context.Context,
	tx *Tx,
	pubkey solana.PublicKey,
	programID solana.PublicKey,
	accountType string,
	owner solana.PublicKey,
	lamports uint64,
	slot uint64,
	payload any,
) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	now := time.Now().Unix()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO resources (
			pubkey, program_id, account_type, owner, lamports, raw_json, slot, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(pubkey) DO UPDATE SET
			program_id = excluded.program_id,
			account_type = excluded.account_type,
			owner = excluded.owner,
			lamports = excluded.lamports,
			raw_json = excluded.raw_json,
			slot = excluded.slot,
			updated_at = excluded.updated_at
	`,
		pubkey.String(),
		programID.String(),
		accountType,
		owner.String(),
		int64(lamports),
		string(raw),
		int64(slot),
		now,
	)
	return err
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

type positionHistorySnapshot struct {
	LongQty               string
	LongEntryNotional     string
	ShortQty              string
	ShortEntryNotional    string
	LastFundingIndexLong  string
	LastFundingIndexShort string
}

func zeroPositionHistorySnapshot() positionHistorySnapshot {
	return positionHistorySnapshot{
		LongQty:               "0",
		LongEntryNotional:     "0",
		ShortQty:              "0",
		ShortEntryNotional:    "0",
		LastFundingIndexLong:  "0",
		LastFundingIndexShort: "0",
	}
}

func snapshotsEqual(left, right positionHistorySnapshot) bool {
	return left.LongQty == right.LongQty &&
		left.LongEntryNotional == right.LongEntryNotional &&
		left.ShortQty == right.ShortQty &&
		left.ShortEntryNotional == right.ShortEntryNotional &&
		left.LastFundingIndexLong == right.LastFundingIndexLong &&
		left.LastFundingIndexShort == right.LastFundingIndexShort
}

func (s *Store) getPositionHistorySnapshotTx(ctx context.Context, tx *Tx, pubkey string) (*positionHistorySnapshot, error) {
	row := tx.QueryRowContext(
		ctx,
		`SELECT long_qty, long_entry_notional, short_qty, short_entry_notional, last_funding_index_long, last_funding_index_short
		 FROM positions
		 WHERE pubkey = ?`,
		pubkey,
	)

	var snapshot positionHistorySnapshot
	err := row.Scan(
		&snapshot.LongQty,
		&snapshot.LongEntryNotional,
		&snapshot.ShortQty,
		&snapshot.ShortEntryNotional,
		&snapshot.LastFundingIndexLong,
		&snapshot.LastFundingIndexShort,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func (s *Store) insertPositionHistoryTx(
	ctx context.Context,
	tx *Tx,
	positionPubkey string,
	userMargin string,
	marketID uint64,
	eventType string,
	prev positionHistorySnapshot,
	next positionHistorySnapshot,
	slot uint64,
	recordedAt int64,
) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO position_history (
			position_pubkey, user_margin, market_id, event_type,
			prev_long_qty, prev_long_entry_notional, prev_short_qty, prev_short_entry_notional,
			prev_last_funding_index_long, prev_last_funding_index_short,
			next_long_qty, next_long_entry_notional, next_short_qty, next_short_entry_notional,
			next_last_funding_index_long, next_last_funding_index_short,
			slot, recorded_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		positionPubkey,
		userMargin,
		int64(marketID),
		eventType,
		prev.LongQty,
		prev.LongEntryNotional,
		prev.ShortQty,
		prev.ShortEntryNotional,
		prev.LastFundingIndexLong,
		prev.LastFundingIndexShort,
		next.LongQty,
		next.LongEntryNotional,
		next.ShortQty,
		next.ShortEntryNotional,
		next.LastFundingIndexLong,
		next.LastFundingIndexShort,
		int64(slot),
		recordedAt,
	)
	return err
}

func (s *Store) getOrderStatusTx(ctx context.Context, tx *Tx, pubkey string) (*string, error) {
	row := tx.QueryRowContext(ctx, `SELECT status FROM orders WHERE pubkey = ?`, pubkey)
	var status string
	err := row.Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &status, nil
}
