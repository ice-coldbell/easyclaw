package indexer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	orderbookSideBid = "bid"
	orderbookSideAsk = "ask"
)

type OrderbookLevel struct {
	Side     string `json:"side"`
	Level    int    `json:"level"`
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
}

type OrderbookSnapshot struct {
	ID                int64
	Exchange          string
	Symbol            string
	SnapshotTime      int64
	ExchangeTimestamp int64
	BestBid           string
	BestAsk           string
	RawJSON           string
	Levels            []OrderbookLevel
}

type OrderbookHeatmapFilter struct {
	Exchange string
	Symbol   string
	FromUnix int64
	ToUnix   int64
	Limit    int
	Offset   int
}

type OrderbookHeatmapRecord struct {
	ID                int64                   `json:"id"`
	Exchange          string                  `json:"exchange"`
	Symbol            string                  `json:"symbol"`
	SnapshotTime      int64                   `json:"snapshot_time"`
	ExchangeTimestamp int64                   `json:"exchange_timestamp"`
	BestBid           string                  `json:"best_bid"`
	BestAsk           string                  `json:"best_ask"`
	Levels            []OrderbookHeatmapLevel `json:"levels"`
}

type OrderbookHeatmapLevel struct {
	Side     string `json:"side"`
	Level    int    `json:"level"`
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
}

type OrderbookHeatmapAggregateFilter struct {
	SymbolKey string
	FromUnix  int64
	ToUnix    int64
	Limit     int
	Offset    int
}

type OrderbookHeatmapAggregatePoint struct {
	SnapshotTime int64  `json:"snapshot_time"`
	Exchange     string `json:"exchange"`
	Symbol       string `json:"symbol"`
	Side         string `json:"side"`
	Price        string `json:"price"`
	Quantity     string `json:"quantity"`
}

func (s *Store) ensureOrderbookTables(ctx context.Context) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS exchange_orderbook_snapshots (
			id BIGSERIAL PRIMARY KEY,
			exchange TEXT NOT NULL,
			symbol TEXT NOT NULL,
			snapshot_time BIGINT NOT NULL,
			exchange_ts BIGINT NOT NULL,
			best_bid TEXT NOT NULL,
			best_ask TEXT NOT NULL,
			raw_json TEXT NOT NULL,
			levels_json TEXT NOT NULL DEFAULT '[]',
			created_at BIGINT NOT NULL,
			UNIQUE(exchange, symbol, snapshot_time)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_orderbook_snapshots_lookup
			ON exchange_orderbook_snapshots(exchange, symbol, snapshot_time DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_orderbook_snapshots_unique
			ON exchange_orderbook_snapshots(exchange, symbol, snapshot_time DESC);`,
	}

	for _, query := range ddl {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("orderbook migration failed: %w", err)
		}
	}

	hasLevelsJSON, err := s.hasColumn(ctx, "exchange_orderbook_snapshots", "levels_json")
	if err != nil {
		return fmt.Errorf("check orderbook levels_json column: %w", err)
	}
	if !hasLevelsJSON {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE exchange_orderbook_snapshots ADD COLUMN levels_json TEXT NOT NULL DEFAULT '[]'`); err != nil {
			return fmt.Errorf("add levels_json column: %w", err)
		}
	}
	if err := s.backfillLegacyOrderbookLevels(ctx); err != nil {
		return err
	}

	return nil
}

