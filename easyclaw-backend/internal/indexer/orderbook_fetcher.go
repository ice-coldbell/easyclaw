package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coldbell/dex/backend/internal/config"
	"github.com/gorilla/websocket"
)

const (
	maxOrderbookWorkerBackoff = 30 * time.Second
	websocketReadLimitBytes   = 16 << 20
	websocketWriteTimeout     = 5 * time.Second
)

type orderbookSnapshotProvider interface {
	Name() string
	FetchOrderbook(ctx context.Context, symbol string, depth int) ([]OrderbookLevel, []OrderbookLevel, int64, string, error)
}

type orderbookStreamHandler func(bids, asks []OrderbookLevel, exchangeTS int64, rawJSON string)

type orderbookStreamingProvider interface {
	orderbookSnapshotProvider
	SupportsWebsocket(depth int) bool
	StreamOrderbook(ctx context.Context, symbol string, depth int, handler orderbookStreamHandler) error
}

type orderbookCacheEntry struct {
	snapshot  OrderbookSnapshot
	updatedAt time.Time
}

type orderbookCollector struct {
	targets         []config.OrderbookTarget
	depth           int
	refreshInterval time.Duration
	providers       map[string]orderbookSnapshotProvider
	logger          *slog.Logger

	mu      sync.RWMutex
	cached  map[string]orderbookCacheEntry
	started bool
}

func newOrderbookCollector(
	targets []config.OrderbookTarget,
	depth int,
	timeout time.Duration,
	refreshInterval time.Duration,
	logger *slog.Logger,
) *orderbookCollector {
	if depth <= 0 {
		depth = 1
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if refreshInterval <= 0 {
		refreshInterval = time.Second
	}

	httpClient := &http.Client{Timeout: timeout}
	providers := map[string]orderbookSnapshotProvider{
		"binance":  &binanceOrderbookProvider{client: httpClient},
		"okx":      &okxOrderbookProvider{client: httpClient},
		"coinbase": &coinbaseOrderbookProvider{client: httpClient},
		"bybit":    &bybitOrderbookProvider{client: httpClient},
	}

	return &orderbookCollector{
		targets:         targets,
		depth:           depth,
		refreshInterval: refreshInterval,
		providers:       providers,
		logger:          logger,
		cached:          make(map[string]orderbookCacheEntry, len(targets)),
	}
}

func (c *orderbookCollector) cacheKey(target config.OrderbookTarget) string {
	return strings.ToLower(target.Exchange) + ":" + target.Symbol
}

func (c *orderbookCollector) Start(ctx context.Context) {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return
	}
	c.started = true
	targets := append([]config.OrderbookTarget(nil), c.targets...)
	c.mu.Unlock()

	for _, target := range targets {
		target := target
		provider := c.providers[strings.ToLower(target.Exchange)]
		if provider == nil {
			c.logger.Warn(
				"orderbook target skipped due to unsupported exchange",
				"exchange", target.Exchange,
				"symbol", target.Symbol,
			)
			continue
		}

		go c.runTargetLoop(ctx, target, provider)
	}
}

func (c *orderbookCollector) Bootstrap(ctx context.Context) map[config.OrderbookTarget]error {
	type result struct {
		target config.OrderbookTarget
		err    error
	}

	results := make(map[config.OrderbookTarget]error, len(c.targets))
	if len(c.targets) == 0 {
		return results
	}

	ch := make(chan result, len(c.targets))
	for _, target := range c.targets {
		target := target
		go func() {
			snapshot, err := c.fetchSnapshot(ctx, target)
			if err == nil {
				c.setCachedSnapshot(target, snapshot, time.Now())
			}
			ch <- result{target: target, err: err}
		}()
	}

	for range c.targets {
		res := <-ch
		results[res.target] = res.err
	}

	return results
}

