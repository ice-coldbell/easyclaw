package indexer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	orderengine "github.com/coldbell/dex/backend/internal/anchor/order_engine"
)

const (
	defaultTradeFeeRate = 0.0004
	defaultAgentEquity  = 10000.0
	rawPriceScale       = 1_000_000.0
	rawNotionalScale    = 1_000_000.0
)

var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
)

type AuthChallengeRecord struct {
	ID           string
	WalletPubkey string
	Intent       string
	Message      string
	CreatedAt    int64
	ExpiresAt    int64
	UsedAt       *int64
}

type AuthSessionRecord struct {
	TokenHash    string
	WalletPubkey string
	CreatedAt    int64
	ExpiresAt    int64
	RefreshedAt  int64
	RevokedAt    *int64
}

type AgentRiskProfile struct {
	MaxPositionUSDC    float64 `json:"max_position_usdc"`
	DailyLossLimitUSDC float64 `json:"daily_loss_limit_usdc"`
	KillSwitchEnabled  bool    `json:"kill_switch_enabled"`
}

type AgentRecord struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	StrategyID  string           `json:"strategy_id"`
	Status      string           `json:"status"`
	OwnerPubkey string           `json:"owner_pubkey"`
	RiskProfile AgentRiskProfile `json:"risk_profile"`
	CreatedAt   int64            `json:"created_at"`
}

type AgentBindingRecord struct {
	Bound            bool   `json:"bound"`
	OwnerPubkey      string `json:"owner_pubkey"`
	BoundAt          *int64 `json:"bound_at"`
	SessionExpiresAt *int64 `json:"session_expires_at"`
}

type CreateAgentInput struct {
	ID          string
	Name        string
	StrategyID  string
	OwnerPubkey string
	RiskProfile AgentRiskProfile
	CreatedAt   int64
}

type StartAgentSessionInput struct {
	ID        string
	AgentID   string
	Mode      string
	CreatedBy string
	StartedAt int64
}

type StrategyTemplateRecord struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	RiskLevel   string         `json:"risk_level"`
	Rules       map[string]any `json:"rules"`
}

type StrategyRecord struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	EntryRules   map[string]any `json:"entry_rules"`
	ExitRules    map[string]any `json:"exit_rules"`
	RiskDefaults map[string]any `json:"risk_defaults"`
	OwnerPubkey  string         `json:"owner_pubkey"`
	IsPublished  bool           `json:"is_published"`
	PublishedAt  *int64         `json:"published_at"`
	CreatedAt    int64          `json:"created_at"`
	UpdatedAt    int64          `json:"updated_at"`
}

type CreateStrategyInput struct {
	ID           string
	Name         string
	EntryRules   map[string]any
	ExitRules    map[string]any
	RiskDefaults map[string]any
	OwnerPubkey  string
	CreatedAt    int64
}

type StrategyPatch struct {
	Name       *string
	EntryRules *map[string]any
	ExitRules  *map[string]any
	UpdatedAt  int64
}

type RiskPatch struct {
	MaxPositionUSDC    *float64
	DailyLossLimitUSDC *float64
	KillSwitchEnabled  *bool
	UpdatedAt          int64
}

type TradeFilter struct {
	AgentID  string
	FromUnix int64
	ToUnix   int64
	Limit    int
	Offset   int
}

type TradeRecord struct {
	ID         string  `json:"id"`
	AgentID    string  `json:"agent_id"`
	AgentName  string  `json:"agent_name"`
	Side       string  `json:"side"`
	EntryPrice float64 `json:"entry_price"`
	ExitPrice  float64 `json:"exit_price"`
	Qty        float64 `json:"qty"`
	Fee        float64 `json:"fee"`
	Pnl        float64 `json:"pnl"`
	TxSig      string  `json:"tx_sig"`
	EntryTime  int64   `json:"entry_time"`
	ExitTime   int64   `json:"exit_time"`
	MarketID   uint64  `json:"market_id"`
}

type EquityPoint struct {
	TS    int64   `json:"ts"`
	Value float64 `json:"value"`
}

type DailyPNLPoint struct {
	Date string  `json:"date"`
	PNL  float64 `json:"pnl"`
}

type PortfolioAgentSummary struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	PnlPct      float64 `json:"pnl_pct"`
	PnlAbs      float64 `json:"pnl_abs"`
	WinRate     float64 `json:"win_rate"`
	TotalTrades int     `json:"total_trades"`
	Drawdown    float64 `json:"drawdown"`
	Equity      float64 `json:"equity"`
}

type PortfolioSummary struct {
	TotalEquity   float64                 `json:"total_equity"`
	TotalPNL      float64                 `json:"total_pnl"`
	TotalPNLPct   float64                 `json:"total_pnl_pct"`
	MaxDrawdown   float64                 `json:"max_drawdown"`
	ActiveAgents  int                     `json:"active_agents"`
	EquityHistory []EquityPoint           `json:"equity_history"`
	Agents        []PortfolioAgentSummary `json:"agents"`
}

type AgentPortfolioSummary struct {
	PnlPct        float64         `json:"pnl_pct"`
	PnlAbs        float64         `json:"pnl_abs"`
	WinRate       float64         `json:"win_rate"`
	TotalTrades   int             `json:"total_trades"`
	Drawdown      float64         `json:"drawdown"`
	DailyPNL      []DailyPNLPoint `json:"daily_pnl"`
	EquityHistory []EquityPoint   `json:"equity_history"`
}

type LeaderboardItem struct {
	Rank        int       `json:"rank"`
	RankChange  int       `json:"rank_change"`
	AgentID     string    `json:"agent_id"`
	AgentName   string    `json:"agent_name"`
	WinRate     float64   `json:"win_rate"`
	PnlPct      float64   `json:"pnl_pct"`
	TotalTrades int       `json:"total_trades"`
	MaxDrawdown float64   `json:"max_drawdown"`
	Sparkline   []float64 `json:"sparkline"`
}

type SystemStatusRecord struct {
	DexConnected    bool   `json:"dex_connected"`
	WSLatencyMS     int64  `json:"ws_latency_ms"`
	LastIndexedSlot uint64 `json:"last_indexed_slot"`
	IndexerLagMS    int64  `json:"indexer_lag_ms"`
}

