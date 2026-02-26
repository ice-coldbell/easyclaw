package apiserver

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coldbell/dex/backend/internal/indexer"
	"github.com/gagliardetto/solana-go"
	"github.com/gorilla/websocket"
)

const (
	authChallengeTTL = 5 * time.Minute
	authSessionTTL   = 24 * time.Hour
)

type authChallengeRequest struct {
	WalletPubkey string `json:"wallet_pubkey"`
	Intent       string `json:"intent"`
}

type authChallengeResponse struct {
	ChallengeID string `json:"challenge_id"`
	Message     string `json:"message"`
}

type authVerifySignatureRequest struct {
	ChallengeID  string `json:"challenge_id"`
	Signature    string `json:"signature"`
	WalletPubkey string `json:"wallet_pubkey"`
}

type authTokenResponse struct {
	SessionToken string `json:"session_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

type createAgentRequest struct {
	Name        string                    `json:"name"`
	StrategyID  string                    `json:"strategy_id"`
	RiskProfile *indexer.AgentRiskProfile `json:"risk_profile"`
}

type createAgentResponse struct {
	AgentID string `json:"agent_id"`
}

type rebindAgentOwnerRequest struct {
	ChallengeID string `json:"challenge_id"`
	Signature   string `json:"signature"`
}

type rebindAgentOwnerResponse struct {
	BoundAt int64 `json:"bound_at"`
}

type startAgentSessionRequest struct {
	Mode string `json:"mode"`
}

type startAgentSessionResponse struct {
	SessionID string `json:"session_id"`
	StartedAt int64  `json:"started_at"`
}

type killSwitchRequest struct {
	AgentIDs []string `json:"agent_ids"`
}

type killSwitchResponse struct {
	StoppedCount int `json:"stopped_count"`
}

type patchRiskRequest struct {
	MaxPositionUSDC    *float64 `json:"max_position_usdc"`
	DailyLossLimitUSDC *float64 `json:"daily_loss_limit_usdc"`
	KillSwitchEnabled  *bool    `json:"kill_switch_enabled"`
}

type patchRiskResponse struct {
	UpdatedRiskProfile indexer.AgentRiskProfile `json:"updated_risk_profile"`
}

type createStrategyRequest struct {
	Name         string         `json:"name"`
	EntryRules   map[string]any `json:"entry_rules"`
	ExitRules    map[string]any `json:"exit_rules"`
	RiskDefaults map[string]any `json:"risk_defaults"`
}

type createStrategyResponse struct {
	StrategyID string `json:"strategy_id"`
}

type patchStrategyRequest struct {
	Name       *string         `json:"name"`
	EntryRules *map[string]any `json:"entry_rules"`
	ExitRules  *map[string]any `json:"exit_rules"`
}

type publishStrategyResponse struct {
	PublishedAt int64 `json:"published_at"`
}

type leaderboardResponse struct {
	Items []indexer.LeaderboardItem `json:"items"`
}

type tradesResponse struct {
	Items  []indexer.TradeRecord `json:"items"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}

type chartCandlesResponse struct {
	Market      string                 `json:"market"`
	Timeframe   string                 `json:"timeframe"`
	IntervalSec int64                  `json:"interval_sec"`
	Candles     []indexer.CandleRecord `json:"candles"`
}

type websocketSubscribeRequest struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
}