func (c *orderbookCollector) SnapshotAll(snapshotTime int64) []OrderbookSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]OrderbookSnapshot, 0, len(c.targets))
	for _, target := range c.targets {
		cached, ok := c.cached[c.cacheKey(target)]
		if !ok {
			continue
		}
		if cached.updatedAt.Unix() < snapshotTime {
			continue
		}

		snapshot := cached.snapshot
		snapshot.SnapshotTime = snapshotTime
		if len(snapshot.Levels) > 0 {
			levels := make([]OrderbookLevel, len(snapshot.Levels))
			copy(levels, snapshot.Levels)
			snapshot.Levels = levels
		}
		out = append(out, snapshot)
	}

	return out
}

func (c *orderbookCollector) runTargetLoop(
	ctx context.Context,
	target config.OrderbookTarget,
	provider orderbookSnapshotProvider,
) {
	if streamProvider, ok := provider.(orderbookStreamingProvider); ok && streamProvider.SupportsWebsocket(c.depth) {
		c.runTargetWebsocketLoop(ctx, target, streamProvider)
		return
	}
	c.runTargetPollingLoop(ctx, target, provider)
}

func (c *orderbookCollector) runTargetPollingLoop(
	ctx context.Context,
	target config.OrderbookTarget,
	provider orderbookSnapshotProvider,
) {
	timer := time.NewTimer(0)
	defer timer.Stop()

	backoff := c.refreshInterval
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		snapshot, err := c.fetchSnapshotFromProvider(ctx, target, provider)
		if err != nil {
			if ctx.Err() != nil {
				return
			}

			c.logger.Warn(
				"orderbook refresh failed",
				"exchange", target.Exchange,
				"symbol", target.Symbol,
				"mode", "polling",
				"err", err,
			)
			backoff = nextBackoff(backoff, c.refreshInterval)
			timer.Reset(backoff)
			continue
		}

		c.setCachedSnapshot(target, snapshot, time.Now())
		backoff = c.refreshInterval
		timer.Reset(c.refreshInterval)
	}
}