type CandleRecord struct {
	TS     int64   `json:"ts"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

type tradeEvent struct {
	FillID     int64
	AgentID    string
	AgentName  string
	Side       string
	Price      float64
	Notional   float64
	Qty        float64
	OrderType  string
	CreatedAt  int64
	ExecutedAt int64
	MarketID   uint64
	TxSig      string
}

type positionState struct {
	Qty      float64
	AvgPrice float64
}

type agentComputed struct {
	Agent         AgentRecord
	Trades        []TradeRecord
	PnlAbs        float64
	PnlPct        float64
	WinRate       float64
	TotalTrades   int
	Drawdown      float64
	Equity        float64
	DailyPNL      []DailyPNLPoint
	EquityHistory []EquityPoint
	Sparkline     []float64
}

func defaultRiskProfile() AgentRiskProfile {
	return AgentRiskProfile{
		MaxPositionUSDC:    5000,
		DailyLossLimitUSDC: 500,
		KillSwitchEnabled:  true,
	}
}

func (s *Store) CreateAuthChallenge(ctx context.Context, challenge AuthChallengeRecord) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO auth_challenges (id, wallet_pubkey, intent, message, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   wallet_pubkey = excluded.wallet_pubkey,
		   intent = excluded.intent,
		   message = excluded.message,
		   created_at = excluded.created_at,
		   expires_at = excluded.expires_at,
		   used_at = NULL`,
		challenge.ID,
		challenge.WalletPubkey,
		challenge.Intent,
		challenge.Message,
		challenge.CreatedAt,
		challenge.ExpiresAt,
	)
	return err
}

func (s *Store) GetAuthChallenge(ctx context.Context, challengeID string) (AuthChallengeRecord, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, wallet_pubkey, intent, message, created_at, expires_at, used_at
		 FROM auth_challenges
		 WHERE id = ?`,
		challengeID,
	)
	var out AuthChallengeRecord
	var usedAt sql.NullInt64
	if err := row.Scan(
		&out.ID,
		&out.WalletPubkey,
		&out.Intent,
		&out.Message,
		&out.CreatedAt,
		&out.ExpiresAt,
		&usedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AuthChallengeRecord{}, ErrNotFound
		}
		return AuthChallengeRecord{}, err
	}
	if usedAt.Valid {
		out.UsedAt = &usedAt.Int64
	}
	return out, nil
}

func (s *Store) MarkAuthChallengeUsed(ctx context.Context, challengeID string, usedAt int64) error {
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE auth_challenges
		 SET used_at = ?
		 WHERE id = ?
		   AND used_at IS NULL`,
		usedAt,
		challengeID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateAuthSession(ctx context.Context, tokenHash, walletPubkey string, createdAt, expiresAt int64) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO auth_sessions (token_hash, wallet_pubkey, created_at, expires_at, refreshed_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(token_hash) DO UPDATE SET
		   wallet_pubkey = excluded.wallet_pubkey,
		   created_at = excluded.created_at,
		   expires_at = excluded.expires_at,
		   refreshed_at = excluded.refreshed_at,
		   revoked_at = NULL`,
		tokenHash,
		walletPubkey,
		createdAt,
		expiresAt,
		createdAt,
	)
	return err
}

func (s *Store) GetAuthSession(ctx context.Context, tokenHash string) (AuthSessionRecord, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT token_hash, wallet_pubkey, created_at, expires_at, refreshed_at, revoked_at
		 FROM auth_sessions
		 WHERE token_hash = ?`,
		tokenHash,
	)
	var out AuthSessionRecord
	var revokedAt sql.NullInt64
	if err := row.Scan(
		&out.TokenHash,
		&out.WalletPubkey,
		&out.CreatedAt,
		&out.ExpiresAt,
		&out.RefreshedAt,
		&revokedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AuthSessionRecord{}, ErrUnauthorized
		}
		return AuthSessionRecord{}, err
	}
	if revokedAt.Valid {
		out.RevokedAt = &revokedAt.Int64
	}
	return out, nil
}

func (s *Store) RotateAuthSession(ctx context.Context, oldTokenHash, newTokenHash string, now, expiresAt int64) (AuthSessionRecord, error) {
	var out AuthSessionRecord
	err := s.WithTx(ctx, func(tx *Tx) error {
		row := tx.QueryRowContext(
			ctx,
			`SELECT wallet_pubkey, created_at, expires_at, revoked_at
			 FROM auth_sessions
			 WHERE token_hash = ?`,
			oldTokenHash,
		)
		var wallet string
		var createdAt int64
		var oldExpires int64
		var revokedAt sql.NullInt64
		if err := row.Scan(&wallet, &createdAt, &oldExpires, &revokedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrUnauthorized
			}
			return err
		}
		if revokedAt.Valid || oldExpires <= now {
			return ErrUnauthorized
		}

		if _, err := tx.ExecContext(
			ctx,
			`UPDATE auth_sessions
			 SET revoked_at = ?
			 WHERE token_hash = ?`,
			now,
			oldTokenHash,
		); err != nil {
			return err
		}

		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO auth_sessions (token_hash, wallet_pubkey, created_at, expires_at, refreshed_at)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(token_hash) DO UPDATE SET
			   wallet_pubkey = excluded.wallet_pubkey,
			   created_at = excluded.created_at,
			   expires_at = excluded.expires_at,
			   refreshed_at = excluded.refreshed_at,
			   revoked_at = NULL`,
			newTokenHash,
			wallet,
			now,
			expiresAt,
			now,
		); err != nil {
			return err
		}

		out = AuthSessionRecord{
			TokenHash:    newTokenHash,
			WalletPubkey: wallet,
			CreatedAt:    now,
			ExpiresAt:    expiresAt,
			RefreshedAt:  now,
		}
		_ = createdAt
		return nil
	})
	if err != nil {
		return AuthSessionRecord{}, err
	}
	return out, nil
}

func (s *Store) CreateAgent(ctx context.Context, input CreateAgentInput) (string, error) {
	if input.RiskProfile.MaxPositionUSDC == 0 &&
		input.RiskProfile.DailyLossLimitUSDC == 0 &&
		!input.RiskProfile.KillSwitchEnabled {
		input.RiskProfile = defaultRiskProfile()
	}

	riskJSON, err := json.Marshal(input.RiskProfile)
	if err != nil {
		return "", err
	}
	status := "stopped"
	if strings.TrimSpace(input.OwnerPubkey) == "" {
		input.OwnerPubkey = "unbound"
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO agents (
		 id, name, strategy_id, status, owner_pubkey, risk_profile_json,
		 bound_at, session_expires_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?)`,
		input.ID,
		input.Name,
		input.StrategyID,
		status,
		input.OwnerPubkey,
		string(riskJSON),
		input.CreatedAt,
		input.CreatedAt,
	)
	if err != nil {
		return "", err
	}
	return input.ID, nil
}

