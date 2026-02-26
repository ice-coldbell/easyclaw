package indexer

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const defaultMarketSymbol = "BTCUSDT"

type MarketPriceTickInput struct {
	Market      string
	Source      string
	FeedID      string
	Slot        int64
	PublishTime int64
	Price       float64
	Conf        float64
	Expo        int32
	ReceivedAt  int64
	RawJSON     string
}

type MarketPriceRecord struct {
	Market      string  `json:"market"`
	Source      string  `json:"source"`
	FeedID      string  `json:"feed_id"`
	Slot        int64   `json:"slot"`
	PublishTime int64   `json:"publish_time"`
	Price       float64 `json:"price"`
	Conf        float64 `json:"conf"`
	Expo        int32   `json:"expo"`
	ReceivedAt  int64   `json:"received_at"`
}

func NormalizeMarketSymbol(raw string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(raw))
	if trimmed == "" {
		return ""
	}

	var out strings.Builder
	out.Grow(len(trimmed))
	for _, r := range trimmed {
		switch {
		case r >= 'A' && r <= 'Z':
			out.WriteRune(r)
		case r >= '0' && r <= '9':
			out.WriteRune(r)
		}
	}
	return out.String()
}

func normalizeMarketWithDefault(raw string) string {
	market := NormalizeMarketSymbol(raw)
	if market == "" {
		return defaultMarketSymbol
	}
	return market
}

func (s *Store) InsertMarketPriceTick(ctx context.Context, input MarketPriceTickInput) (bool, error) {
	market := normalizeMarketWithDefault(input.Market)
	source := strings.TrimSpace(input.Source)
	if source == "" {
		source = "pyth"
	}
	feedID := strings.ToLower(strings.TrimSpace(input.FeedID))
	if feedID == "" {
		return false, fmt.Errorf("feed id is required")
	}
	if input.Price <= 0 {
		return false, fmt.Errorf("price must be > 0")
	}
	now := time.Now().Unix()
	publishTime := input.PublishTime
	if publishTime <= 0 {
		publishTime = now
	}
	receivedAt := input.ReceivedAt
	if receivedAt <= 0 {
		receivedAt = now
	}
	rawJSON := strings.TrimSpace(input.RawJSON)
	if rawJSON == "" {
		rawJSON = "{}"
	}

	result, err := s.db.ExecContext(
		ctx,
		`
		INSERT INTO market_price_ticks (
			market, source, feed_id, slot, publish_time, price, conf, expo, received_at, raw_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (market, source, publish_time, slot) DO NOTHING
		`,
		market,
		source,
		feedID,
		input.Slot,
		publishTime,
		input.Price,
		input.Conf,
		int64(input.Expo),
		receivedAt,
		rawJSON,
	)
	if err != nil {
		return false, err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, nil
	}
	return affected > 0, nil
}

func (s *Store) GetLatestMarketPrice(ctx context.Context, market string) (MarketPriceRecord, error) {
	normalized := normalizeMarketWithDefault(market)

	row := s.db.QueryRowContext(
		ctx,
		`
		SELECT market, source, feed_id, slot, publish_time, price, conf, expo, received_at
		FROM market_price_ticks
		WHERE market = ?
		ORDER BY publish_time DESC, slot DESC, id DESC
		LIMIT 1
		`,
		normalized,
	)

	var item MarketPriceRecord
	var expo int64
	if err := row.Scan(
		&item.Market,
		&item.Source,
		&item.FeedID,
		&item.Slot,
		&item.PublishTime,
		&item.Price,
		&item.Conf,
		&expo,
		&item.ReceivedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return MarketPriceRecord{}, ErrNotFound
		}
		return MarketPriceRecord{}, err
	}

	item.Expo = int32(expo)
	item.Price = round2(item.Price)
	item.Conf = round6(item.Conf)
	return item, nil
}

func (s *Store) GetMarketCandles(ctx context.Context, market string, intervalSec int64, limit int) ([]CandleRecord, error) {
	normalized := normalizeMarketWithDefault(market)
	if intervalSec <= 0 {
		intervalSec = 60
	}
	if limit <= 0 {
		limit = 120
	}
	if limit > 2000 {
		limit = 2000
	}

	lookbackBuckets := int64(limit * 8)
	if lookbackBuckets < 240 {
		lookbackBuckets = 240
	}
	fromUnix := time.Now().Unix() - (lookbackBuckets * intervalSec)

	rows, err := s.db.QueryContext(
		ctx,
		`
		WITH bucketed AS (
			SELECT
				(publish_time / ?) * ? AS bucket_ts,
				publish_time,
				slot,
				id,
				price,
				ROW_NUMBER() OVER (
					PARTITION BY (publish_time / ?) * ?
					ORDER BY publish_time ASC, slot ASC, id ASC
				) AS rn_open,
				ROW_NUMBER() OVER (
					PARTITION BY (publish_time / ?) * ?
					ORDER BY publish_time DESC, slot DESC, id DESC
				) AS rn_close
			FROM market_price_ticks
			WHERE market = ?
			  AND publish_time >= ?
		),
		aggregated AS (
			SELECT
				bucket_ts,
				MAX(CASE WHEN rn_open = 1 THEN price END) AS open,
				MAX(price) AS high,
				MIN(price) AS low,
				MAX(CASE WHEN rn_close = 1 THEN price END) AS close,
				COUNT(*)::DOUBLE PRECISION AS volume
			FROM bucketed
			GROUP BY bucket_ts
		)
		SELECT bucket_ts, open, high, low, close, volume
		FROM aggregated
		ORDER BY bucket_ts DESC
		LIMIT ?
		`,
		intervalSec,
		intervalSec,
		intervalSec,
		intervalSec,
		intervalSec,
		intervalSec,
		normalized,
		fromUnix,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candles := make([]CandleRecord, 0, limit)
	for rows.Next() {
		var item CandleRecord
		if err := rows.Scan(
			&item.TS,
			&item.Open,
			&item.High,
			&item.Low,
			&item.Close,
			&item.Volume,
		); err != nil {
			return nil, err
		}
		item.Open = round2(item.Open)
		item.High = round2(item.High)
		item.Low = round2(item.Low)
		item.Close = round2(item.Close)
		item.Volume = round6(item.Volume)
		candles = append(candles, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Query is DESC for efficient LIMIT; convert back to ASC for chart consumers.
	for left, right := 0, len(candles)-1; left < right; left, right = left+1, right-1 {
		candles[left], candles[right] = candles[right], candles[left]
	}

	return candles, nil
}