func (c *orderbookCollector) runTargetWebsocketLoop(
	ctx context.Context,
	target config.OrderbookTarget,
	provider orderbookStreamingProvider,
) {
	backoff := c.refreshInterval
	for {
		if ctx.Err() != nil {
			return
		}

		err := provider.StreamOrderbook(ctx, target.Symbol, c.depth, func(
			bids, asks []OrderbookLevel,
			exchangeTS int64,
			rawJSON string,
		) {
			snapshot := c.buildSnapshot(target, bids, asks, exchangeTS, rawJSON)
			c.setCachedSnapshot(target, snapshot, time.Now())
		})
		if ctx.Err() != nil {
			return
		}

		if err != nil && !errors.Is(err, context.Canceled) {
			c.logger.Warn(
				"orderbook websocket stream failed",
				"exchange", target.Exchange,
				"symbol", target.Symbol,
				"err", err,
			)
		}

		backoff = nextBackoff(backoff, c.refreshInterval)
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (c *orderbookCollector) setCachedSnapshot(target config.OrderbookTarget, snapshot OrderbookSnapshot, updatedAt time.Time) {
	c.mu.Lock()
	c.cached[c.cacheKey(target)] = orderbookCacheEntry{
		snapshot:  snapshot,
		updatedAt: updatedAt,
	}
	c.mu.Unlock()
}

func nextBackoff(current, floor time.Duration) time.Duration {
	if floor <= 0 {
		floor = time.Second
	}
	if current < floor {
		current = floor
	}
	next := current * 2
	if next > maxOrderbookWorkerBackoff {
		return maxOrderbookWorkerBackoff
	}
	return next
}

func (c *orderbookCollector) fetchSnapshot(ctx context.Context, target config.OrderbookTarget) (OrderbookSnapshot, error) {
	provider := c.providers[strings.ToLower(target.Exchange)]
	if provider == nil {
		return OrderbookSnapshot{}, fmt.Errorf("unsupported exchange %q", target.Exchange)
	}
	return c.fetchSnapshotFromProvider(ctx, target, provider)
}

func (c *orderbookCollector) fetchSnapshotFromProvider(
	ctx context.Context,
	target config.OrderbookTarget,
	provider orderbookSnapshotProvider,
) (OrderbookSnapshot, error) {
	bids, asks, exchangeTS, rawJSON, err := provider.FetchOrderbook(ctx, target.Symbol, c.depth)
	if err != nil {
		return OrderbookSnapshot{}, err
	}
	return c.buildSnapshot(target, bids, asks, exchangeTS, rawJSON), nil
}

func (c *orderbookCollector) buildSnapshot(
	target config.OrderbookTarget,
	bids, asks []OrderbookLevel,
	exchangeTS int64,
	rawJSON string,
) OrderbookSnapshot {
	snapshot := OrderbookSnapshot{
		Exchange:          target.Exchange,
		Symbol:            target.Symbol,
		SnapshotTime:      0,
		ExchangeTimestamp: exchangeTS,
		RawJSON:           rawJSON,
		Levels:            make([]OrderbookLevel, 0, len(bids)+len(asks)),
	}
	if len(bids) > 0 {
		snapshot.BestBid = bids[0].Price
	}
	if len(asks) > 0 {
		snapshot.BestAsk = asks[0].Price
	}

	for i := range bids {
		level := bids[i]
		level.Side = orderbookSideBid
		snapshot.Levels = append(snapshot.Levels, level)
	}
	for i := range asks {
		level := asks[i]
		level.Side = orderbookSideAsk
		snapshot.Levels = append(snapshot.Levels, level)
	}

	return snapshot
}

type levelBook struct {
	bids map[string]string
	asks map[string]string
}

func newLevelBook() *levelBook {
	return &levelBook{
		bids: make(map[string]string, 2048),
		asks: make(map[string]string, 2048),
	}
}

func (b *levelBook) resetFromLevels(bids, asks []OrderbookLevel) {
	clear(b.bids)
	clear(b.asks)
	for i := range bids {
		b.apply(orderbookSideBid, bids[i].Price, bids[i].Quantity)
	}
	for i := range asks {
		b.apply(orderbookSideAsk, asks[i].Price, asks[i].Quantity)
	}
}

func (b *levelBook) resetFromPairs(bids, asks [][]string) {
	clear(b.bids)
	clear(b.asks)
	b.applyUpdates(orderbookSideBid, bids)
	b.applyUpdates(orderbookSideAsk, asks)
}

func (b *levelBook) applyUpdates(side string, levels [][]string) {
	for _, level := range levels {
		if len(level) < 2 {
			continue
		}
		b.apply(side, level[0], level[1])
	}
}

func (b *levelBook) apply(side, price, quantity string) {
	price = strings.TrimSpace(price)
	if price == "" {
		return
	}
	if isZeroQuantity(quantity) {
		if side == orderbookSideBid {
			delete(b.bids, price)
			return
		}
		delete(b.asks, price)
		return
	}

	if side == orderbookSideBid {
		b.bids[price] = quantity
		return
	}
	b.asks[price] = quantity
}

func (b *levelBook) topLevels(depth int) ([]OrderbookLevel, []OrderbookLevel) {
	if depth <= 0 {
		depth = 1
	}

	bids := topSideLevels(b.bids, depth, true)
	asks := topSideLevels(b.asks, depth, false)
	return bids, asks
}

func topSideLevels(levels map[string]string, depth int, desc bool) []OrderbookLevel {
	prices := make([]string, 0, len(levels))
	for price, quantity := range levels {
		if strings.TrimSpace(price) == "" || isZeroQuantity(quantity) {
			continue
		}
		prices = append(prices, price)
	}

	sort.Slice(prices, func(i, j int) bool {
		leftVal, leftOK := parsePrice(prices[i])
		rightVal, rightOK := parsePrice(prices[j])
		if leftOK && rightOK {
			if leftVal == rightVal {
				if desc {
					return prices[i] > prices[j]
				}
				return prices[i] < prices[j]
			}
			if desc {
				return leftVal > rightVal
			}
			return leftVal < rightVal
		}
		if desc {
			return prices[i] > prices[j]
		}
		return prices[i] < prices[j]
	})

	if len(prices) > depth {
		prices = prices[:depth]
	}

	out := make([]OrderbookLevel, 0, len(prices))
	for idx, price := range prices {
		out = append(out, OrderbookLevel{
			Level:    idx,
			Price:    price,
			Quantity: levels[price],
		})
	}
	return out
}

func parsePrice(raw string) (float64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func isZeroQuantity(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return true
	}
	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return false
	}
	return value == 0
}

func dialWebsocket(ctx context.Context, endpoint string) (*websocket.Conn, *http.Response, error) {
	dialer := websocket.Dialer{
		Proxy:             http.ProxyFromEnvironment,
		HandshakeTimeout:  10 * time.Second,
		EnableCompression: true,
	}

	conn, resp, err := dialer.DialContext(ctx, endpoint, nil)
	if err != nil {
		return nil, resp, err
	}
	conn.SetReadLimit(websocketReadLimitBytes)
	return conn, resp, nil
}

func writeWebsocketJSON(conn *websocket.Conn, value any) error {
	if err := conn.SetWriteDeadline(time.Now().Add(websocketWriteTimeout)); err != nil {
		return err
	}
	return conn.WriteJSON(value)
}

func closeConnOnContextDone(ctx context.Context, conn *websocket.Conn) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	return func() {
		close(done)
	}
}