func (s *Store) ListAgents(ctx context.Context) ([]AgentRecord, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, name, strategy_id, status, owner_pubkey, risk_profile_json, created_at
		 FROM agents
		 ORDER BY created_at DESC, id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]AgentRecord, 0, 32)
	for rows.Next() {
		var item AgentRecord
		var riskRaw string
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.StrategyID,
			&item.Status,
			&item.OwnerPubkey,
			&riskRaw,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		item.RiskProfile = parseRiskProfile(riskRaw)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) GetAgent(ctx context.Context, agentID string) (AgentRecord, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, name, strategy_id, status, owner_pubkey, risk_profile_json, created_at
		 FROM agents
		 WHERE id = ?`,
		agentID,
	)
	var out AgentRecord
	var riskRaw string
	if err := row.Scan(
		&out.ID,
		&out.Name,
		&out.StrategyID,
		&out.Status,
		&out.OwnerPubkey,
		&riskRaw,
		&out.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AgentRecord{}, ErrNotFound
		}
		return AgentRecord{}, err
	}
	out.RiskProfile = parseRiskProfile(riskRaw)
	return out, nil
}

func (s *Store) GetAgentBinding(ctx context.Context, agentID string) (AgentBindingRecord, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT owner_pubkey, bound_at, session_expires_at
		 FROM agents
		 WHERE id = ?`,
		agentID,
	)
	var owner string
	var boundAt sql.NullInt64
	var sessionExpires sql.NullInt64
	if err := row.Scan(&owner, &boundAt, &sessionExpires); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AgentBindingRecord{}, ErrNotFound
		}
		return AgentBindingRecord{}, err
	}
	out := AgentBindingRecord{
		Bound:       boundAt.Valid && owner != "" && owner != "unbound",
		OwnerPubkey: owner,
	}
	if boundAt.Valid {
		out.BoundAt = &boundAt.Int64
	}
	if sessionExpires.Valid {
		out.SessionExpiresAt = &sessionExpires.Int64
	}
	return out, nil
}

func (s *Store) RebindAgentOwner(ctx context.Context, agentID, ownerPubkey string, boundAt, sessionExpiresAt int64) error {
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE agents
		 SET owner_pubkey = ?,
		     bound_at = ?,
		     session_expires_at = ?,
		     updated_at = ?
		 WHERE id = ?`,
		ownerPubkey,
		boundAt,
		sessionExpiresAt,
		boundAt,
		agentID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) StartAgentSession(ctx context.Context, input StartAgentSessionInput) (string, int64, error) {
	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	if mode != "paper" && mode != "live" {
		return "", 0, fmt.Errorf("invalid mode")
	}
	status := mode
	err := s.WithTx(ctx, func(tx *Tx) error {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE agent_sessions
			 SET status = 'stopped', stopped_at = ?
			 WHERE agent_id = ? AND status = 'active'`,
			input.StartedAt,
			input.AgentID,
		); err != nil {
			return err
		}

		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO agent_sessions (id, agent_id, mode, status, started_at, created_by, created_at)
			 VALUES (?, ?, ?, 'active', ?, ?, ?)`,
			input.ID,
			input.AgentID,
			mode,
			input.StartedAt,
			input.CreatedBy,
			input.StartedAt,
		); err != nil {
			return err
		}

		result, err := tx.ExecContext(
			ctx,
			`UPDATE agents
			 SET status = ?, updated_at = ?
			 WHERE id = ?`,
			status,
			input.StartedAt,
			input.AgentID,
		)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err == nil && affected == 0 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return "", 0, err
	}
	return input.ID, input.StartedAt, nil
}

func (s *Store) StopAgentSession(ctx context.Context, agentID, sessionID string, stoppedAt int64) error {
	return s.WithTx(ctx, func(tx *Tx) error {
		result, err := tx.ExecContext(
			ctx,
			`UPDATE agent_sessions
			 SET status = 'stopped', stopped_at = ?
			 WHERE id = ? AND agent_id = ? AND status = 'active'`,
			stoppedAt,
			sessionID,
			agentID,
		)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err == nil && affected == 0 {
			return ErrNotFound
		}

		if _, err := tx.ExecContext(
			ctx,
			`UPDATE agents
			 SET status = 'stopped', updated_at = ?
			 WHERE id = ?`,
			stoppedAt,
			agentID,
		); err != nil {
			return err
		}
		return nil
	})
}

func (s *Store) KillSwitch(ctx context.Context, allAgents bool, agentIDs []string, stoppedAt int64) (int, error) {
	if !allAgents && len(agentIDs) == 0 {
		return 0, nil
	}

	var whereClause string
	args := []any{stoppedAt}
	if allAgents {
		whereClause = "WHERE status = 'active'"
	} else {
		placeholders := make([]string, 0, len(agentIDs))
		for _, id := range agentIDs {
			placeholders = append(placeholders, "?")
			args = append(args, id)
		}
		whereClause = fmt.Sprintf("WHERE status = 'active' AND agent_id IN (%s)", strings.Join(placeholders, ","))
	}

	query := fmt.Sprintf(
		`UPDATE agent_sessions
		 SET status = 'stopped', stopped_at = ?
		 %s`,
		whereClause,
	)
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	if allAgents {
		if _, err := s.db.ExecContext(
			ctx,
			`UPDATE agents
			 SET status = 'stopped', updated_at = ?`,
			stoppedAt,
		); err != nil {
			return 0, err
		}
	} else {
		updateArgs := []any{stoppedAt}
		placeholders := make([]string, 0, len(agentIDs))
		for _, id := range agentIDs {
			placeholders = append(placeholders, "?")
			updateArgs = append(updateArgs, id)
		}
		updateQuery := fmt.Sprintf(
			`UPDATE agents
			 SET status = 'stopped', updated_at = ?
			 WHERE id IN (%s)`,
			strings.Join(placeholders, ","),
		)
		if _, err := s.db.ExecContext(ctx, updateQuery, updateArgs...); err != nil {
			return 0, err
		}
	}

	return int(affected), nil
}

func (s *Store) GetAgentRisk(ctx context.Context, agentID string) (AgentRiskProfile, error) {
	row := s.db.QueryRowContext(ctx, `SELECT risk_profile_json FROM agents WHERE id = ?`, agentID)
	var riskRaw string
	if err := row.Scan(&riskRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AgentRiskProfile{}, ErrNotFound
		}
		return AgentRiskProfile{}, err
	}
	return parseRiskProfile(riskRaw), nil
}

func (s *Store) PatchAgentRisk(ctx context.Context, agentID string, patch RiskPatch) (AgentRiskProfile, error) {
	current, err := s.GetAgentRisk(ctx, agentID)
	if err != nil {
		return AgentRiskProfile{}, err
	}
	if patch.MaxPositionUSDC != nil {
		current.MaxPositionUSDC = *patch.MaxPositionUSDC
	}
	if patch.DailyLossLimitUSDC != nil {
		current.DailyLossLimitUSDC = *patch.DailyLossLimitUSDC
	}
	if patch.KillSwitchEnabled != nil {
		current.KillSwitchEnabled = *patch.KillSwitchEnabled
	}

	riskJSON, err := json.Marshal(current)
	if err != nil {
		return AgentRiskProfile{}, err
	}
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE agents
		 SET risk_profile_json = ?, updated_at = ?
		 WHERE id = ?`,
		string(riskJSON),
		patch.UpdatedAt,
		agentID,
	)
	if err != nil {
		return AgentRiskProfile{}, err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return AgentRiskProfile{}, ErrNotFound
	}
	return current, nil
}

