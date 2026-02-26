package indexer

import (
	"context"
	"fmt"
	"strings"
)

const (
	defaultPageLimit = 50
	maxPageLimit     = 200
)

type PositionFilter struct {
	UserMargin string
	MarketID   *uint64
	Limit      int
	Offset     int
}

type PositionRecord struct {
	Pubkey                string `json:"pubkey"`
	UserMargin            string `json:"user_margin"`
	MarketID              uint64 `json:"market_id"`
	LongQty               string `json:"long_qty"`
	LongEntryNotional     string `json:"long_entry_notional"`
	ShortQty              string `json:"short_qty"`
	ShortEntryNotional    string `json:"short_entry_notional"`
	LastFundingIndexLong  string `json:"last_funding_index_long"`
	LastFundingIndexShort string `json:"last_funding_index_short"`
	Slot                  uint64 `json:"slot"`
	UpdatedAt             int64  `json:"updated_at"`
}

type OrderFilter struct {
	UserMargin string
	UserPubkey string
	MarketID   *uint64
	Status     string
	Limit      int
	Offset     int
}

type OrderRecord struct {
	Pubkey        string `json:"pubkey"`
	UserMargin    string `json:"user_margin"`
	UserPubkey    string `json:"user_pubkey"`
	MarketID      uint64 `json:"market_id"`
	Side          string `json:"side"`
	OrderType     string `json:"order_type"`
	ReduceOnly    bool   `json:"reduce_only"`
	Margin        string `json:"margin"`
	Price         string `json:"price"`
	Status        string `json:"status"`
	CreatedAt     int64  `json:"created_at"`
	ExpiresAt     int64  `json:"expires_at"`
	ClientOrderID string `json:"client_order_id"`
	Slot          uint64 `json:"slot"`
	UpdatedAt     int64  `json:"updated_at"`
}

type FillFilter struct {
	UserMargin string
	UserPubkey string
	MarketID   *uint64
	Limit      int
	Offset     int
}

type FillRecord struct {
	ID            int64  `json:"id"`
	OrderPubkey   string `json:"order_pubkey"`
	UserMargin    string `json:"user_margin"`
	UserPubkey    string `json:"user_pubkey"`
	MarketID      uint64 `json:"market_id"`
	Side          string `json:"side"`
	OrderType     string `json:"order_type"`
	ReduceOnly    bool   `json:"reduce_only"`
	Margin        string `json:"margin"`
	Price         string `json:"price"`
	ClientOrderID string `json:"client_order_id"`
	CreatedAt     int64  `json:"created_at"`
	ExpiresAt     int64  `json:"expires_at"`
	ExecutedSlot  uint64 `json:"executed_slot"`
	ExecutedAt    int64  `json:"executed_at"`
}

type PositionHistoryFilter struct {
	UserMargin string
	MarketID   *uint64
	Limit      int
	Offset     int
}

type PositionHistoryRecord struct {
	ID                        int64  `json:"id"`
	PositionPubkey            string `json:"position_pubkey"`
	UserMargin                string `json:"user_margin"`
	MarketID                  uint64 `json:"market_id"`
	EventType                 string `json:"event_type"`
	PrevLongQty               string `json:"prev_long_qty"`
	PrevLongEntryNotional     string `json:"prev_long_entry_notional"`
	PrevShortQty              string `json:"prev_short_qty"`
	PrevShortEntryNotional    string `json:"prev_short_entry_notional"`
	PrevLastFundingIndexLong  string `json:"prev_last_funding_index_long"`
	PrevLastFundingIndexShort string `json:"prev_last_funding_index_short"`
	NextLongQty               string `json:"next_long_qty"`
	NextLongEntryNotional     string `json:"next_long_entry_notional"`
	NextShortQty              string `json:"next_short_qty"`
	NextShortEntryNotional    string `json:"next_short_entry_notional"`
	NextLastFundingIndexLong  string `json:"next_last_funding_index_long"`
	NextLastFundingIndexShort string `json:"next_last_funding_index_short"`
	Slot                      uint64 `json:"slot"`
	RecordedAt                int64  `json:"recorded_at"`
}