type binanceOrderbookProvider struct {
	client *http.Client
}

func (*binanceOrderbookProvider) Name() string { return "binance" }

func (*binanceOrderbookProvider) SupportsWebsocket(depth int) bool {
	return depth > 0
}

func (p *binanceOrderbookProvider) StreamOrderbook(
	ctx context.Context,
	symbol string,
	depth int,
	handler orderbookStreamHandler,
) error {
	endpoint := fmt.Sprintf("wss://stream.binance.com:9443/ws/%s@depth@100ms", strings.ToLower(symbol))
	conn, _, err := dialWebsocket(ctx, endpoint)
	if err != nil {
		return err
	}
	defer conn.Close()
	stopClose := closeConnOnContextDone(ctx, conn)
	defer stopClose()

	snapshotDepth := depth
	if snapshotDepth < 1000 {
		snapshotDepth = 1000
	}
	if snapshotDepth > 5000 {
		snapshotDepth = 5000
	}

	bids, asks, lastUpdateID, _, err := p.FetchOrderbook(ctx, symbol, snapshotDepth)
	if err != nil {
		return fmt.Errorf("binance snapshot bootstrap failed: %w", err)
	}

	book := newLevelBook()
	book.resetFromLevels(bids, asks)

	synced := false
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}

		var message struct {
			EventType     string     `json:"e"`
			EventTime     int64      `json:"E"`
			FirstUpdateID int64      `json:"U"`
			FinalUpdateID int64      `json:"u"`
			Bids          [][]string `json:"b"`
			Asks          [][]string `json:"a"`
		}
		if err := json.Unmarshal(payload, &message); err != nil {
			continue
		}
		if message.FinalUpdateID == 0 {
			continue
		}
		if message.FinalUpdateID <= lastUpdateID {
			continue
		}
		if !synced {
			expected := lastUpdateID + 1
			if message.FirstUpdateID > expected || message.FinalUpdateID < expected {
				return fmt.Errorf(
					"binance stream out of sync: snapshot=%d event=[%d,%d]",
					lastUpdateID,
					message.FirstUpdateID,
					message.FinalUpdateID,
				)
			}
			synced = true
		}

		book.applyUpdates(orderbookSideBid, message.Bids)
		book.applyUpdates(orderbookSideAsk, message.Asks)
		lastUpdateID = message.FinalUpdateID

		outBids, outAsks := book.topLevels(depth)
		handler(outBids, outAsks, message.EventTime, string(payload))
	}
}