func (s *Store) UpsertOrderbookSnapshotTx(ctx context.Context, tx *Tx, snapshot OrderbookSnapshot) (int64, error) {
	levels := snapshot.Levels
	if levels == nil {
		levels = make([]OrderbookLevel, 0)
	}
	levelsJSON, err := json.Marshal(levels)
	if err != nil {
		return 0, fmt.Errorf("marshal orderbook levels: %w", err)
	}

	now := nowUnix()
	var snapshotID int64
	row := tx.QueryRowContext(
		ctx,
		`
		INSERT INTO exchange_orderbook_snapshots (
			exchange, symbol, snapshot_time, exchange_ts,
			best_bid, best_ask, raw_json, levels_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(exchange, symbol, snapshot_time) DO UPDATE SET
			exchange_ts = excluded.exchange_ts,
			best_bid = excluded.best_bid,
			best_ask = excluded.best_ask,
			raw_json = excluded.raw_json,
			levels_json = excluded.levels_json,
			created_at = excluded.created_at
		RETURNING id
		`,
		snapshot.Exchange,
		snapshot.Symbol,
		snapshot.SnapshotTime,
		snapshot.ExchangeTimestamp,
		snapshot.BestBid,
		snapshot.BestAsk,
		snapshot.RawJSON,
		string(levelsJSON),
		now,
	)
	if err := row.Scan(&snapshotID); err != nil {
		return 0, err
	}

	return snapshotID, nil
}