func (s *Store) ListPositions(ctx context.Context, filter PositionFilter) ([]PositionRecord, int, int, error) {
	limit, offset := normalizePagination(filter.Limit, filter.Offset)
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 4)

	if filter.UserMargin != "" {
		clauses = append(clauses, "user_margin = ?")
		args = append(args, filter.UserMargin)
	}
	if filter.MarketID != nil {
		clauses = append(clauses, "market_id = ?")
		args = append(args, int64(*filter.MarketID))
	}

	query := fmt.Sprintf(`
		SELECT
			pubkey,
			user_margin,
			market_id,
			long_qty,
			long_entry_notional,
			short_qty,
			short_entry_notional,
			last_funding_index_long,
			last_funding_index_short,
			slot,
			updated_at
		FROM positions
		WHERE %s
		ORDER BY updated_at DESC, pubkey ASC
		LIMIT ? OFFSET ?
	`, strings.Join(clauses, " AND "))
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()

	items := make([]PositionRecord, 0, limit)
	for rows.Next() {
		var item PositionRecord
		var marketID int64
		var slot int64
		if err := rows.Scan(
			&item.Pubkey,
			&item.UserMargin,
			&marketID,
			&item.LongQty,
			&item.LongEntryNotional,
			&item.ShortQty,
			&item.ShortEntryNotional,
			&item.LastFundingIndexLong,
			&item.LastFundingIndexShort,
			&slot,
			&item.UpdatedAt,
		); err != nil {
			return nil, 0, 0, err
		}
		item.MarketID = uint64(marketID)
		item.Slot = uint64(slot)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, err
	}

	return items, limit, offset, nil
}

func (s *Store) ListOrders(ctx context.Context, filter OrderFilter) ([]OrderRecord, int, int, error) {
	limit, offset := normalizePagination(filter.Limit, filter.Offset)
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 6)

	if filter.UserMargin != "" {
		clauses = append(clauses, "user_margin = ?")
		args = append(args, filter.UserMargin)
	}
	if filter.UserPubkey != "" {
		clauses = append(clauses, "user_pubkey = ?")
		args = append(args, filter.UserPubkey)
	}
	if filter.MarketID != nil {
		clauses = append(clauses, "market_id = ?")
		args = append(args, int64(*filter.MarketID))
	}
	if filter.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, filter.Status)
	}

	query := fmt.Sprintf(`
		SELECT
			pubkey,
			user_margin,
			user_pubkey,
			market_id,
			side,
			order_type,
			reduce_only,
			margin,
			price,
			status,
			created_at,
			expires_at,
			client_order_id,
			slot,
			updated_at
		FROM orders
		WHERE %s
		ORDER BY updated_at DESC, pubkey ASC
		LIMIT ? OFFSET ?
	`, strings.Join(clauses, " AND "))
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()

	items := make([]OrderRecord, 0, limit)
	for rows.Next() {
		var item OrderRecord
		var marketID int64
		var reduceOnly int
		var slot int64
		if err := rows.Scan(
			&item.Pubkey,
			&item.UserMargin,
			&item.UserPubkey,
			&marketID,
			&item.Side,
			&item.OrderType,
			&reduceOnly,
			&item.Margin,
			&item.Price,
			&item.Status,
			&item.CreatedAt,
			&item.ExpiresAt,
			&item.ClientOrderID,
			&slot,
			&item.UpdatedAt,
		); err != nil {
			return nil, 0, 0, err
		}
		item.MarketID = uint64(marketID)
		item.ReduceOnly = reduceOnly != 0
		item.Slot = uint64(slot)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, err
	}

	return items, limit, offset, nil
}