func (p *binanceOrderbookProvider) FetchOrderbook(ctx context.Context, symbol string, depth int) ([]OrderbookLevel, []OrderbookLevel, int64, string, error) {
	endpoint := fmt.Sprintf(
		"https://api.binance.com/api/v3/depth?symbol=%s&limit=%d",
		url.QueryEscape(symbol),
		depth,
	)
	payload, raw, err := fetchJSON(ctx, p.client, endpoint)
	if err != nil {
		return nil, nil, 0, "", err
	}

	bids, err := parseOrderbookLevels(payload["bids"], depth)
	if err != nil {
		return nil, nil, 0, "", err
	}
	asks, err := parseOrderbookLevels(payload["asks"], depth)
	if err != nil {
		return nil, nil, 0, "", err
	}

	return bids, asks, asInt64(payload["lastUpdateId"]), raw, nil
}

type okxOrderbookProvider struct {
	client *http.Client
}

func (*okxOrderbookProvider) Name() string { return "okx" }

func (*okxOrderbookProvider) SupportsWebsocket(depth int) bool {
	return depth > 0
}

func (p *okxOrderbookProvider) StreamOrderbook(
	ctx context.Context,
	symbol string,
	depth int,
	handler orderbookStreamHandler,
) error {
	endpoint := "wss://ws.okx.com:8443/ws/v5/public"
	conn, _, err := dialWebsocket(ctx, endpoint)
	if err != nil {
		return err
	}
	defer conn.Close()
	stopClose := closeConnOnContextDone(ctx, conn)
	defer stopClose()

	channel := okxOrderbookChannel(depth)
	subscribePayload := map[string]any{
		"op": "subscribe",
		"args": []map[string]string{
			{"channel": channel, "instId": symbol},
		},
	}
	if err := writeWebsocketJSON(conn, subscribePayload); err != nil {
		return err
	}

	book := newLevelBook()
	initialized := false
	lastSeqID := int64(0)

	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}

		var message struct {
			Event  string `json:"event"`
			Code   string `json:"code"`
			Msg    string `json:"msg"`
			Action string `json:"action"`
			Arg    struct {
				Channel string `json:"channel"`
				InstID  string `json:"instId"`
			} `json:"arg"`
			Data []struct {
				Asks      [][]string `json:"asks"`
				Bids      [][]string `json:"bids"`
				TS        string     `json:"ts"`
				SeqID     int64      `json:"seqId"`
				PrevSeqID int64      `json:"prevSeqId"`
			} `json:"data"`
		}
		if err := json.Unmarshal(payload, &message); err != nil {
			continue
		}

		if message.Event == "error" {
			return fmt.Errorf("okx websocket error: code=%s msg=%s", message.Code, message.Msg)
		}
		if message.Event != "" {
			continue
		}
		if len(message.Data) == 0 {
			continue
		}

		for _, item := range message.Data {
			if !initialized {
				if message.Action == "snapshot" || channel == "books5" {
					book.resetFromPairs(item.Bids, item.Asks)
				} else {
					bootstrapDepth := depth
					if bootstrapDepth < 400 {
						bootstrapDepth = 400
					}
					bids, asks, _, _, err := p.FetchOrderbook(ctx, symbol, bootstrapDepth)
					if err != nil {
						return fmt.Errorf("okx websocket bootstrap snapshot failed: %w", err)
					}
					book.resetFromLevels(bids, asks)
					book.applyUpdates(orderbookSideBid, item.Bids)
					book.applyUpdates(orderbookSideAsk, item.Asks)
				}
				initialized = true
				lastSeqID = item.SeqID
			} else {
				if channel == "books5" || message.Action == "snapshot" {
					book.resetFromPairs(item.Bids, item.Asks)
				} else {
					if item.PrevSeqID != 0 && lastSeqID != 0 && item.PrevSeqID != lastSeqID {
						return fmt.Errorf(
							"okx sequence gap: prev=%d expected=%d current=%d",
							item.PrevSeqID,
							lastSeqID,
							item.SeqID,
						)
					}
					book.applyUpdates(orderbookSideBid, item.Bids)
					book.applyUpdates(orderbookSideAsk, item.Asks)
				}
				lastSeqID = item.SeqID
			}

			outBids, outAsks := book.topLevels(depth)
			handler(outBids, outAsks, asInt64(item.TS), string(payload))
		}
	}
}