func (s *Store) ListStrategyTemplates(ctx context.Context) ([]StrategyTemplateRecord, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, name, description, risk_level, rules_json
		 FROM strategy_templates
		 ORDER BY id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]StrategyTemplateRecord, 0, 16)
	for rows.Next() {
		var item StrategyTemplateRecord
		var rulesRaw string
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Description,
			&item.RiskLevel,
			&rulesRaw,
		); err != nil {
			return nil, err
		}
		item.Rules = parseJSONMap(rulesRaw)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) CreateStrategy(ctx context.Context, input CreateStrategyInput) (string, error) {
	entryJSON, err := json.Marshal(input.EntryRules)
	if err != nil {
		return "", err
	}
	exitJSON, err := json.Marshal(input.ExitRules)
	if err != nil {
		return "", err
	}
	riskJSON, err := json.Marshal(input.RiskDefaults)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(input.OwnerPubkey) == "" {
		input.OwnerPubkey = "unbound"
	}

	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO strategies (
		 id, name, entry_rules_json, exit_rules_json, risk_defaults_json,
		 owner_pubkey, is_published, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		input.ID,
		input.Name,
		string(entryJSON),
		string(exitJSON),
		string(riskJSON),
		input.OwnerPubkey,
		input.CreatedAt,
		input.CreatedAt,
	)
	if err != nil {
		return "", err
	}
	return input.ID, nil
}

func (s *Store) GetStrategy(ctx context.Context, strategyID string) (StrategyRecord, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, name, entry_rules_json, exit_rules_json, risk_defaults_json,
		        owner_pubkey, is_published, published_at, created_at, updated_at
		 FROM strategies
		 WHERE id = ?`,
		strategyID,
	)

	var out StrategyRecord
	var entryRaw string
	var exitRaw string
	var riskRaw string
	var publishedInt int
	var publishedAt sql.NullInt64
	if err := row.Scan(
		&out.ID,
		&out.Name,
		&entryRaw,
		&exitRaw,
		&riskRaw,
		&out.OwnerPubkey,
		&publishedInt,
		&publishedAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return StrategyRecord{}, ErrNotFound
		}
		return StrategyRecord{}, err
	}
	out.EntryRules = parseJSONMap(entryRaw)
	out.ExitRules = parseJSONMap(exitRaw)
	out.RiskDefaults = parseJSONMap(riskRaw)
	out.IsPublished = publishedInt != 0
	if publishedAt.Valid {
		out.PublishedAt = &publishedAt.Int64
	}
	return out, nil
}

func (s *Store) PatchStrategy(ctx context.Context, strategyID string, patch StrategyPatch) (StrategyRecord, error) {
	current, err := s.GetStrategy(ctx, strategyID)
	if err != nil {
		return StrategyRecord{}, err
	}

	if patch.Name != nil {
		current.Name = strings.TrimSpace(*patch.Name)
	}
	if patch.EntryRules != nil {
		current.EntryRules = *patch.EntryRules
	}
	if patch.ExitRules != nil {
		current.ExitRules = *patch.ExitRules
	}

	entryJSON, err := json.Marshal(current.EntryRules)
	if err != nil {
		return StrategyRecord{}, err
	}
	exitJSON, err := json.Marshal(current.ExitRules)
	if err != nil {
		return StrategyRecord{}, err
	}

	result, err := s.db.ExecContext(
		ctx,
		`UPDATE strategies
		 SET name = ?, entry_rules_json = ?, exit_rules_json = ?, updated_at = ?
		 WHERE id = ?`,
		current.Name,
		string(entryJSON),
		string(exitJSON),
		patch.UpdatedAt,
		strategyID,
	)
	if err != nil {
		return StrategyRecord{}, err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return StrategyRecord{}, ErrNotFound
	}
	return s.GetStrategy(ctx, strategyID)
}

func (s *Store) PublishStrategy(ctx context.Context, strategyID string, publishedAt int64) (int64, error) {
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE strategies
		 SET is_published = 1, published_at = ?, updated_at = ?
		 WHERE id = ?`,
		publishedAt,
		publishedAt,
		strategyID,
	)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return 0, ErrNotFound
	}
	return publishedAt, nil
}

func (s *Store) ListTrades(ctx context.Context, filter TradeFilter) ([]TradeRecord, int, int, error) {
	limit, offset := normalizePagination(filter.Limit, filter.Offset)
	events, err := s.loadTradeEvents(ctx, filter.AgentID, filter.FromUnix, filter.ToUnix)
	if err != nil {
		return nil, 0, 0, err
	}

	trades := computeTrades(events)
	sort.SliceStable(trades, func(i, j int) bool {
		if trades[i].ExitTime == trades[j].ExitTime {
			return trades[i].ID > trades[j].ID
		}
		return trades[i].ExitTime > trades[j].ExitTime
	})

	if offset >= len(trades) {
		return []TradeRecord{}, limit, offset, nil
	}
	end := offset + limit
	if end > len(trades) {
		end = len(trades)
	}

	items := make([]TradeRecord, 0, end-offset)
	for _, item := range trades[offset:end] {
		items = append(items, roundTrade(item))
	}

	return items, limit, offset, nil
}

func (s *Store) GetPortfolioSummary(ctx context.Context, period string) (PortfolioSummary, error) {
	startUnix, err := portfolioPeriodStart(period)
	if err != nil {
		return PortfolioSummary{}, err
	}

	agents, err := s.ListAgents(ctx)
	if err != nil {
		return PortfolioSummary{}, err
	}
	computed, portfolioHistory, err := s.computePortfolio(ctx, agents, startUnix, 0)
	if err != nil {
		return PortfolioSummary{}, err
	}

	activeAgents := 0
	agentSummaries := make([]PortfolioAgentSummary, 0, len(computed))
	totalPNL := 0.0
	totalEquity := 0.0
	for _, item := range computed {
		if item.Agent.Status == "paper" || item.Agent.Status == "live" {
			activeAgents++
		}
		totalPNL += item.PnlAbs
		totalEquity += item.Equity
		agentSummaries = append(agentSummaries, PortfolioAgentSummary{
			ID:          item.Agent.ID,
			Name:        item.Agent.Name,
			PnlPct:      round2(item.PnlPct),
			PnlAbs:      round2(item.PnlAbs),
			WinRate:     round2(item.WinRate),
			TotalTrades: item.TotalTrades,
			Drawdown:    round2(item.Drawdown),
			Equity:      round2(item.Equity),
		})
	}

	sort.SliceStable(agentSummaries, func(i, j int) bool {
		if agentSummaries[i].PnlPct == agentSummaries[j].PnlPct {
			return agentSummaries[i].ID < agentSummaries[j].ID
		}
		return agentSummaries[i].PnlPct > agentSummaries[j].PnlPct
	})

	baseTotal := defaultAgentEquity * float64(len(agents))
	totalPNLPct := 0.0
	if baseTotal > 0 {
		totalPNLPct = (totalPNL / baseTotal) * 100
	}

	maxDrawdown := 0.0
	if len(portfolioHistory) > 0 {
		peak := portfolioHistory[0].Value
		for _, point := range portfolioHistory {
			if point.Value > peak {
				peak = point.Value
			}
			if peak > 0 {
				dd := ((point.Value - peak) / peak) * 100
				if dd < maxDrawdown {
					maxDrawdown = dd
				}
			}
		}
	}

	for i := range portfolioHistory {
		portfolioHistory[i].Value = round2(portfolioHistory[i].Value)
	}

	return PortfolioSummary{
		TotalEquity:   round2(totalEquity),
		TotalPNL:      round2(totalPNL),
		TotalPNLPct:   round2(totalPNLPct),
		MaxDrawdown:   round2(maxDrawdown),
		ActiveAgents:  activeAgents,
		EquityHistory: portfolioHistory,
		Agents:        agentSummaries,
	}, nil
}

