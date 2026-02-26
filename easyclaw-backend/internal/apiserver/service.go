package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/coldbell/dex/backend/internal/config"
	"github.com/coldbell/dex/backend/internal/indexer"
)

type Service struct {
	cfg              config.APIServerConfig
	logger           *slog.Logger
	store            *indexer.Store
	allowAllOrigins  bool
	allowedOriginSet map[string]struct{}
}

func New(cfg config.APIServerConfig, logger *slog.Logger) (*Service, error) {
	store, err := indexer.NewStore(cfg.DBDSN)
	if err != nil {
		return nil, fmt.Errorf("init store: %w", err)
	}

	allowAllOrigins := false
	allowedOriginSet := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" {
			continue
		}
		if trimmed == "*" {
			allowAllOrigins = true
			continue
		}
		allowedOriginSet[trimmed] = struct{}{}
	}
	if len(allowedOriginSet) == 0 && !allowAllOrigins {
		allowAllOrigins = true
	}

	return &Service{
		cfg:              cfg,
		logger:           logger,
		store:            store,
		allowAllOrigins:  allowAllOrigins,
		allowedOriginSet: allowedOriginSet,
	}, nil
}

func (s *Service) Run(ctx context.Context) error {
	defer func() {
		if err := s.store.Close(); err != nil {
			s.logger.Error("failed to close store", "err", err)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/api/v1/positions", s.handlePositions)
	mux.HandleFunc("/api/v1/orders", s.handleOrders)
	mux.HandleFunc("/api/v1/fills", s.handleFills)
	mux.HandleFunc("/api/v1/position-history", s.handlePositionHistory)
	mux.HandleFunc("/api/v1/orderbook-heatmap", s.handleOrderbookHeatmap)
	mux.HandleFunc("/api/v1/orderbook-heatmap-aggregated", s.handleOrderbookHeatmapAggregated)
	mux.HandleFunc("/v1/auth/challenge", s.handleAuthChallenge)
	mux.HandleFunc("/v1/auth/verify-signature", s.handleAuthVerifySignature)
	mux.HandleFunc("/v1/auth/session/refresh", s.handleAuthSessionRefresh)
	mux.HandleFunc("/v1/agents", s.handleAgentsRoot)
	mux.HandleFunc("/v1/agents/", s.handleAgentsSubroutes)
	mux.HandleFunc("/v1/safety/kill-switch", s.handleKillSwitch)
	mux.HandleFunc("/v1/strategy/templates", s.handleStrategyTemplates)
	mux.HandleFunc("/v1/strategies", s.handleStrategiesRoot)
	mux.HandleFunc("/v1/strategies/", s.handleStrategiesSubroutes)
	mux.HandleFunc("/v1/portfolio/agents/", s.handlePortfolioAgent)
	mux.HandleFunc("/v1/portfolio", s.handlePortfolio)
	mux.HandleFunc("/v1/chart/candles", s.handleChartCandles)
	mux.HandleFunc("/v1/trades", s.handleTrades)
	mux.HandleFunc("/v1/leaderboard", s.handleLeaderboard)
	mux.HandleFunc("/v1/system/status", s.handleSystemStatus)
	mux.HandleFunc("/ws", s.handleWebsocket)

	handler := s.withCORS(mux)
	server := &http.Server{
		Addr:         s.cfg.ListenAddr,
		Handler:      handler,
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
		IdleTimeout:  s.cfg.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			errCh <- nil
			return
		}
		errCh <- err
	}()

	s.logger.Info("api-server started",
		"listen_addr", s.cfg.ListenAddr,
		"db_driver", "postgres",
		"allowed_origins", strings.Join(s.cfg.AllowedOrigins, ","),
	)

	select {
	case <-ctx.Done():
		s.logger.Info("api-server stopping")
		if err := server.Shutdown(context.Background()); err != nil {
			return fmt.Errorf("shutdown api-server: %w", err)
		}
		return <-errCh
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("listen and serve: %w", err)
		}
		return nil
	}
}

type listResponse[T any] struct {
	Items  []T `json:"items"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

type healthResponse struct {
	OK bool `json:"ok"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondMethodNotAllowed(w)
		return
	}
	s.respondJSON(w, http.StatusOK, healthResponse{OK: true})
}

func (s *Service) handlePositions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondMethodNotAllowed(w)
		return
	}

	marketID, err := parseOptionalUint64(r, "market_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := parseOptionalInt(r, "limit", 0)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	offset, err := parseOptionalInt(r, "offset", 0)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	items, normalizedLimit, normalizedOffset, err := s.store.ListPositions(r.Context(), indexer.PositionFilter{
		UserMargin: strings.TrimSpace(r.URL.Query().Get("user_margin")),
		MarketID:   marketID,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		s.logger.Error("list positions failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to list positions")
		return
	}

	s.respondJSON(w, http.StatusOK, listResponse[indexer.PositionRecord]{
		Items:  items,
		Limit:  normalizedLimit,
		Offset: normalizedOffset,
	})
}

func (s *Service) handleOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondMethodNotAllowed(w)
		return
	}

	marketID, err := parseOptionalUint64(r, "market_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := parseOptionalInt(r, "limit", 0)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	offset, err := parseOptionalInt(r, "offset", 0)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	items, normalizedLimit, normalizedOffset, err := s.store.ListOrders(r.Context(), indexer.OrderFilter{
		UserMargin: strings.TrimSpace(r.URL.Query().Get("user_margin")),
		UserPubkey: strings.TrimSpace(r.URL.Query().Get("user_pubkey")),
		MarketID:   marketID,
		Status:     strings.TrimSpace(r.URL.Query().Get("status")),
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		s.logger.Error("list orders failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to list orders")
		return
	}

	s.respondJSON(w, http.StatusOK, listResponse[indexer.OrderRecord]{
		Items:  items,
		Limit:  normalizedLimit,
		Offset: normalizedOffset,
	})
}