func okxOrderbookChannel(depth int) string {
	if depth <= 5 {
		return "books5"
	}
	return "books"
}

func (p *okxOrderbookProvider) FetchOrderbook(ctx context.Context, symbol string, depth int) ([]OrderbookLevel, []OrderbookLevel, int64, string, error) {
	endpoint := fmt.Sprintf(
		"https://www.okx.com/api/v5/market/books?instId=%s&sz=%d",
		url.QueryEscape(symbol),
		depth,
	)
	payload, raw, err := fetchJSON(ctx, p.client, endpoint)
	if err != nil {
		return nil, nil, 0, "", err
	}
	if code, ok := payload["code"]; ok && asString(code) != "0" {
		return nil, nil, 0, "", fmt.Errorf("okx api error: %s", asString(payload["msg"]))
	}

	dataRows, ok := payload["data"].([]any)
	if !ok || len(dataRows) == 0 {
		return nil, nil, 0, "", fmt.Errorf("okx response missing data")
	}
	dataObj, ok := dataRows[0].(map[string]any)
	if !ok {
		return nil, nil, 0, "", fmt.Errorf("okx response data type invalid")
	}

	bids, err := parseOrderbookLevels(dataObj["bids"], depth)
	if err != nil {
		return nil, nil, 0, "", err
	}
	asks, err := parseOrderbookLevels(dataObj["asks"], depth)
	if err != nil {
		return nil, nil, 0, "", err
	}

	return bids, asks, asInt64(dataObj["ts"]), raw, nil
}

type coinbaseOrderbookProvider struct {
	client *http.Client
}

func (*coinbaseOrderbookProvider) Name() string { return "coinbase" }

func (*coinbaseOrderbookProvider) SupportsWebsocket(depth int) bool {
	return depth > 0
}

func (p *coinbaseOrderbookProvider) StreamOrderbook(
	ctx context.Context,
	symbol string,
	depth int,
	handler orderbookStreamHandler,
) error {
	endpoint := "wss://ws-feed.exchange.coinbase.com"
	conn, _, err := dialWebsocket(ctx, endpoint)
	if err != nil {
		return err
	}
	defer conn.Close()
	stopClose := closeConnOnContextDone(ctx, conn)
	defer stopClose()

	subscribePayload := map[string]any{
		"type": "subscribe",
		"channels": []map[string]any{
			{
				"name":        "level2",
				"product_ids": []string{symbol},
			},
		},
	}
	if err := writeWebsocketJSON(conn, subscribePayload); err != nil {
		return err
	}

	book := newLevelBook()
	initialized := false

	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}

		var message struct {
			Type      string     `json:"type"`
			Message   string     `json:"message"`
			Reason    string     `json:"reason"`
			ProductID string     `json:"product_id"`
			Time      string     `json:"time"`
			Bids      [][]string `json:"bids"`
			Asks      [][]string `json:"asks"`
			Changes   [][]string `json:"changes"`
		}
		if err := json.Unmarshal(payload, &message); err != nil {
			continue
		}
		if message.Type == "error" {
			return fmt.Errorf("coinbase websocket error: %s %s", message.Reason, message.Message)
		}

		switch message.Type {
		case "snapshot":
			book.resetFromPairs(message.Bids, message.Asks)
			initialized = true
			outBids, outAsks := book.topLevels(depth)
			handler(outBids, outAsks, parseCoinbaseTime(message.Time), string(payload))
		case "l2update":
			if !initialized {
				continue
			}
			for _, change := range message.Changes {
				if len(change) < 3 {
					continue
				}
				side := orderbookSideAsk
				if strings.EqualFold(change[0], "buy") {
					side = orderbookSideBid
				}
				book.apply(side, change[1], change[2])
			}
			outBids, outAsks := book.topLevels(depth)
			handler(outBids, outAsks, parseCoinbaseTime(message.Time), string(payload))
		default:
			continue
		}
	}
}