func (s *Store) GetAgentPortfolioSummary(ctx context.Context, agentID, period string) (AgentPortfolioSummary, error) {
	startUnix, err := portfolioPeriodStart(period)
	if err != nil {
		return AgentPortfolioSummary{}, err
	}

	agent, err := s.GetAgent(ctx, agentID)
	if err != nil {
		return AgentPortfolioSummary{}, err
	}

	computed, _, err := s.computePortfolio(ctx, []AgentRecord{agent}, startUnix, 0)
	if err != nil {
		return AgentPortfolioSummary{}, err
	}
	if len(computed) == 0 {
		return AgentPortfolioSummary{
			PnlPct:        0,
			PnlAbs:        0,
			WinRate:       0,
			TotalTrades:   0,
			Drawdown:      0,
			DailyPNL:      []DailyPNLPoint{},
			EquityHistory: []EquityPoint{},
		}, nil
	}

	entry := computed[0]
	for i := range entry.EquityHistory {
		entry.EquityHistory[i].Value = round2(entry.EquityHistory[i].Value)
	}
	for i := range entry.DailyPNL {
		entry.DailyPNL[i].PNL = round2(entry.DailyPNL[i].PNL)
	}

	return AgentPortfolioSummary{
		PnlPct:        round2(entry.PnlPct),
		PnlAbs:        round2(entry.PnlAbs),
		WinRate:       round2(entry.WinRate),
		TotalTrades:   entry.TotalTrades,
		Drawdown:      round2(entry.Drawdown),
		DailyPNL:      entry.DailyPNL,
		EquityHistory: entry.EquityHistory,
	}, nil
}

func (s *Store) GetLeaderboard(ctx context.Context, metric, period string, minTrades int) ([]LeaderboardItem, error) {
	if minTrades < 0 {
		minTrades = 0
	}
	startUnix, duration, err := leaderboardPeriodStart(period)
	if err != nil {
		return nil, err
	}
	if metric == "" {
		metric = "pnl_pct"
	}
	metric = strings.ToLower(metric)
	if metric != "win_rate" && metric != "pnl_pct" {
		return nil, fmt.Errorf("invalid metric")
	}

	agents, err := s.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	computed, _, err := s.computePortfolio(ctx, agents, startUnix, 0)
	if err != nil {
		return nil, err
	}

	currentItems := make([]agentComputed, 0, len(computed))
	for _, item := range computed {
		if item.TotalTrades < minTrades {
			continue
		}
		currentItems = append(currentItems, item)
	}

	sort.SliceStable(currentItems, func(i, j int) bool {
		if metric == "win_rate" {
			if currentItems[i].WinRate == currentItems[j].WinRate {
				return currentItems[i].PnlPct > currentItems[j].PnlPct
			}
			return currentItems[i].WinRate > currentItems[j].WinRate
		}
		if currentItems[i].PnlPct == currentItems[j].PnlPct {
			return currentItems[i].WinRate > currentItems[j].WinRate
		}
		return currentItems[i].PnlPct > currentItems[j].PnlPct
	})

	previousRanks := map[string]int{}
	if duration > 0 {
		now := time.Now().Unix()
		prevStart := now - 2*duration
		prevEnd := now - duration
		previousComputed, _, err := s.computePortfolio(ctx, agents, prevStart, prevEnd)
		if err != nil {
			return nil, err
		}
		previousItems := make([]agentComputed, 0, len(previousComputed))
		for _, item := range previousComputed {
			if item.TotalTrades < minTrades {
				continue
			}
			previousItems = append(previousItems, item)
		}
		sort.SliceStable(previousItems, func(i, j int) bool {
			if metric == "win_rate" {
				if previousItems[i].WinRate == previousItems[j].WinRate {
					return previousItems[i].PnlPct > previousItems[j].PnlPct
				}
				return previousItems[i].WinRate > previousItems[j].WinRate
			}
			if previousItems[i].PnlPct == previousItems[j].PnlPct {
				return previousItems[i].WinRate > previousItems[j].WinRate
			}
			return previousItems[i].PnlPct > previousItems[j].PnlPct
		})
		for i, item := range previousItems {
			previousRanks[item.Agent.ID] = i + 1
		}
	}

	items := make([]LeaderboardItem, 0, len(currentItems))
	for i, item := range currentItems {
		rank := i + 1
		rankChange := 0
		if prevRank, ok := previousRanks[item.Agent.ID]; ok {
			rankChange = prevRank - rank
		}
		sparkline := item.Sparkline
		if len(sparkline) == 0 {
			sparkline = []float64{0}
		}
		for idx := range sparkline {
			sparkline[idx] = round2(sparkline[idx])
		}

		items = append(items, LeaderboardItem{
			Rank:        rank,
			RankChange:  rankChange,
			AgentID:     item.Agent.ID,
			AgentName:   item.Agent.Name,
			WinRate:     round2(item.WinRate),
			PnlPct:      round2(item.PnlPct),
			TotalTrades: item.TotalTrades,
			MaxDrawdown: round2(item.Drawdown),
			Sparkline:   sparkline,
		})
	}
	return items, nil
}

func (s *Store) GetSystemStatus(ctx context.Context) (SystemStatusRecord, error) {
	var lastSlot int64
	var updatedAt int64
	row := s.db.QueryRowContext(ctx, `SELECT last_slot, updated_at FROM sync_state WHERE id = 1`)
	if err := row.Scan(&lastSlot, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SystemStatusRecord{}, nil
		}
		return SystemStatusRecord{}, err
	}

	now := time.Now().Unix()
	lagMS := (now - updatedAt) * 1000
	if lagMS < 0 {
		lagMS = 0
	}

	var latestExchangeTS sql.NullInt64
	latestOrderbookRow := s.db.QueryRowContext(
		ctx,
		`SELECT MAX(exchange_ts) FROM exchange_orderbook_snapshots`,
	)
	if err := latestOrderbookRow.Scan(&latestExchangeTS); err != nil {
		return SystemStatusRecord{}, err
	}

	wsLatency := int64(0)
	if latestExchangeTS.Valid {
		wsLatency = (time.Now().UnixMilli() - latestExchangeTS.Int64)
		if wsLatency < 0 {
			wsLatency = 0
		}
	}

	return SystemStatusRecord{
		DexConnected:    (now - updatedAt) <= 20,
		WSLatencyMS:     wsLatency,
		LastIndexedSlot: uint64(lastSlot),
		IndexerLagMS:    lagMS,
	}, nil
}