type websocketEnvelope struct {
	Type    string `json:"type"`
	Channel string `json:"channel,omitempty"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
	TS      int64  `json:"ts"`
}

var websocketUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

func (s *Service) handleAuthChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondMethodNotAllowed(w)
		return
	}

	var request authChallengeRequest
	if err := decodeJSONBody(r, &request); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	request.WalletPubkey = strings.TrimSpace(request.WalletPubkey)
	request.Intent = strings.TrimSpace(request.Intent)
	if request.WalletPubkey == "" {
		s.respondError(w, http.StatusBadRequest, "wallet_pubkey is required")
		return
	}
	if !isAllowedIntent(request.Intent) {
		s.respondError(w, http.StatusBadRequest, "intent must be owner_bind, session, or live_stepup")
		return
	}
	if _, err := solana.PublicKeyFromBase58(request.WalletPubkey); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid wallet_pubkey")
		return
	}

	now := time.Now().Unix()
	expiresAt := now + int64(authChallengeTTL/time.Second)
	challengeID, err := newID("chl")
	if err != nil {
		s.logger.Error("create challenge id failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to create challenge")
		return
	}
	message := fmt.Sprintf("Easyclaw authentication\nintent: %s\nchallenge_id: %s\nwallet: %s\nexpires_at: %d", request.Intent, challengeID, request.WalletPubkey, expiresAt)

	err = s.store.CreateAuthChallenge(r.Context(), indexer.AuthChallengeRecord{
		ID:           challengeID,
		WalletPubkey: request.WalletPubkey,
		Intent:       request.Intent,
		Message:      message,
		CreatedAt:    now,
		ExpiresAt:    expiresAt,
	})
	if err != nil {
		s.logger.Error("store auth challenge failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to create challenge")
		return
	}

	s.respondJSON(w, http.StatusOK, authChallengeResponse{
		ChallengeID: challengeID,
		Message:     message,
	})
}

func (s *Service) handleAuthVerifySignature(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondMethodNotAllowed(w)
		return
	}

	var request authVerifySignatureRequest
	if err := decodeJSONBody(r, &request); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	request.ChallengeID = strings.TrimSpace(request.ChallengeID)
	request.Signature = strings.TrimSpace(request.Signature)
	request.WalletPubkey = strings.TrimSpace(request.WalletPubkey)
	if request.ChallengeID == "" || request.Signature == "" || request.WalletPubkey == "" {
		s.respondError(w, http.StatusBadRequest, "challenge_id, signature, wallet_pubkey are required")
		return
	}

	challenge, err := s.store.GetAuthChallenge(r.Context(), request.ChallengeID)
	if err != nil {
		if errors.Is(err, indexer.ErrNotFound) {
			s.respondError(w, http.StatusNotFound, "challenge not found")
			return
		}
		s.logger.Error("get challenge failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to verify challenge")
		return
	}

	now := time.Now().Unix()
	if challenge.ExpiresAt <= now {
		s.respondError(w, http.StatusUnauthorized, "challenge expired")
		return
	}
	if challenge.UsedAt != nil {
		s.respondError(w, http.StatusUnauthorized, "challenge already used")
		return
	}
	if challenge.WalletPubkey != request.WalletPubkey {
		s.respondError(w, http.StatusUnauthorized, "wallet mismatch")
		return
	}
	if err := verifyWalletSignature(request.WalletPubkey, request.Signature, challenge.Message); err != nil {
		s.respondError(w, http.StatusUnauthorized, "invalid signature")
		return
	}

	if err := s.store.MarkAuthChallengeUsed(r.Context(), request.ChallengeID, now); err != nil {
		s.logger.Error("mark challenge used failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to finalize challenge")
		return
	}

	token, tokenHash, err := newSessionToken()
	if err != nil {
		s.logger.Error("create session token failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	expiresAt := now + int64(authSessionTTL/time.Second)
	if err := s.store.CreateAuthSession(r.Context(), tokenHash, request.WalletPubkey, now, expiresAt); err != nil {
		s.logger.Error("store session failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	s.respondJSON(w, http.StatusOK, authTokenResponse{SessionToken: token, ExpiresAt: expiresAt})
}

func (s *Service) handleAuthSessionRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondMethodNotAllowed(w)
		return
	}

	token, err := bearerTokenFromRequest(r)
	if err != nil {
		s.respondError(w, http.StatusUnauthorized, err.Error())
		return
	}

	now := time.Now().Unix()
	oldHash := hashToken(token)
	newToken, newTokenHash, err := newSessionToken()
	if err != nil {
		s.logger.Error("create refresh token failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to refresh session")
		return
	}
	newExpiresAt := now + int64(authSessionTTL/time.Second)
	_, err = s.store.RotateAuthSession(r.Context(), oldHash, newTokenHash, now, newExpiresAt)
	if err != nil {
		if errors.Is(err, indexer.ErrUnauthorized) {
			s.respondError(w, http.StatusUnauthorized, "invalid or expired session")
			return
		}
		s.logger.Error("rotate session failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to refresh session")
		return
	}

	s.respondJSON(w, http.StatusOK, authTokenResponse{SessionToken: newToken, ExpiresAt: newExpiresAt})
}

func (s *Service) handleAgentsRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		agents, err := s.store.ListAgents(r.Context())
		if err != nil {
			s.logger.Error("list agents failed", "err", err)
			s.respondError(w, http.StatusInternalServerError, "failed to list agents")
			return
		}
		s.respondJSON(w, http.StatusOK, agents)
	case http.MethodPost:
		session, err := s.requireSession(r)
		if err != nil {
			s.respondError(w, http.StatusUnauthorized, err.Error())
			return
		}
		var request createAgentRequest
		if err := decodeJSONBody(r, &request); err != nil {
			s.respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		request.Name = strings.TrimSpace(request.Name)
		request.StrategyID = strings.TrimSpace(request.StrategyID)
		if request.Name == "" || request.StrategyID == "" {
			s.respondError(w, http.StatusBadRequest, "name and strategy_id are required")
			return
		}

		now := time.Now().Unix()
		agentID, err := newID("agent")
		if err != nil {
			s.logger.Error("create agent id failed", "err", err)
			s.respondError(w, http.StatusInternalServerError, "failed to create agent")
			return
		}
		riskProfile := indexer.AgentRiskProfile{}
		if request.RiskProfile != nil {
			riskProfile = *request.RiskProfile
		}

		createdAgentID, err := s.store.CreateAgent(r.Context(), indexer.CreateAgentInput{
			ID:          agentID,
			Name:        request.Name,
			StrategyID:  request.StrategyID,
			OwnerPubkey: session.WalletPubkey,
			RiskProfile: riskProfile,
			CreatedAt:   now,
		})
		if err != nil {
			s.logger.Error("create agent failed", "err", err)
			s.respondError(w, http.StatusInternalServerError, "failed to create agent")
			return
		}

		s.respondJSON(w, http.StatusOK, createAgentResponse{AgentID: createdAgentID})
	default:
		s.respondMethodNotAllowed(w)
	}
}

func (s *Service) handleAgentsSubroutes(w http.ResponseWriter, r *http.Request) {
	agentID, tail := splitAgentSubroute(r.URL.Path)
	if agentID == "" {
		s.respondError(w, http.StatusNotFound, "agent id is required")
		return
	}

	switch {
	case tail == "":
		if r.Method != http.MethodGet {
			s.respondMethodNotAllowed(w)
			return
		}
		agent, err := s.store.GetAgent(r.Context(), agentID)
		if err != nil {
			if errors.Is(err, indexer.ErrNotFound) {
				s.respondError(w, http.StatusNotFound, "agent not found")
				return
			}
			s.logger.Error("get agent failed", "err", err)
			s.respondError(w, http.StatusInternalServerError, "failed to get agent")
			return
		}
		s.respondJSON(w, http.StatusOK, agent)
		return

	case tail == "owner-binding":
		if r.Method != http.MethodGet {
			s.respondMethodNotAllowed(w)
			return
		}
		binding, err := s.store.GetAgentBinding(r.Context(), agentID)
		if err != nil {
			if errors.Is(err, indexer.ErrNotFound) {
				s.respondError(w, http.StatusNotFound, "agent not found")
				return
			}
			s.logger.Error("get owner binding failed", "err", err)
			s.respondError(w, http.StatusInternalServerError, "failed to get owner binding")
			return
		}
		s.respondJSON(w, http.StatusOK, binding)
		return

	case tail == "owner-binding/rebind":
		if r.Method != http.MethodPost {
			s.respondMethodNotAllowed(w)
			return
		}
		var request rebindAgentOwnerRequest
		if err := decodeJSONBody(r, &request); err != nil {
			s.respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		request.ChallengeID = strings.TrimSpace(request.ChallengeID)
		request.Signature = strings.TrimSpace(request.Signature)
		if request.ChallengeID == "" || request.Signature == "" {
			s.respondError(w, http.StatusBadRequest, "challenge_id and signature are required")
			return
		}

		challenge, err := s.store.GetAuthChallenge(r.Context(), request.ChallengeID)
		if err != nil {
			if errors.Is(err, indexer.ErrNotFound) {
				s.respondError(w, http.StatusNotFound, "challenge not found")
				return
			}
			s.logger.Error("get rebind challenge failed", "err", err)
			s.respondError(w, http.StatusInternalServerError, "failed to verify challenge")
			return
		}
		now := time.Now().Unix()
		if challenge.ExpiresAt <= now {
			s.respondError(w, http.StatusUnauthorized, "challenge expired")
			return
		}
		if challenge.UsedAt != nil {
			s.respondError(w, http.StatusUnauthorized, "challenge already used")
			return
		}
		if err := verifyWalletSignature(challenge.WalletPubkey, request.Signature, challenge.Message); err != nil {
			s.respondError(w, http.StatusUnauthorized, "invalid signature")
			return
		}
		if err := s.store.MarkAuthChallengeUsed(r.Context(), request.ChallengeID, now); err != nil {
			s.logger.Error("mark rebind challenge used failed", "err", err)
			s.respondError(w, http.StatusInternalServerError, "failed to finalize rebind")
			return
		}

		sessionExpiresAt := now + int64(authSessionTTL/time.Second)
		if session, err := s.optionalSession(r); err == nil {
			sessionExpiresAt = session.ExpiresAt
		}
		if err := s.store.RebindAgentOwner(r.Context(), agentID, challenge.WalletPubkey, now, sessionExpiresAt); err != nil {
			if errors.Is(err, indexer.ErrNotFound) {
				s.respondError(w, http.StatusNotFound, "agent not found")
				return
			}
			s.logger.Error("rebind agent owner failed", "err", err)
			s.respondError(w, http.StatusInternalServerError, "failed to rebind owner")
			return
		}

		s.respondJSON(w, http.StatusOK, rebindAgentOwnerResponse{BoundAt: now})
		return

	case tail == "sessions":
		if r.Method != http.MethodPost {
			s.respondMethodNotAllowed(w)
			return
		}
		session, err := s.requireSession(r)
		if err != nil {
			s.respondError(w, http.StatusUnauthorized, err.Error())
			return
		}
		var request startAgentSessionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			s.respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		now := time.Now().Unix()
		sessionID, err := newID("sess")
		if err != nil {
			s.logger.Error("create session id failed", "err", err)
			s.respondError(w, http.StatusInternalServerError, "failed to start session")
			return
		}
		createdSessionID, startedAt, err := s.store.StartAgentSession(r.Context(), indexer.StartAgentSessionInput{
			ID:        sessionID,
			AgentID:   agentID,
			Mode:      request.Mode,
			CreatedBy: session.WalletPubkey,
			StartedAt: now,
		})
		if err != nil {
			if strings.Contains(err.Error(), "invalid mode") {
				s.respondError(w, http.StatusBadRequest, "mode must be paper or live")
				return
			}
			if errors.Is(err, indexer.ErrNotFound) {
				s.respondError(w, http.StatusNotFound, "agent not found")
				return
			}
			s.logger.Error("start agent session failed", "err", err)
			s.respondError(w, http.StatusInternalServerError, "failed to start agent session")
			return
		}
		s.respondJSON(w, http.StatusOK, startAgentSessionResponse{SessionID: createdSessionID, StartedAt: startedAt})
		return

	case strings.HasPrefix(tail, "sessions/"):
		if r.Method != http.MethodDelete {
			s.respondMethodNotAllowed(w)
			return
		}
		if _, err := s.requireSession(r); err != nil {
			s.respondError(w, http.StatusUnauthorized, err.Error())
			return
		}
		sessionID := strings.TrimSpace(strings.TrimPrefix(tail, "sessions/"))
		if sessionID == "" {
			s.respondError(w, http.StatusBadRequest, "session id is required")
			return
		}
		if err := s.store.StopAgentSession(r.Context(), agentID, sessionID, time.Now().Unix()); err != nil {
			if errors.Is(err, indexer.ErrNotFound) {
				s.respondError(w, http.StatusNotFound, "session not found")
				return
			}
			s.logger.Error("stop agent session failed", "err", err)
			s.respondError(w, http.StatusInternalServerError, "failed to stop session")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return

	case tail == "risk":
		switch r.Method {
		case http.MethodGet:
			risk, err := s.store.GetAgentRisk(r.Context(), agentID)
			if err != nil {
				if errors.Is(err, indexer.ErrNotFound) {
					s.respondError(w, http.StatusNotFound, "agent not found")
					return
				}
				s.logger.Error("get agent risk failed", "err", err)
				s.respondError(w, http.StatusInternalServerError, "failed to get risk")
				return
			}
			s.respondJSON(w, http.StatusOK, risk)
		case http.MethodPatch:
			if _, err := s.requireSession(r); err != nil {
				s.respondError(w, http.StatusUnauthorized, err.Error())
				return
			}
			var request patchRiskRequest
			if err := decodeJSONBody(r, &request); err != nil {
				s.respondError(w, http.StatusBadRequest, err.Error())
				return
			}
			updated, err := s.store.PatchAgentRisk(r.Context(), agentID, indexer.RiskPatch{
				MaxPositionUSDC:    request.MaxPositionUSDC,
				DailyLossLimitUSDC: request.DailyLossLimitUSDC,
				KillSwitchEnabled:  request.KillSwitchEnabled,
				UpdatedAt:          time.Now().Unix(),
			})
			if err != nil {
				if errors.Is(err, indexer.ErrNotFound) {
					s.respondError(w, http.StatusNotFound, "agent not found")
					return
				}
				s.logger.Error("patch agent risk failed", "err", err)
				s.respondError(w, http.StatusInternalServerError, "failed to patch risk")
				return
			}
			s.respondJSON(w, http.StatusOK, patchRiskResponse{UpdatedRiskProfile: updated})
		default:
			s.respondMethodNotAllowed(w)
		}
		return
	default:
		s.respondError(w, http.StatusNotFound, "not found")
	}
}

func (s *Service) handleKillSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondMethodNotAllowed(w)
		return
	}
	if _, err := s.requireSession(r); err != nil {
		s.respondError(w, http.StatusUnauthorized, err.Error())
		return
	}

	var request killSwitchRequest
	if err := decodeJSONBody(r, &request); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	allAgents := false
	agentIDs := make([]string, 0, len(request.AgentIDs))
	for _, rawID := range request.AgentIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if strings.EqualFold(id, "all") {
			allAgents = true
			break
		}
		agentIDs = append(agentIDs, id)
	}

	count, err := s.store.KillSwitch(r.Context(), allAgents, agentIDs, time.Now().Unix())
	if err != nil {
		s.logger.Error("kill switch failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to stop sessions")
		return
	}
	s.respondJSON(w, http.StatusOK, killSwitchResponse{StoppedCount: count})
}

func (s *Service) handleStrategyTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondMethodNotAllowed(w)
		return
	}
	items, err := s.store.ListStrategyTemplates(r.Context())
	if err != nil {
		s.logger.Error("list strategy templates failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to list strategy templates")
		return
	}
	s.respondJSON(w, http.StatusOK, items)
}

func (s *Service) handleStrategiesRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondMethodNotAllowed(w)
		return
	}
	session, err := s.requireSession(r)
	if err != nil {
		s.respondError(w, http.StatusUnauthorized, err.Error())
		return
	}

	var request createStrategyRequest
	if err := decodeJSONBody(r, &request); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" {
		s.respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if request.EntryRules == nil {
		request.EntryRules = map[string]any{}
	}
	if request.ExitRules == nil {
		request.ExitRules = map[string]any{}
	}
	if request.RiskDefaults == nil {
		request.RiskDefaults = map[string]any{}
	}

	strategyID, err := newID("strategy")
	if err != nil {
		s.logger.Error("create strategy id failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to create strategy")
		return
	}
	now := time.Now().Unix()
	createdID, err := s.store.CreateStrategy(r.Context(), indexer.CreateStrategyInput{
		ID:           strategyID,
		Name:         request.Name,
		EntryRules:   request.EntryRules,
		ExitRules:    request.ExitRules,
		RiskDefaults: request.RiskDefaults,
		OwnerPubkey:  session.WalletPubkey,
		CreatedAt:    now,
	})
	if err != nil {
		s.logger.Error("create strategy failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to create strategy")
		return
	}
	s.respondJSON(w, http.StatusOK, createStrategyResponse{StrategyID: createdID})
}

func (s *Service) handleStrategiesSubroutes(w http.ResponseWriter, r *http.Request) {
	strategyID, tail := splitStrategiesSubroute(r.URL.Path)
	if strategyID == "" {
		s.respondError(w, http.StatusNotFound, "strategy id is required")
		return
	}

	switch {
	case tail == "":
		switch r.Method {
		case http.MethodGet:
			strategy, err := s.store.GetStrategy(r.Context(), strategyID)
			if err != nil {
				if errors.Is(err, indexer.ErrNotFound) {
					s.respondError(w, http.StatusNotFound, "strategy not found")
					return
				}
				s.logger.Error("get strategy failed", "err", err)
				s.respondError(w, http.StatusInternalServerError, "failed to get strategy")
				return
			}
			s.respondJSON(w, http.StatusOK, strategy)
		case http.MethodPatch:
			if _, err := s.requireSession(r); err != nil {
				s.respondError(w, http.StatusUnauthorized, err.Error())
				return
			}
			var request patchStrategyRequest
			if err := decodeJSONBody(r, &request); err != nil {
				s.respondError(w, http.StatusBadRequest, err.Error())
				return
			}
			updated, err := s.store.PatchStrategy(r.Context(), strategyID, indexer.StrategyPatch{
				Name:       request.Name,
				EntryRules: request.EntryRules,
				ExitRules:  request.ExitRules,
				UpdatedAt:  time.Now().Unix(),
			})
			if err != nil {
				if errors.Is(err, indexer.ErrNotFound) {
					s.respondError(w, http.StatusNotFound, "strategy not found")
					return
				}
				s.logger.Error("patch strategy failed", "err", err)
				s.respondError(w, http.StatusInternalServerError, "failed to patch strategy")
				return
			}
			s.respondJSON(w, http.StatusOK, updated)
		default:
			s.respondMethodNotAllowed(w)
		}
		return
	case tail == "publish":
		if r.Method != http.MethodPost {
			s.respondMethodNotAllowed(w)
			return
		}
		if _, err := s.requireSession(r); err != nil {
			s.respondError(w, http.StatusUnauthorized, err.Error())
			return
		}
		publishedAt, err := s.store.PublishStrategy(r.Context(), strategyID, time.Now().Unix())
		if err != nil {
			if errors.Is(err, indexer.ErrNotFound) {
				s.respondError(w, http.StatusNotFound, "strategy not found")
				return
			}
			s.logger.Error("publish strategy failed", "err", err)
			s.respondError(w, http.StatusInternalServerError, "failed to publish strategy")
			return
		}
		s.respondJSON(w, http.StatusOK, publishStrategyResponse{PublishedAt: publishedAt})
		return
	default:
		s.respondError(w, http.StatusNotFound, "not found")
	}
}

func (s *Service) handlePortfolio(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondMethodNotAllowed(w)
		return
	}
	period := strings.TrimSpace(r.URL.Query().Get("period"))
	if period == "" {
		period = "7d"
	}
	summary, err := s.store.GetPortfolioSummary(r.Context(), period)
	if err != nil {
		if strings.Contains(err.Error(), "invalid period") {
			s.respondError(w, http.StatusBadRequest, "period must be 7d, 30d, or all")
			return
		}
		s.logger.Error("get portfolio failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to load portfolio")
		return
	}
	s.respondJSON(w, http.StatusOK, summary)
}

func (s *Service) handlePortfolioAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondMethodNotAllowed(w)
		return
	}
	agentID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/portfolio/agents/"))
	agentID = strings.Trim(agentID, "/")
	if agentID == "" {
		s.respondError(w, http.StatusBadRequest, "agent id is required")
		return
	}
	period := strings.TrimSpace(r.URL.Query().Get("period"))
	if period == "" {
		period = "7d"
	}
	result, err := s.store.GetAgentPortfolioSummary(r.Context(), agentID, period)
	if err != nil {
		if strings.Contains(err.Error(), "invalid period") {
			s.respondError(w, http.StatusBadRequest, "period must be 7d, 30d, or all")
			return
		}
		if errors.Is(err, indexer.ErrNotFound) {
			s.respondError(w, http.StatusNotFound, "agent not found")
			return
		}
		s.logger.Error("get agent portfolio failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to load agent portfolio")
		return
	}
	s.respondJSON(w, http.StatusOK, result)
}

func (s *Service) handleChartCandles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondMethodNotAllowed(w)
		return
	}

	market := indexer.NormalizeMarketSymbol(r.URL.Query().Get("market"))
	if market == "" {
		market = "BTCUSDT"
	}

	timeframe, intervalSec, err := parseChartTimeframe(r.URL.Query().Get("timeframe"))
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	limit, err := parseOptionalInt(r, "limit", 120)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	candles, err := s.store.GetMarketCandles(r.Context(), market, intervalSec, limit)
	if err != nil {
		s.logger.Error("get market candles failed", "market", market, "timeframe", timeframe, "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to load candles")
		return
	}

	s.respondJSON(w, http.StatusOK, chartCandlesResponse{
		Market:      market,
		Timeframe:   timeframe,
		IntervalSec: intervalSec,
		Candles:     candles,
	})
}

func (s *Service) handleTrades(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondMethodNotAllowed(w)
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

	items, normalizedLimit, normalizedOffset, err := s.store.ListTrades(r.Context(), indexer.TradeFilter{
		AgentID:  strings.TrimSpace(r.URL.Query().Get("agent_id")),
		FromUnix: from,
		ToUnix:   to,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		s.logger.Error("list trades failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to list trades")
		return
	}

	s.respondJSON(w, http.StatusOK, tradesResponse{Items: items, Limit: normalizedLimit, Offset: normalizedOffset})
}

func (s *Service) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondMethodNotAllowed(w)
		return
	}
	metric := strings.TrimSpace(r.URL.Query().Get("metric"))
	period := strings.TrimSpace(r.URL.Query().Get("period"))
	if period == "" {
		period = "7d"
	}
	minTrades, err := parseOptionalInt(r, "min_trades", 20)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	items, err := s.store.GetLeaderboard(r.Context(), metric, period, minTrades)
	if err != nil {
		if strings.Contains(err.Error(), "invalid") {
			s.respondError(w, http.StatusBadRequest, "invalid metric or period")
			return
		}
		s.logger.Error("get leaderboard failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to load leaderboard")
		return
	}
	s.respondJSON(w, http.StatusOK, leaderboardResponse{Items: items})
}

func (s *Service) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondMethodNotAllowed(w)
		return
	}
	status, err := s.store.GetSystemStatus(r.Context())
	if err != nil {
		s.logger.Error("get system status failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "failed to get system status")
		return
	}
	s.respondJSON(w, http.StatusOK, status)
}

func (s *Service) handleWebsocket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondMethodNotAllowed(w)
		return
	}
	upgrader := websocketUpgrader
	upgrader.CheckOrigin = func(req *http.Request) bool {
		origin := strings.TrimSpace(req.Header.Get("Origin"))
		return s.isOriginAllowed(origin)
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("websocket upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	subs := newSubscriptionSet()
	readErrCh := make(chan error, 1)
	go s.websocketReadLoop(ctx, conn, subs, readErrCh)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-readErrCh:
			if err != nil {
				s.logger.Debug("websocket read loop ended", "err", err)
			}
			return
		case <-ticker.C:
			channels := subs.List()
			for _, channel := range channels {
				payload, err := s.getWebsocketPayload(ctx, channel)
				if err != nil {
					_ = writeWebsocketJSON(conn, websocketEnvelope{Type: "error", Channel: channel, Error: "failed to fetch channel data", TS: time.Now().Unix()})
					continue
				}
				if payload == nil {
					continue
				}
				if err := writeWebsocketJSON(conn, websocketEnvelope{Type: "event", Channel: channel, Data: payload, TS: time.Now().Unix()}); err != nil {
					return
				}
			}
		}
	}
}

func (s *Service) websocketReadLoop(ctx context.Context, conn *websocket.Conn, subs *subscriptionSet, readErrCh chan<- error) {
	conn.SetReadLimit(1024 * 1024)
	if err := conn.SetReadDeadline(time.Now().Add(90 * time.Second)); err == nil {
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		})
	}
	for {
		select {
		case <-ctx.Done():
			readErrCh <- nil
			return
		default:
		}
		var message websocketSubscribeRequest
		if err := conn.ReadJSON(&message); err != nil {
			readErrCh <- err
			return
		}
		message.Type = strings.ToLower(strings.TrimSpace(message.Type))
		message.Channel = strings.TrimSpace(message.Channel)
		if message.Channel == "" {
			continue
		}
		switch message.Type {
		case "subscribe":
			subs.Add(message.Channel)
		case "unsubscribe":
			subs.Remove(message.Channel)
		}
	}
}

func (s *Service) getWebsocketPayload(ctx context.Context, channel string) (any, error) {
	switch {
	case strings.HasPrefix(channel, "market.price."):
		market := strings.TrimSpace(strings.TrimPrefix(channel, "market.price."))
		price, err := s.store.GetLatestMarketPrice(ctx, market)
		if err != nil {
			if errors.Is(err, indexer.ErrNotFound) {
				return nil, nil
			}
			return nil, err
		}
		return price, nil
	case strings.HasPrefix(channel, "chart.ticks."):
		market := strings.TrimSpace(strings.TrimPrefix(channel, "chart.ticks."))
		candles, err := s.store.GetCandles(ctx, market, 120)
		if err != nil {
			return nil, err
		}
		return map[string]any{"market": market, "candles": candles}, nil
	case channel == "agent.signals":
		trades, _, _, err := s.store.ListTrades(ctx, indexer.TradeFilter{Limit: 128, Offset: 0})
		if err != nil {
			return nil, err
		}
		items := make([]map[string]any, 0, 16)
		seen := map[string]struct{}{}
		for _, trade := range trades {
			if _, exists := seen[trade.AgentID]; exists {
				continue
			}
			seen[trade.AgentID] = struct{}{}
			confidence := deterministicConfidence(trade.AgentID, trade.ExitTime)
			items = append(items, map[string]any{
				"agent_name": trade.AgentName,
				"side":       trade.Side,
				"price":      trade.ExitPrice,
				"confidence": confidence,
				"ts":         trade.ExitTime,
			})
			if len(items) >= 20 {
				break
			}
		}
		return items, nil
	case channel == "agent.executions":
		trades, _, _, err := s.store.ListTrades(ctx, indexer.TradeFilter{Limit: 128, Offset: 0})
		if err != nil {
			return nil, err
		}
		items := make([]map[string]any, 0, len(trades))
		for _, trade := range trades {
			items = append(items, map[string]any{
				"agent_id": trade.AgentID,
				"trade_id": trade.ID,
				"side":     trade.Side,
				"price":    trade.ExitPrice,
				"qty":      trade.Qty,
				"ts":       trade.ExitTime,
			})
		}
		return items, nil
	case channel == "portfolio.updates":
		summary, err := s.store.GetPortfolioSummary(ctx, "7d")
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"total_equity": summary.TotalEquity,
			"total_pnl":    summary.TotalPNL,
		}, nil
	case channel == "leaderboard.updates":
		items, err := s.store.GetLeaderboard(ctx, "pnl_pct", "7d", 20)
		if err != nil {
			return nil, err
		}
		rankChanges := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if item.RankChange == 0 {
				continue
			}
			rankChanges = append(rankChanges, map[string]any{
				"agent_id": item.AgentID,
				"old_rank": item.Rank + item.RankChange,
				"new_rank": item.Rank,
			})
		}
		return map[string]any{"rank_changes": rankChanges}, nil
	case channel == "system.status":
		status, err := s.store.GetSystemStatus(ctx)
		if err != nil {
			return nil, err
		}
		return status, nil
	default:
		return nil, nil
	}
}

func (s *Service) requireSession(r *http.Request) (indexer.AuthSessionRecord, error) {
	token, err := bearerTokenFromRequest(r)
	if err != nil {
		return indexer.AuthSessionRecord{}, err
	}
	session, err := s.store.GetAuthSession(r.Context(), hashToken(token))
	if err != nil {
		if errors.Is(err, indexer.ErrUnauthorized) {
			return indexer.AuthSessionRecord{}, fmt.Errorf("invalid or expired session")
		}
		return indexer.AuthSessionRecord{}, err
	}
	now := time.Now().Unix()
	if session.RevokedAt != nil || session.ExpiresAt <= now {
		return indexer.AuthSessionRecord{}, fmt.Errorf("invalid or expired session")
	}
	return session, nil
}

func (s *Service) optionalSession(r *http.Request) (indexer.AuthSessionRecord, error) {
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if authorization == "" {
		return indexer.AuthSessionRecord{}, fmt.Errorf("authorization header is required")
	}
	return s.requireSession(r)
}

func bearerTokenFromRequest(r *http.Request) (string, error) {
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if authorization == "" {
		return "", fmt.Errorf("authorization header is required")
	}
	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authorization, bearerPrefix) {
		return "", fmt.Errorf("authorization header must use bearer token")
	}
	token := strings.TrimSpace(strings.TrimPrefix(authorization, bearerPrefix))
	if token == "" {
		return "", fmt.Errorf("authorization token is empty")
	}
	return token, nil
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func newSessionToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, hashToken(token), nil
}

func newID(prefix string) (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + "-" + hex.EncodeToString(raw), nil
}

func isAllowedIntent(intent string) bool {
	switch strings.ToLower(strings.TrimSpace(intent)) {
	case "owner_bind", "session", "live_stepup":
		return true
	default:
		return false
	}
}

func verifyWalletSignature(walletPubkey, signature, message string) error {
	wallet, err := solana.PublicKeyFromBase58(walletPubkey)
	if err != nil {
		return fmt.Errorf("invalid wallet pubkey: %w", err)
	}
	sigBytes, err := decodeSignature(signature)
	if err != nil {
		return err
	}
	if !ed25519.Verify(wallet[:], []byte(message), sigBytes) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func decodeSignature(raw string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("signature is required")
	}

	if sig, err := solana.SignatureFromBase58(trimmed); err == nil {
		bytes := sig[:]
		return bytes, nil
	}
	if bytes, err := base64.StdEncoding.DecodeString(trimmed); err == nil && len(bytes) == ed25519.SignatureSize {
		return bytes, nil
	}
	if bytes, err := base64.RawStdEncoding.DecodeString(trimmed); err == nil && len(bytes) == ed25519.SignatureSize {
		return bytes, nil
	}
	if bytes, err := base64.URLEncoding.DecodeString(trimmed); err == nil && len(bytes) == ed25519.SignatureSize {
		return bytes, nil
	}
	if bytes, err := base64.RawURLEncoding.DecodeString(trimmed); err == nil && len(bytes) == ed25519.SignatureSize {
		return bytes, nil
	}
	if bytes, err := hex.DecodeString(trimmed); err == nil && len(bytes) == ed25519.SignatureSize {
		return bytes, nil
	}

	return nil, fmt.Errorf("unsupported signature encoding")
}

func decodeJSONBody(r *http.Request, destination any) error {
	if r.Body == nil {
		return fmt.Errorf("request body is required")
	}
	defer r.Body.Close()

	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("invalid request body: multiple JSON values")
	}
	return nil
}

func splitAgentSubroute(path string) (string, string) {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/v1/agents/"), "/")
	if trimmed == "" {
		return "", ""
	}
	segments := strings.Split(trimmed, "/")
	agentID := strings.TrimSpace(segments[0])
	if len(segments) == 1 {
		return agentID, ""
	}
	return agentID, strings.Join(segments[1:], "/")
}

func splitStrategiesSubroute(path string) (string, string) {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/v1/strategies/"), "/")
	if trimmed == "" {
		return "", ""
	}
	segments := strings.Split(trimmed, "/")
	strategyID := strings.TrimSpace(segments[0])
	if len(segments) == 1 {
		return strategyID, ""
	}
	return strategyID, strings.Join(segments[1:], "/")
}

func (s *Service) isOriginAllowed(origin string) bool {
	if origin == "" {
		return true
	}
	if s.allowAllOrigins {
		return true
	}
	_, ok := s.allowedOriginSet[origin]
	return ok
}

func deterministicConfidence(agentID string, ts int64) float64 {
	source := agentID + ":" + strconv.FormatInt(ts, 10)
	hash := sha256.Sum256([]byte(source))
	return 0.5 + (float64(hash[0])/255.0)*0.5
}

func parseChartTimeframe(raw string) (string, int64, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "1m", "1min", "1":
		return "1m", 60, nil
	case "5m", "5min", "5":
		return "5m", 5 * 60, nil
	case "15m", "15min", "15":
		return "15m", 15 * 60, nil
	case "1h", "60m", "60min":
		return "1h", 60 * 60, nil
	case "4h", "240m", "240min":
		return "4h", 4 * 60 * 60, nil
	case "1d", "24h":
		return "1d", 24 * 60 * 60, nil
	default:
		return "", 0, fmt.Errorf("timeframe must be one of 1m, 5m, 15m, 1h, 4h, 1d")
	}
}

func writeWebsocketJSON(conn *websocket.Conn, payload websocketEnvelope) error {
	if err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	return conn.WriteJSON(payload)
}

type subscriptionSet struct {
	mu    sync.RWMutex
	items map[string]struct{}
}

func newSubscriptionSet() *subscriptionSet {
	return &subscriptionSet{items: map[string]struct{}{}}
}

func (s *subscriptionSet) Add(channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[channel] = struct{}{}
}

func (s *subscriptionSet) Remove(channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, channel)
}

func (s *subscriptionSet) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.items))
	for channel := range s.items {
		out = append(out, channel)
	}
	return out
}