func (p *coinbaseOrderbookProvider) FetchOrderbook(ctx context.Context, symbol string, depth int) ([]OrderbookLevel, []OrderbookLevel, int64, string, error) {
	endpoint := fmt.Sprintf(
		"https://api.exchange.coinbase.com/products/%s/book?level=2",
		url.PathEscape(symbol),
	)
	payload, raw, err := fetchJSON(ctx, p.client, endpoint)
	if err != nil {
		return nil, nil, 0, "", err
	}
	if message, ok := payload["message"]; ok {
		msg := asString(message)
		if msg != "" {
			return nil, nil, 0, "", fmt.Errorf("coinbase api error: %s", msg)
		}
	}

	bids, err := parseOrderbookLevels(payload["bids"], depth)
	if err != nil {
		return nil, nil, 0, "", err
	}
	asks, err := parseOrderbookLevels(payload["asks"], depth)
	if err != nil {
		return nil, nil, 0, "", err
	}

	return bids, asks, parseCoinbaseTime(payload["time"]), raw, nil
}

type bybitOrderbookProvider struct {
	client *http.Client
}

func (*bybitOrderbookProvider) Name() string { return "bybit" }

func (*bybitOrderbookProvider) SupportsWebsocket(depth int) bool {
	return depth > 0
}

func (p *bybitOrderbookProvider) StreamOrderbook(
	ctx context.Context,
	symbol string,
	depth int,
	handler orderbookStreamHandler,
) error {
	endpoint := "wss://stream.bybit.com/v5/public/linear"
	conn, _, err := dialWebsocket(ctx, endpoint)
	if err != nil {
		return err
	}
	defer conn.Close()
	stopClose := closeConnOnContextDone(ctx, conn)
	defer stopClose()

	streamDepth := bybitStreamDepth(depth)
	topic := fmt.Sprintf("orderbook.%d.%s", streamDepth, symbol)
	subscribePayload := map[string]any{
		"op":   "subscribe",
		"args": []string{topic},
	}
	if err := writeWebsocketJSON(conn, subscribePayload); err != nil {
		return err
	}

	book := newLevelBook()
	initialized := false

	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}

		var message struct {
			Op      string `json:"op"`
			Success *bool  `json:"success"`
			RetMsg  string `json:"ret_msg"`
			Topic   string `json:"topic"`
			Type    string `json:"type"`
			TS      int64  `json:"ts"`
			CTS     int64  `json:"cts"`
			Data    struct {
				Symbol string     `json:"s"`
				Bids   [][]string `json:"b"`
				Asks   [][]string `json:"a"`
				U      int64      `json:"u"`
			} `json:"data"`
		}
		if err := json.Unmarshal(payload, &message); err != nil {
			continue
		}

		if message.Success != nil {
			if !*message.Success {
				return fmt.Errorf("bybit subscribe failed: %s", message.RetMsg)
			}
			continue
		}
		if message.Topic == "" {
			continue
		}

		switch message.Type {
		case "snapshot":
			book.resetFromPairs(message.Data.Bids, message.Data.Asks)
			initialized = true
		case "delta":
			if !initialized {
				continue
			}
			book.applyUpdates(orderbookSideBid, message.Data.Bids)
			book.applyUpdates(orderbookSideAsk, message.Data.Asks)
		default:
			continue
		}

		outBids, outAsks := book.topLevels(depth)
		exchangeTS := message.CTS
		if exchangeTS == 0 {
			exchangeTS = message.TS
		}
		handler(outBids, outAsks, exchangeTS, string(payload))
	}
}