func (s *Store) GetCandles(ctx context.Context, market string, limit int) ([]CandleRecord, error) {
	marketSymbol := NormalizeMarketSymbol(market)
	if marketSymbol != "" {
		candles, err := s.GetMarketCandles(ctx, marketSymbol, 60, limit)
		if err != nil {
			return nil, err
		}
		if len(candles) > 0 {
			return candles, nil
		}
	}

	return s.getCandlesFromFills(ctx, market, limit)
}

func (s *Store) getCandlesFromFills(ctx context.Context, market string, limit int) ([]CandleRecord, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	baseQuery := `SELECT price, margin, executed_at FROM fills`
	args := make([]any, 0, 1)
	if market != "" {
		if marketID, err := strconv.ParseUint(market, 10, 64); err == nil {
			baseQuery += ` WHERE market_id = ?`
			args = append(args, int64(marketID))
		}
	}
	baseQuery += ` ORDER BY executed_at ASC, id ASC LIMIT 5000`

	rows, err := s.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type candleBuilder struct {
		TS     int64
		Open   float64
		High   float64
		Low    float64
		Close  float64
		Volume float64
		Has    bool
	}

	builders := map[int64]*candleBuilder{}
	keys := make([]int64, 0, 256)

	for rows.Next() {
		var priceRaw string
		var qtyRaw string
		var executedAt int64
		if err := rows.Scan(&priceRaw, &qtyRaw, &executedAt); err != nil {
			return nil, err
		}
		price := parseFloat(priceRaw)
		qty := parseFloat(qtyRaw)
		bucket := executedAt - (executedAt % 60)
		builder, ok := builders[bucket]
		if !ok {
			builder = &candleBuilder{TS: bucket}
			builders[bucket] = builder
			keys = append(keys, bucket)
		}
		if !builder.Has {
			builder.Open = price
			builder.High = price
			builder.Low = price
			builder.Close = price
			builder.Volume = qty
			builder.Has = true
			continue
		}
		if price > builder.High {
			builder.High = price
		}
		if price < builder.Low {
			builder.Low = price
		}
		builder.Close = price
		builder.Volume += qty
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	candles := make([]CandleRecord, 0, len(keys))
	for _, key := range keys {
		builder := builders[key]
		if !builder.Has {
			continue
		}
		candles = append(candles, CandleRecord{
			TS:     builder.TS,
			Open:   round2(builder.Open),
			High:   round2(builder.High),
			Low:    round2(builder.Low),
			Close:  round2(builder.Close),
			Volume: round6(builder.Volume),
		})
	}
	if len(candles) > limit {
		candles = candles[len(candles)-limit:]
	}
	return candles, nil
}

func (s *Store) computePortfolio(ctx context.Context, agents []AgentRecord, startUnix int64, endUnix int64) ([]agentComputed, []EquityPoint, error) {
	if len(agents) == 0 {
		return []agentComputed{}, []EquityPoint{}, nil
	}

	events, err := s.loadTradeEvents(ctx, "", startUnix, endUnix)
	if err != nil {
		return nil, nil, err
	}
	trades := computeTrades(events)

	tradesByAgent := map[string][]TradeRecord{}
	portfolioEvents := make([]TradeRecord, 0, len(trades))
	for _, trade := range trades {
		tradesByAgent[trade.AgentID] = append(tradesByAgent[trade.AgentID], trade)
		portfolioEvents = append(portfolioEvents, trade)
	}

	out := make([]agentComputed, 0, len(agents))
	for _, agent := range agents {
		agentTrades := tradesByAgent[agent.ID]
		metrics := buildAgentMetrics(agent, agentTrades)
		out = append(out, metrics)
	}

	portfolioHistory := buildPortfolioHistory(portfolioEvents, len(agents))
	return out, portfolioHistory, nil
}

func buildPortfolioHistory(trades []TradeRecord, agentCount int) []EquityPoint {
	if agentCount <= 0 {
		return []EquityPoint{}
	}
	if len(trades) == 0 {
		now := time.Now().Unix()
		return []EquityPoint{{TS: now, Value: defaultAgentEquity * float64(agentCount)}}
	}

	sort.SliceStable(trades, func(i, j int) bool {
		if trades[i].ExitTime == trades[j].ExitTime {
			return trades[i].ID < trades[j].ID
		}
		return trades[i].ExitTime < trades[j].ExitTime
	})

	equity := defaultAgentEquity * float64(agentCount)
	history := make([]EquityPoint, 0, len(trades)+1)
	history = append(history, EquityPoint{TS: trades[0].ExitTime, Value: equity})
	for _, trade := range trades {
		equity += trade.Pnl
		history = append(history, EquityPoint{TS: trade.ExitTime, Value: equity})
	}
	if len(history) > 120 {
		history = history[len(history)-120:]
	}
	return history
}

func buildAgentMetrics(agent AgentRecord, trades []TradeRecord) agentComputed {
	sort.SliceStable(trades, func(i, j int) bool {
		if trades[i].ExitTime == trades[j].ExitTime {
			return trades[i].ID < trades[j].ID
		}
		return trades[i].ExitTime < trades[j].ExitTime
	})

	equity := defaultAgentEquity
	peak := equity
	maxDrawdown := 0.0
	wins := 0
	totalPNL := 0.0
	equityHistory := make([]EquityPoint, 0, len(trades)+1)
	dailyPNLByDate := map[string]float64{}

	if len(trades) == 0 {
		return agentComputed{
			Agent:         agent,
			Trades:        []TradeRecord{},
			PnlAbs:        0,
			PnlPct:        0,
			WinRate:       0,
			TotalTrades:   0,
			Drawdown:      0,
			Equity:        defaultAgentEquity,
			DailyPNL:      []DailyPNLPoint{},
			EquityHistory: []EquityPoint{{TS: time.Now().Unix(), Value: defaultAgentEquity}},
			Sparkline:     []float64{0},
		}
	}

	equityHistory = append(equityHistory, EquityPoint{TS: trades[0].ExitTime, Value: equity})
	for _, trade := range trades {
		totalPNL += trade.Pnl
		equity += trade.Pnl
		equityHistory = append(equityHistory, EquityPoint{TS: trade.ExitTime, Value: equity})
		if trade.Pnl > 0 {
			wins++
		}
		if equity > peak {
			peak = equity
		}
		if peak > 0 {
			dd := ((equity - peak) / peak) * 100
			if dd < maxDrawdown {
				maxDrawdown = dd
			}
		}
		dateKey := time.Unix(trade.ExitTime, 0).UTC().Format("2006-01-02")
		dailyPNLByDate[dateKey] += trade.Pnl
	}

	dates := make([]string, 0, len(dailyPNLByDate))
	for date := range dailyPNLByDate {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	dailyPNL := make([]DailyPNLPoint, 0, len(dates))
	for _, date := range dates {
		dailyPNL = append(dailyPNL, DailyPNLPoint{Date: date, PNL: dailyPNLByDate[date]})
	}

	if len(equityHistory) > 120 {
		equityHistory = equityHistory[len(equityHistory)-120:]
	}
	sparkline := make([]float64, 0, len(equityHistory))
	for _, point := range equityHistory {
		sparkline = append(sparkline, ((point.Value-defaultAgentEquity)/defaultAgentEquity)*100)
	}
	if len(sparkline) > 20 {
		sparkline = sparkline[len(sparkline)-20:]
	}

	winRate := 0.0
	if len(trades) > 0 {
		winRate = (float64(wins) / float64(len(trades))) * 100
	}

	pnlPct := 0.0
	if defaultAgentEquity > 0 {
		pnlPct = (totalPNL / defaultAgentEquity) * 100
	}

	return agentComputed{
		Agent:         agent,
		Trades:        trades,
		PnlAbs:        totalPNL,
		PnlPct:        pnlPct,
		WinRate:       winRate,
		TotalTrades:   len(trades),
		Drawdown:      maxDrawdown,
		Equity:        equity,
		DailyPNL:      dailyPNL,
		EquityHistory: equityHistory,
		Sparkline:     sparkline,
	}
}

func (s *Store) loadTradeEvents(ctx context.Context, agentID string, fromUnix, toUnix int64) ([]tradeEvent, error) {
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 8)

	if agentID != "" {
		clauses = append(clauses, "a.id = ?")
		args = append(args, agentID)
	}
	if fromUnix > 0 {
		clauses = append(clauses, "f.executed_at >= ?")
		args = append(args, fromUnix)
	}
	if toUnix > 0 {
		clauses = append(clauses, "f.executed_at <= ?")
		args = append(args, toUnix)
	}

	query := fmt.Sprintf(
		`SELECT f.id, a.id, a.name, f.side, f.order_type, f.price, f.margin, f.created_at, f.executed_at, f.market_id, f.order_pubkey
		 FROM fills f
		 JOIN agents a ON a.owner_pubkey = f.user_pubkey
		 WHERE %s
		 ORDER BY f.executed_at ASC, f.id ASC`,
		strings.Join(clauses, " AND "),
	)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]tradeEvent, 0, 256)
	priceMarketsByID, err := s.loadExecutionPriceMarketSymbols(ctx)
	if err != nil {
		return nil, err
	}
	executionPriceCache := make(map[string]float64, 256)
	for rows.Next() {
		var item tradeEvent
		var orderType string
		var priceRaw string
		var qtyRaw string
		var marketID int64
		if err := rows.Scan(
			&item.FillID,
			&item.AgentID,
			&item.AgentName,
			&item.Side,
			&orderType,
			&priceRaw,
			&qtyRaw,
			&item.CreatedAt,
			&item.ExecutedAt,
			&marketID,
			&item.TxSig,
		); err != nil {
			return nil, err
		}
		item.OrderType = orderType
		item.MarketID = uint64(maxInt64(marketID, 0))
		item.Notional = math.Abs(parseScaledFloat(qtyRaw, rawNotionalScale))
		resolvedPrice := parseScaledFloat(priceRaw, rawPriceScale)
		if strings.EqualFold(orderType, "market") {
			cacheKey := fmt.Sprintf("%d:%d", item.MarketID, item.ExecutedAt)
			if cachedPrice, ok := executionPriceCache[cacheKey]; ok {
				resolvedPrice = cachedPrice
			} else {
				execPrice, lookupErr := s.lookupExecutionPrice(ctx, priceMarketsByID[item.MarketID], item.ExecutedAt)
				if lookupErr == nil && execPrice > 0 {
					resolvedPrice = execPrice
				}
				executionPriceCache[cacheKey] = resolvedPrice
			}
		}
		item.Price = resolvedPrice
		if item.Notional > 0 && item.Price > 0 {
			item.Qty = item.Notional / item.Price
		}
		events = append(events, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Store) lookupExecutionPrice(ctx context.Context, marketSymbol string, executedAt int64) (float64, error) {
	if strings.TrimSpace(marketSymbol) == "" {
		return 0, nil
	}

	var price float64
	err := s.db.QueryRowContext(
		ctx,
		`SELECT price
		 FROM market_price_ticks
		 WHERE market = ? AND publish_time <= ?
		 ORDER BY publish_time DESC, id DESC
		 LIMIT 1`,
		marketSymbol,
		executedAt,
	).Scan(&price)
	if err == nil {
		return price, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	err = s.db.QueryRowContext(
		ctx,
		`SELECT price
		 FROM market_price_ticks
		 WHERE market = ? AND publish_time >= ?
		 ORDER BY publish_time ASC, id ASC
		 LIMIT 1`,
		marketSymbol,
		executedAt,
	).Scan(&price)
	if err == nil {
		return price, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return 0, err
}

func (s *Store) loadExecutionPriceMarketSymbols(ctx context.Context) (map[uint64]string, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT raw_json
		 FROM resources
		 WHERE account_type = ?`,
		"Market",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	marketSymbolsByID := make(map[uint64]string, 16)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}

		var market orderengine.Market
		if err := json.Unmarshal([]byte(raw), &market); err != nil {
			continue
		}
		if market.MarketId == 0 {
			continue
		}
		marketSymbolsByID[market.MarketId] = decodeFixedSymbol(market.Symbol[:])
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	priceMarkets, err := s.loadAvailablePriceMarkets(ctx)
	if err != nil {
		return nil, err
	}

	resolved := make(map[uint64]string, len(marketSymbolsByID))
	for marketID, marketSymbol := range marketSymbolsByID {
		resolved[marketID] = resolvePriceMarketSymbol(marketSymbol, priceMarkets)
	}
	return resolved, nil
}

func (s *Store) loadAvailablePriceMarkets(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT DISTINCT market
		 FROM market_price_ticks
		 WHERE market <> ''
		 ORDER BY market ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	markets := make([]string, 0, 16)
	for rows.Next() {
		var market string
		if err := rows.Scan(&market); err != nil {
			return nil, err
		}
		trimmed := strings.TrimSpace(market)
		if trimmed == "" {
			continue
		}
		markets = append(markets, strings.ToUpper(trimmed))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return markets, nil
}

func decodeFixedSymbol(raw []uint8) string {
	if len(raw) == 0 {
		return ""
	}
	symbolBytes := make([]byte, 0, len(raw))
	for _, b := range raw {
		if b == 0 {
			break
		}
		symbolBytes = append(symbolBytes, b)
	}
	return strings.ToUpper(strings.TrimSpace(string(symbolBytes)))
}

func resolvePriceMarketSymbol(marketSymbol string, availableMarkets []string) string {
	target := strings.ToUpper(strings.TrimSpace(marketSymbol))
	if target == "" {
		if len(availableMarkets) == 1 {
			return availableMarkets[0]
		}
		return ""
	}

	availableSet := make(map[string]struct{}, len(availableMarkets))
	for _, market := range availableMarkets {
		availableSet[market] = struct{}{}
	}
	if _, ok := availableSet[target]; ok {
		return target
	}

	canonicalTarget := canonicalSymbol(target)
	for _, market := range availableMarkets {
		if canonicalSymbol(market) == canonicalTarget {
			return market
		}
	}

	base := baseAssetSymbol(target)
	if base != "" {
		candidates := make([]string, 0, len(availableMarkets))
		for _, market := range availableMarkets {
			if baseAssetSymbol(market) == base {
				candidates = append(candidates, market)
			}
		}
		if len(candidates) == 1 {
			return candidates[0]
		}
		if len(candidates) > 1 {
			for _, quoteSuffix := range []string{"USDT", "USD", "USDC"} {
				for _, candidate := range candidates {
					if strings.HasSuffix(candidate, quoteSuffix) {
						return candidate
					}
				}
			}
			return candidates[0]
		}
	}

	perpNormalized := canonicalSymbol(strings.TrimSuffix(target, "-PERP") + "USDT")
	for _, market := range availableMarkets {
		if canonicalSymbol(market) == perpNormalized {
			return market
		}
	}

	if len(availableMarkets) == 1 {
		return availableMarkets[0]
	}
	return ""
}

func canonicalSymbol(symbol string) string {
	upper := strings.ToUpper(strings.TrimSpace(symbol))
	var builder strings.Builder
	builder.Grow(len(upper))
	for _, char := range upper {
		if (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func baseAssetSymbol(symbol string) string {
	upper := strings.ToUpper(strings.TrimSpace(symbol))
	if upper == "" {
		return ""
	}
	for _, delimiter := range []string{"-", "_", "/"} {
		if index := strings.Index(upper, delimiter); index > 0 {
			upper = upper[:index]
			break
		}
	}
	for _, suffix := range []string{"PERP", "USDT", "USDC", "USD"} {
		upper = strings.TrimSuffix(upper, suffix)
	}
	var builder strings.Builder
	for _, char := range upper {
		if char >= 'A' && char <= 'Z' {
			builder.WriteRune(char)
			continue
		}
		break
	}
	return builder.String()
}

func computeTrades(events []tradeEvent) []TradeRecord {
	if len(events) == 0 {
		return []TradeRecord{}
	}

	stateByKey := make(map[string]positionState, 64)
	trades := make([]TradeRecord, 0, len(events))
	for _, event := range events {
		qty := math.Abs(event.Qty)
		if qty == 0 && event.Price > 0 && event.Notional > 0 {
			qty = event.Notional / event.Price
		}
		if qty == 0 {
			continue
		}
		if event.Price <= 0 {
			continue
		}

		key := event.AgentID + ":" + strconv.FormatUint(event.MarketID, 10)
		state := stateByKey[key]
		entryPrice := event.Price
		realizedPNL := 0.0
		buy := isBuySide(event.Side)
		direction := -1.0
		side := "short"
		if buy {
			direction = 1.0
			side = "long"
		}

		if state.Qty == 0 || sameSign(state.Qty, direction) {
			if state.Qty != 0 {
				entryPrice = state.AvgPrice
			}
			newAbs := math.Abs(state.Qty) + qty
			if newAbs > 0 {
				if state.Qty == 0 {
					state.AvgPrice = event.Price
				} else {
					state.AvgPrice = ((math.Abs(state.Qty) * state.AvgPrice) + (qty * event.Price)) / newAbs
				}
			}
			state.Qty += direction * qty
		} else {
			entryPrice = state.AvgPrice
			closeQty := math.Min(math.Abs(state.Qty), qty)
			if state.Qty > 0 {
				realizedPNL = closeQty * (event.Price - state.AvgPrice)
			} else {
				realizedPNL = closeQty * (state.AvgPrice - event.Price)
			}

			remaining := qty - closeQty
			state.Qty += direction * qty
			if math.Abs(state.Qty) < 1e-9 {
				state.Qty = 0
				state.AvgPrice = 0
			} else if remaining > 0 {
				state.AvgPrice = event.Price
			}
		}

		stateByKey[key] = state
		feeNotional := math.Abs(event.Notional)
		if feeNotional == 0 {
			feeNotional = qty * event.Price
		}
		fee := feeNotional * defaultTradeFeeRate
		trades = append(trades, TradeRecord{
			ID:         strconv.FormatInt(event.FillID, 10),
			AgentID:    event.AgentID,
			AgentName:  event.AgentName,
			Side:       side,
			EntryPrice: entryPrice,
			ExitPrice:  event.Price,
			Qty:        qty,
			Fee:        fee,
			Pnl:        realizedPNL - fee,
			TxSig:      event.TxSig,
			EntryTime:  event.CreatedAt,
			ExitTime:   event.ExecutedAt,
			MarketID:   event.MarketID,
		})
	}
	return trades
}

func sameSign(a, b float64) bool {
	return (a > 0 && b > 0) || (a < 0 && b < 0)
}

func isBuySide(side string) bool {
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "bid", "buy", "long", "b":
		return true
	default:
		return false
	}
}

func parseRiskProfile(raw string) AgentRiskProfile {
	if strings.TrimSpace(raw) == "" {
		return defaultRiskProfile()
	}
	var profile AgentRiskProfile
	if err := json.Unmarshal([]byte(raw), &profile); err != nil {
		return defaultRiskProfile()
	}
	return profile
}

func parseJSONMap(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	if out == nil {
		return map[string]any{}
	}
	return out
}

func parseFloat(raw string) float64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0
	}
	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0
	}
	return value
}

func parseScaledFloat(raw string, scale float64) float64 {
	value := parseFloat(raw)
	if scale <= 0 {
		return value
	}
	return value / scale
}

func portfolioPeriodStart(period string) (int64, error) {
	now := time.Now().Unix()
	switch strings.ToLower(strings.TrimSpace(period)) {
	case "", "7d":
		return now - int64(7*24*time.Hour/time.Second), nil
	case "30d":
		return now - int64(30*24*time.Hour/time.Second), nil
	case "all":
		return 0, nil
	default:
		return 0, fmt.Errorf("invalid period")
	}
}

func leaderboardPeriodStart(period string) (int64, int64, error) {
	now := time.Now().Unix()
	switch strings.ToLower(strings.TrimSpace(period)) {
	case "", "7d":
		duration := int64(7 * 24 * time.Hour / time.Second)
		return now - duration, duration, nil
	case "30d":
		duration := int64(30 * 24 * time.Hour / time.Second)
		return now - duration, duration, nil
	case "all_time":
		return 0, 0, nil
	default:
		return 0, 0, fmt.Errorf("invalid period")
	}
}

func round2(v float64) float64 {
	rounded := math.Round(v*100) / 100
	if math.Abs(rounded) < 0.005 {
		return 0
	}
	return rounded
}

func round6(v float64) float64 {
	rounded := math.Round(v*1_000_000) / 1_000_000
	if math.Abs(rounded) < 0.0000005 {
		return 0
	}
	return rounded
}

func roundTrade(trade TradeRecord) TradeRecord {
	trade.EntryPrice = round2(trade.EntryPrice)
	trade.ExitPrice = round2(trade.ExitPrice)
	trade.Qty = round6(trade.Qty)
	trade.Fee = round6(trade.Fee)
	trade.Pnl = round2(trade.Pnl)
	return trade
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