func (s *Service) handleFills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondMethodNotAllowed(w)
		return
	}

	marketID, err := parseOptionalUint64(r, "market_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := parseOptionalInt(r, "limit", 0)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	offset, err := parseOptionalInt(r, "offset", 0)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	items, normalizedLimit, normalizedOffset, err := s.store.ListFills(r.Context(), indexer.FillFilter{
		UserMargin: strings.TrimSpace(r.URL.Query().Get("user_margin")),
		UserPubkey: strings.TrimSpace(r.URL.Query().Get("user_pubkey")),
		MarketID:   marketID,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		s.logger.Error("list fills failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to list fills")
		return
	}

	s.respondJSON(w, http.StatusOK, listResponse[indexer.FillRecord]{
		Items:  items,
		Limit:  normalizedLimit,
		Offset: normalizedOffset,
	})
}

func (s *Service) handlePositionHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondMethodNotAllowed(w)
		return
	}

	marketID, err := parseOptionalUint64(r, "market_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := parseOptionalInt(r, "limit", 0)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	offset, err := parseOptionalInt(r, "offset", 0)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	items, normalizedLimit, normalizedOffset, err := s.store.ListPositionHistory(r.Context(), indexer.PositionHistoryFilter{
		UserMargin: strings.TrimSpace(r.URL.Query().Get("user_margin")),
		MarketID:   marketID,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		s.logger.Error("list position history failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to list position history")
		return
	}

	s.respondJSON(w, http.StatusOK, listResponse[indexer.PositionHistoryRecord]{
		Items:  items,
		Limit:  normalizedLimit,
		Offset: normalizedOffset,
	})
}

func (s *Service) handleOrderbookHeatmap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondMethodNotAllowed(w)
		return
	}

	exchange := strings.TrimSpace(r.URL.Query().Get("exchange"))
	symbol := strings.TrimSpace(r.URL.Query().Get("symbol"))
	if exchange == "" || symbol == "" {
		s.respondError(w, http.StatusBadRequest, "exchange and symbol are required")
		return
	}

	from, err := parseOptionalInt64(r, "from", 0)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	to, err := parseOptionalInt64(r, "to", 0)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if from != 0 && to != 0 && from > to {
		s.respondError(w, http.StatusBadRequest, "from must be <= to")
		return
	}

	limit, err := parseOptionalInt(r, "limit", 0)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	offset, err := parseOptionalInt(r, "offset", 0)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	items, normalizedLimit, normalizedOffset, err := s.store.ListOrderbookHeatmap(r.Context(), indexer.OrderbookHeatmapFilter{
		Exchange: exchange,
		Symbol:   symbol,
		FromUnix: from,
		ToUnix:   to,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		s.logger.Error("list orderbook heatmap failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to list orderbook heatmap")
		return
	}

	s.respondJSON(w, http.StatusOK, listResponse[indexer.OrderbookHeatmapRecord]{
		Items:  items,
		Limit:  normalizedLimit,
		Offset: normalizedOffset,
	})
}

func (s *Service) handleOrderbookHeatmapAggregated(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondMethodNotAllowed(w)
		return
	}

	symbolKey := strings.TrimSpace(r.URL.Query().Get("symbol_key"))
	if symbolKey == "" {
		s.respondError(w, http.StatusBadRequest, "symbol_key is required")
		return
	}

	from, err := parseOptionalInt64(r, "from", 0)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	to, err := parseOptionalInt64(r, "to", 0)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if from != 0 && to != 0 && from > to {
		s.respondError(w, http.StatusBadRequest, "from must be <= to")
		return
	}

	limit, err := parseOptionalInt(r, "limit", 0)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	offset, err := parseOptionalInt(r, "offset", 0)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	items, normalizedLimit, normalizedOffset, err := s.store.ListOrderbookHeatmapAggregated(
		r.Context(),
		indexer.OrderbookHeatmapAggregateFilter{
			SymbolKey: symbolKey,
			FromUnix:  from,
			ToUnix:    to,
			Limit:     limit,
			Offset:    offset,
		},
	)
	if err != nil {
		s.logger.Error("list aggregated orderbook heatmap failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to list aggregated orderbook heatmap")
		return
	}

	s.respondJSON(w, http.StatusOK, listResponse[indexer.OrderbookHeatmapAggregatePoint]{
		Items:  items,
		Limit:  normalizedLimit,
		Offset: normalizedOffset,
	})
}

func (s *Service) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" {
			allowed := s.allowAllOrigins
			if !allowed {
				_, allowed = s.allowedOriginSet[origin]
			}

			if allowed {
				if s.allowAllOrigins {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Max-Age", "300")
			}
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func parseOptionalUint64(r *http.Request, key string) (*uint64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", key, err)
	}
	return &value, nil
}

func parseOptionalInt(r *http.Request, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return value, nil
}

func parseOptionalInt64(r *http.Request, key string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return value, nil
}

func (s *Service) respondMethodNotAllowed(w http.ResponseWriter) {
	s.respondError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (s *Service) respondError(w http.ResponseWriter, code int, message string) {
	s.respondJSON(w, code, errorResponse{Error: message})
}

func (s *Service) respondJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.logger.Error("failed to write JSON response", "err", err)
	}
}