func bybitStreamDepth(depth int) int {
	if depth <= 1 {
		return 1
	}
	if depth <= 50 {
		return 50
	}
	if depth <= 200 {
		return 200
	}
	return 1000
}

func (p *bybitOrderbookProvider) FetchOrderbook(ctx context.Context, symbol string, depth int) ([]OrderbookLevel, []OrderbookLevel, int64, string, error) {
	endpoint := fmt.Sprintf(
		"https://api.bybit.com/v5/market/orderbook?category=linear&symbol=%s&limit=%d",
		url.QueryEscape(symbol),
		depth,
	)
	payload, raw, err := fetchJSON(ctx, p.client, endpoint)
	if err != nil {
		return nil, nil, 0, "", err
	}
	if retCode := asInt64(payload["retCode"]); retCode != 0 {
		return nil, nil, 0, "", fmt.Errorf("bybit api error: %s", asString(payload["retMsg"]))
	}

	resultObj, ok := payload["result"].(map[string]any)
	if !ok {
		return nil, nil, 0, "", fmt.Errorf("bybit response result type invalid")
	}
	bids, err := parseOrderbookLevels(resultObj["b"], depth)
	if err != nil {
		return nil, nil, 0, "", err
	}
	asks, err := parseOrderbookLevels(resultObj["a"], depth)
	if err != nil {
		return nil, nil, 0, "", err
	}

	return bids, asks, asInt64(resultObj["ts"]), raw, nil
}

func fetchJSON(ctx context.Context, client *http.Client, endpoint string) (map[string]any, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "dex-orderbook-collector/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("request failed (%d): %s", resp.StatusCode, string(raw))
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, "", err
	}
	return payload, string(raw), nil
}

func parseOrderbookLevels(raw any, limit int) ([]OrderbookLevel, error) {
	if raw == nil {
		return nil, nil
	}

	levelRows, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid level payload type: %T", raw)
	}

	out := make([]OrderbookLevel, 0, minInt(limit, len(levelRows)))
	for idx, row := range levelRows {
		if idx >= limit {
			break
		}

		values, ok := asStringSlice(row)
		if !ok || len(values) < 2 {
			continue
		}

		out = append(out, OrderbookLevel{
			Level:    idx,
			Price:    values[0],
			Quantity: values[1],
		})
	}
	return out, nil
}

func asStringSlice(raw any) ([]string, bool) {
	switch value := raw.(type) {
	case []any:
		if len(value) < 2 {
			return nil, false
		}
		out := make([]string, 0, len(value))
		for _, item := range value {
			out = append(out, asString(item))
		}
		return out, true
	case []string:
		return value, true
	default:
		return nil, false
	}
}

func asString(raw any) string {
	switch value := raw.(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	case float64:
		if float64(int64(value)) == value {
			return strconv.FormatInt(int64(value), 10)
		}
		return strconv.FormatFloat(value, 'f', -1, 64)
	case int:
		return strconv.FormatInt(int64(value), 10)
	case int64:
		return strconv.FormatInt(value, 10)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", value)
	}
}

func asInt64(raw any) int64 {
	switch value := raw.(type) {
	case nil:
		return 0
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	case string:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0
		}
		return parsed
	case fmt.Stringer:
		parsed, err := strconv.ParseInt(value.String(), 10, 64)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func parseCoinbaseTime(raw any) int64 {
	asText := strings.TrimSpace(asString(raw))
	if asText == "" {
		return 0
	}

	if parsed, err := time.Parse(time.RFC3339Nano, asText); err == nil {
		return parsed.UnixMilli()
	}

	return asInt64(raw)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