func (s *Store) ListOrderbookHeatmap(
	ctx context.Context,
	filter OrderbookHeatmapFilter,
) ([]OrderbookHeatmapRecord, int, int, error) {
	limit, offset := normalizePagination(filter.Limit, filter.Offset)

	clauses := []string{"1 = 1"}
	args := make([]any, 0, 6)

	if filter.Exchange != "" {
		clauses = append(clauses, "exchange = ?")
		args = append(args, filter.Exchange)
	}
	if filter.Symbol != "" {
		clauses = append(clauses, "symbol = ?")
		args = append(args, filter.Symbol)
	}
	if filter.FromUnix > 0 {
		clauses = append(clauses, "snapshot_time >= ?")
		args = append(args, filter.FromUnix)
	}
	if filter.ToUnix > 0 {
		clauses = append(clauses, "snapshot_time <= ?")
		args = append(args, filter.ToUnix)
	}

	rows, err := s.db.QueryContext(
		ctx,
		`
		SELECT
			id, exchange, symbol, snapshot_time, exchange_ts, best_bid, best_ask, levels_json
		FROM exchange_orderbook_snapshots
		WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY snapshot_time DESC
		LIMIT ? OFFSET ?
		`,
		append(append(args, limit), offset)...,
	)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()

	snapshots := make([]OrderbookHeatmapRecord, 0, limit)
	for rows.Next() {
		var item OrderbookHeatmapRecord
		var levelsJSON string
		if err := rows.Scan(
			&item.ID,
			&item.Exchange,
			&item.Symbol,
			&item.SnapshotTime,
			&item.ExchangeTimestamp,
			&item.BestBid,
			&item.BestAsk,
			&levelsJSON,
		); err != nil {
			return nil, 0, 0, err
		}

		if strings.TrimSpace(levelsJSON) == "" {
			levelsJSON = "[]"
		}
		if err := json.Unmarshal([]byte(levelsJSON), &item.Levels); err != nil {
			return nil, 0, 0, fmt.Errorf("decode levels_json for %s:%s@%d: %w", item.Exchange, item.Symbol, item.SnapshotTime, err)
		}
		if item.Levels == nil {
			item.Levels = make([]OrderbookHeatmapLevel, 0)
		}

		snapshots = append(snapshots, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, err
	}

	return snapshots, limit, offset, nil
}

func (s *Store) ListOrderbookHeatmapAggregated(
	ctx context.Context,
	filter OrderbookHeatmapAggregateFilter,
) ([]OrderbookHeatmapAggregatePoint, int, int, error) {
	limit, offset := normalizePagination(filter.Limit, filter.Offset)
	symbolKey := normalizeOrderbookSymbolKey(filter.SymbolKey)

	clauses := []string{"1 = 1"}
	args := make([]any, 0, 6)

	if symbolKey != "" {
		clauses = append(
			clauses,
			"UPPER(REPLACE(REPLACE(REPLACE(symbol, '-', ''), '_', ''), '/', '')) LIKE ?",
		)
		args = append(args, "%"+symbolKey+"%")
	}
	if filter.FromUnix > 0 {
		clauses = append(clauses, "snapshot_time >= ?")
		args = append(args, filter.FromUnix)
	}
	if filter.ToUnix > 0 {
		clauses = append(clauses, "snapshot_time <= ?")
		args = append(args, filter.ToUnix)
	}

	rows, err := s.db.QueryContext(
		ctx,
		`
		SELECT
			exchange, symbol, snapshot_time, levels_json
		FROM exchange_orderbook_snapshots
		WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY snapshot_time DESC, id DESC
		LIMIT ? OFFSET ?
		`,
		append(append(args, limit), offset)...,
	)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()

	points := make([]OrderbookHeatmapAggregatePoint, 0, limit*64)
	for rows.Next() {
		var exchange string
		var symbol string
		var snapshotTime int64
		var levelsJSON string
		if err := rows.Scan(&exchange, &symbol, &snapshotTime, &levelsJSON); err != nil {
			return nil, 0, 0, err
		}

		if symbolKey != "" && normalizeOrderbookSymbolKey(symbol) != symbolKey {
			continue
		}
		if strings.TrimSpace(levelsJSON) == "" {
			continue
		}

		var levels []OrderbookHeatmapLevel
		if err := json.Unmarshal([]byte(levelsJSON), &levels); err != nil {
			return nil, 0, 0, fmt.Errorf("decode levels_json for %s:%s@%d: %w", exchange, symbol, snapshotTime, err)
		}

		aggregated := aggregateOrderbookLevels(levels)
		for _, level := range aggregated {
			points = append(points, OrderbookHeatmapAggregatePoint{
				SnapshotTime: snapshotTime,
				Exchange:     exchange,
				Symbol:       symbol,
				Side:         level.Side,
				Price:        level.Price,
				Quantity:     level.Quantity,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, err
	}

	sort.Slice(points, func(i, j int) bool {
		left := points[i]
		right := points[j]
		if left.SnapshotTime != right.SnapshotTime {
			return left.SnapshotTime < right.SnapshotTime
		}
		if left.Exchange != right.Exchange {
			return left.Exchange < right.Exchange
		}
		if left.Side != right.Side {
			return left.Side < right.Side
		}
		leftPrice, leftErr := strconv.ParseFloat(left.Price, 64)
		rightPrice, rightErr := strconv.ParseFloat(right.Price, 64)
		if leftErr == nil && rightErr == nil && leftPrice != rightPrice {
			return leftPrice < rightPrice
		}
		return left.Price < right.Price
	})

	return points, limit, offset, nil
}

func aggregateOrderbookLevels(levels []OrderbookHeatmapLevel) []OrderbookHeatmapLevel {
	type levelMeta struct {
		price string
		side  string
	}

	quantityByKey := make(map[string]float64, len(levels))
	metaByKey := make(map[string]levelMeta, len(levels))

	for _, level := range levels {
		side := strings.ToLower(strings.TrimSpace(level.Side))
		if side != orderbookSideBid && side != orderbookSideAsk {
			continue
		}

		price := strings.TrimSpace(level.Price)
		quantityRaw := strings.TrimSpace(level.Quantity)
		if price == "" || quantityRaw == "" {
			continue
		}

		quantity, err := strconv.ParseFloat(quantityRaw, 64)
		if err != nil || quantity <= 0 {
			continue
		}

		key := side + "|" + price
		quantityByKey[key] += quantity
		metaByKey[key] = levelMeta{
			price: price,
			side:  side,
		}
	}

	out := make([]OrderbookHeatmapLevel, 0, len(quantityByKey))
	for key, quantity := range quantityByKey {
		meta := metaByKey[key]
		out = append(out, OrderbookHeatmapLevel{
			Side:     meta.side,
			Price:    meta.price,
			Quantity: strconv.FormatFloat(quantity, 'f', -1, 64),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Side != out[j].Side {
			return out[i].Side < out[j].Side
		}
		leftPrice, leftErr := strconv.ParseFloat(out[i].Price, 64)
		rightPrice, rightErr := strconv.ParseFloat(out[j].Price, 64)
		if leftErr == nil && rightErr == nil && leftPrice != rightPrice {
			return leftPrice < rightPrice
		}
		return out[i].Price < out[j].Price
	})

	return out
}

func normalizeOrderbookSymbolKey(raw string) string {
	value := strings.TrimSpace(strings.ToUpper(raw))
	if value == "" {
		return ""
	}

	if colonIndex := strings.LastIndex(value, ":"); colonIndex >= 0 && colonIndex+1 < len(value) {
		value = value[colonIndex+1:]
	}

	value = strings.NewReplacer("-", "", "_", "", "/", "", " ", "").Replace(value)
	for _, suffix := range []string{"PERP", "USDT", "USDC", "USD"} {
		if strings.HasSuffix(value, suffix) && len(value) > len(suffix) {
			value = strings.TrimSuffix(value, suffix)
		}
	}

	return value
}

func nowUnix() int64 {
	return time.Now().Unix()
}

func (s *Store) backfillLegacyOrderbookLevels(ctx context.Context) error {
	hasLegacyTable, err := s.hasTable(ctx, "exchange_orderbook_levels")
	if err != nil {
		return fmt.Errorf("check legacy orderbook levels table: %w", err)
	}
	if !hasLegacyTable {
		return nil
	}

	rows, err := s.db.QueryContext(
		ctx,
		`
		SELECT
			s.id, l.side, l.level, l.price, l.quantity
		FROM exchange_orderbook_snapshots s
		INNER JOIN exchange_orderbook_levels l ON l.snapshot_id = s.id
		WHERE s.levels_json = '[]'
		ORDER BY s.id ASC, CASE l.side WHEN 'bid' THEN 0 ELSE 1 END, l.level ASC
		`,
	)
	if err != nil {
		return fmt.Errorf("query legacy orderbook levels: %w", err)
	}
	defer rows.Close()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	updateStmt, err := tx.PrepareContext(ctx, `UPDATE exchange_orderbook_snapshots SET levels_json = ? WHERE id = ?`)
	if err != nil {
		return err
	}
	defer updateStmt.Close()

	flush := func(snapshotID int64, levels []OrderbookLevel) error {
		if snapshotID == 0 || len(levels) == 0 {
			return nil
		}
		payload, err := json.Marshal(levels)
		if err != nil {
			return err
		}
		_, err = updateStmt.ExecContext(ctx, string(payload), snapshotID)
		return err
	}

	var currentSnapshotID int64
	levels := make([]OrderbookLevel, 0, 128)
	for rows.Next() {
		var snapshotID int64
		var level OrderbookLevel
		if err := rows.Scan(&snapshotID, &level.Side, &level.Level, &level.Price, &level.Quantity); err != nil {
			return err
		}

		if currentSnapshotID != 0 && currentSnapshotID != snapshotID {
			if err := flush(currentSnapshotID, levels); err != nil {
				return fmt.Errorf("backfill levels_json for snapshot %d: %w", currentSnapshotID, err)
			}
			levels = levels[:0]
		}

		currentSnapshotID = snapshotID
		levels = append(levels, level)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := flush(currentSnapshotID, levels); err != nil {
		return fmt.Errorf("backfill levels_json for snapshot %d: %w", currentSnapshotID, err)
	}

	return tx.Commit()
}

func (s *Store) hasTable(ctx context.Context, table string) (bool, error) {
	row := s.db.QueryRowContext(
		ctx,
		`
		SELECT 1
		FROM information_schema.tables
		WHERE table_schema = current_schema()
		  AND table_name = ?
		LIMIT 1
		`,
		table,
	)
	var one int
	if err := row.Scan(&one); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return one == 1, nil
}