func (s *Store) ListFills(ctx context.Context, filter FillFilter) ([]FillRecord, int, int, error) {
	limit, offset := normalizePagination(filter.Limit, filter.Offset)
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 5)

	if filter.UserMargin != "" {
		clauses = append(clauses, "user_margin = ?")
		args = append(args, filter.UserMargin)
	}
	if filter.UserPubkey != "" {
		clauses = append(clauses, "user_pubkey = ?")
		args = append(args, filter.UserPubkey)
	}
	if filter.MarketID != nil {
		clauses = append(clauses, "market_id = ?")
		args = append(args, int64(*filter.MarketID))
	}

	query := fmt.Sprintf(`
		SELECT
			id,
			order_pubkey,
			user_margin,
			user_pubkey,
			market_id,
			side,
			order_type,
			reduce_only,
			margin,
			price,
			client_order_id,
			created_at,
			expires_at,
			executed_slot,
			executed_at
		FROM fills
		WHERE %s
		ORDER BY executed_at DESC, id DESC
		LIMIT ? OFFSET ?
	`, strings.Join(clauses, " AND "))
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()

	items := make([]FillRecord, 0, limit)
	for rows.Next() {
		var item FillRecord
		var marketID int64
		var reduceOnly int
		var executedSlot int64
		if err := rows.Scan(
			&item.ID,
			&item.OrderPubkey,
			&item.UserMargin,
			&item.UserPubkey,
			&marketID,
			&item.Side,
			&item.OrderType,
			&reduceOnly,
			&item.Margin,
			&item.Price,
			&item.ClientOrderID,
			&item.CreatedAt,
			&item.ExpiresAt,
			&executedSlot,
			&item.ExecutedAt,
		); err != nil {
			return nil, 0, 0, err
		}
		item.MarketID = uint64(marketID)
		item.ReduceOnly = reduceOnly != 0
		item.ExecutedSlot = uint64(executedSlot)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, err
	}

	return items, limit, offset, nil
}

func (s *Store) ListPositionHistory(ctx context.Context, filter PositionHistoryFilter) ([]PositionHistoryRecord, int, int, error) {
	limit, offset := normalizePagination(filter.Limit, filter.Offset)
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 4)

	if filter.UserMargin != "" {
		clauses = append(clauses, "user_margin = ?")
		args = append(args, filter.UserMargin)
	}
	if filter.MarketID != nil {
		clauses = append(clauses, "market_id = ?")
		args = append(args, int64(*filter.MarketID))
	}

	query := fmt.Sprintf(`
		SELECT
			id,
			position_pubkey,
			user_margin,
			market_id,
			event_type,
			prev_long_qty,
			prev_long_entry_notional,
			prev_short_qty,
			prev_short_entry_notional,
			prev_last_funding_index_long,
			prev_last_funding_index_short,
			next_long_qty,
			next_long_entry_notional,
			next_short_qty,
			next_short_entry_notional,
			next_last_funding_index_long,
			next_last_funding_index_short,
			slot,
			recorded_at
		FROM position_history
		WHERE %s
		ORDER BY recorded_at DESC, id DESC
		LIMIT ? OFFSET ?
	`, strings.Join(clauses, " AND "))
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()

	items := make([]PositionHistoryRecord, 0, limit)
	for rows.Next() {
		var item PositionHistoryRecord
		var marketID int64
		var slot int64
		if err := rows.Scan(
			&item.ID,
			&item.PositionPubkey,
			&item.UserMargin,
			&marketID,
			&item.EventType,
			&item.PrevLongQty,
			&item.PrevLongEntryNotional,
			&item.PrevShortQty,
			&item.PrevShortEntryNotional,
			&item.PrevLastFundingIndexLong,
			&item.PrevLastFundingIndexShort,
			&item.NextLongQty,
			&item.NextLongEntryNotional,
			&item.NextShortQty,
			&item.NextShortEntryNotional,
			&item.NextLastFundingIndexLong,
			&item.NextLastFundingIndexShort,
			&slot,
			&item.RecordedAt,
		); err != nil {
			return nil, 0, 0, err
		}
		item.MarketID = uint64(marketID)
		item.Slot = uint64(slot)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, err
	}

	return items, limit, offset, nil
}

func normalizePagination(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = defaultPageLimit
	}
	if limit > maxPageLimit {
		limit = maxPageLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
