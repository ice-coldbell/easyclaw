const DEFAULT_API_BASE_URL = "http://127.0.0.1:8080";
const SESSION_STORAGE_KEY = "easyclaw_session";

export type TradingMode = "paper" | "live";

interface StoredSession {
  token: string;
  expiresAt?: number;
}

export interface AgentRiskProfile {
  max_position_usdc: number;
  daily_loss_limit_usdc: number;
  kill_switch_enabled: boolean;
}

export interface AgentRecord {
  id: string;
  name: string;
  strategy_id: string;
  status: "stopped" | "paper" | "live";
  owner_pubkey: string;
  risk_profile: AgentRiskProfile;
  created_at: number;
}

export interface AgentBindingRecord {
  bound: boolean;
  owner_pubkey: string;
  bound_at?: number;
  session_expires_at?: number;
}

export interface StrategyTemplateRecord {
  id: string;
  name: string;
  description: string;
  risk_level: string;
  rules: Record<string, unknown>;
}

export interface StrategyRecord {
  id: string;
  name: string;
  entry_rules: Record<string, unknown>;
  exit_rules: Record<string, unknown>;
  risk_defaults: Record<string, unknown>;
  owner_pubkey: string;
  is_published: boolean;
  published_at?: number;
  created_at: number;
  updated_at: number;
}

export interface TradeRecord {
  id: string;
  agent_id: string;
  agent_name: string;
  side: "long" | "short";
  entry_price: number;
  exit_price: number;
  qty: number;
  fee: number;
  pnl: number;
  tx_sig: string;
  entry_time: number;
  exit_time: number;
  market_id: number;
}

export interface EquityPoint {
  ts: number;
  value: number;
}

export interface DailyPnlPoint {
  date: string;
  pnl: number;
}

export interface PortfolioAgentSummary {
  id: string;
  name: string;
  pnl_pct: number;
  pnl_abs: number;
  win_rate: number;
  total_trades: number;
  drawdown: number;
  equity: number;
}

export interface PortfolioSummary {
  total_equity: number;
  total_pnl: number;
  total_pnl_pct: number;
  max_drawdown: number;
  active_agents: number;
  equity_history: EquityPoint[];
  agents: PortfolioAgentSummary[];
}

export interface AgentPortfolioSummary {
  pnl_pct: number;
  pnl_abs: number;
  win_rate: number;
  total_trades: number;
  drawdown: number;
  daily_pnl: DailyPnlPoint[];
  equity_history: EquityPoint[];
}

export interface LeaderboardItem {
  rank: number;
  rank_change: number;
  agent_id: string;
  agent_name: string;
  win_rate: number;
  pnl_pct: number;
  total_trades: number;
  max_drawdown: number;
  sparkline: number[];
}

export interface SystemStatus {
  dex_connected: boolean;
  ws_latency_ms: number;
  last_indexed_slot: number;
  indexer_lag_ms: number;
}

export interface AuthChallengeResponse {
  challenge_id: string;
  message: string;
}

export interface AuthTokenResponse {
  session_token: string;
  expires_at: number;
}

export interface TradingSignal {
  agent_name: string;
  side: "long" | "short";
  price: number;
  confidence: number;
  ts: number;
}

export interface CandleRecord {
  ts: number;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
}

export interface ChartTicksPayload {
  market: string;
  candles: CandleRecord[];
}

export type ChartTimeframe = "1m" | "5m" | "15m" | "1h" | "4h" | "1d";

export interface ChartCandlesResponse {
  market: string;
  timeframe: ChartTimeframe;
  interval_sec: number;
  candles: CandleRecord[];
}

export interface MarketPricePayload {
  market: string;
  source: string;
  feed_id: string;
  slot: number;
  publish_time: number;
  price: number;
  conf: number;
  expo: number;
  received_at: number;
}

interface ApiRequestOptions {
  method?: "GET" | "POST" | "PATCH" | "DELETE";
  body?: unknown;
  query?: Record<string, string | number | boolean | undefined | null>;
  auth?: boolean;
  token?: string;
  signal?: AbortSignal;
}

interface ListResponse<T> {
  items: T[];
  limit: number;
  offset: number;
}

interface LeaderboardResponse {
  items: LeaderboardItem[];
}

function getApiBaseUrl(): string {
  const configured = (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? DEFAULT_API_BASE_URL;
  return configured.replace(/\/$/, "");
}

export function getWsUrl(): string {
  const configured = import.meta.env.VITE_WS_URL as string | undefined;
  if (configured && configured.trim().length > 0) {
    return configured.trim();
  }

  const apiBaseUrl = getApiBaseUrl();
  if (apiBaseUrl.startsWith("https://")) {
    return `${apiBaseUrl.replace("https://", "wss://")}/ws`;
  }
  if (apiBaseUrl.startsWith("http://")) {
    return `${apiBaseUrl.replace("http://", "ws://")}/ws`;
  }
  return `${apiBaseUrl}/ws`;
}

export function getSession(): StoredSession | null {
  if (typeof window === "undefined") {
    return null;
  }

  try {
    const raw = window.localStorage.getItem(SESSION_STORAGE_KEY);
    if (!raw) {
      return null;
    }
    const parsed = JSON.parse(raw) as StoredSession;
    if (!parsed || typeof parsed.token !== "string" || parsed.token.length === 0) {
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

export function getSessionToken(): string | null {
  const envToken = import.meta.env.VITE_API_TOKEN as string | undefined;
  if (envToken && envToken.trim().length > 0) {
    return envToken.trim();
  }
  return getSession()?.token ?? null;
}

export function setSessionToken(token: string, expiresAt?: number): void {
  if (typeof window === "undefined") {
    return;
  }

  const session: StoredSession = { token };
  if (typeof expiresAt === "number") {
    session.expiresAt = expiresAt;
  }
  window.localStorage.setItem(SESSION_STORAGE_KEY, JSON.stringify(session));
}

export function clearSessionToken(): void {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.removeItem(SESSION_STORAGE_KEY);
}

async function request<T>(path: string, options: ApiRequestOptions = {}): Promise<T> {
  const {
    method = "GET",
    body,
    query,
    auth = false,
    token,
    signal,
  } = options;

  const url = new URL(path, `${getApiBaseUrl()}/`);
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value === undefined || value === null || String(value).trim() === "") {
        continue;
      }
      url.searchParams.set(key, String(value));
    }
  }

  const headers: Record<string, string> = {
    Accept: "application/json",
  };

  const resolvedToken = token ?? getSessionToken();
  if (resolvedToken) {
    headers.Authorization = `Bearer ${resolvedToken}`;
  } else if (auth) {
    throw new Error("Session token is required for this request.");
  }

  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
  }

  const response = await fetch(url.toString(), {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  });

  const text = await response.text();
  let payload: unknown = null;
  if (text.length > 0) {
    try {
      payload = JSON.parse(text) as unknown;
    } catch {
      payload = text;
    }
  }

  if (!response.ok) {
    const message =
      typeof payload === "object" && payload !== null && "error" in payload
        ? String((payload as { error: unknown }).error)
        : `HTTP ${response.status}`;

    if (response.status === 401 || response.status === 403) {
      throw new Error(`Unauthorized: ${message}`);
    }

    throw new Error(message);
  }

  return payload as T;
}

export async function getSystemStatus(signal?: AbortSignal): Promise<SystemStatus> {
  return request<SystemStatus>("/v1/system/status", { signal });
}

export async function getChartCandles(params: {
  market: string;
  timeframe: ChartTimeframe;
  limit?: number;
}, signal?: AbortSignal): Promise<ChartCandlesResponse> {
  return request<ChartCandlesResponse>("/v1/chart/candles", {
    query: {
      market: params.market,
      timeframe: params.timeframe,
      limit: params.limit,
    },
    signal,
  });
}

export async function getAgents(signal?: AbortSignal): Promise<AgentRecord[]> {
  return request<AgentRecord[]>("/v1/agents", { signal });
}

export function filterAgentsByOwner(agents: AgentRecord[], ownerPubkey: string | null | undefined): AgentRecord[] {
  if (!ownerPubkey || ownerPubkey.trim().length === 0) {
    return [];
  }
  return agents.filter((agent) => agent.owner_pubkey === ownerPubkey);
}

export async function getAgent(agentId: string, signal?: AbortSignal): Promise<AgentRecord> {
  return request<AgentRecord>(`/v1/agents/${encodeURIComponent(agentId)}`, { signal });
}

export async function createAgent(params: {
  name: string;
  strategyId: string;
  riskProfile: AgentRiskProfile;
}): Promise<{ agent_id: string }> {
  return request<{ agent_id: string }>("/v1/agents", {
    method: "POST",
    auth: true,
    body: {
      name: params.name,
      strategy_id: params.strategyId,
      risk_profile: params.riskProfile,
    },
  });
}

export async function getAgentBinding(agentId: string, signal?: AbortSignal): Promise<AgentBindingRecord> {
  return request<AgentBindingRecord>(`/v1/agents/${encodeURIComponent(agentId)}/owner-binding`, { signal });
}

export async function rebindAgentOwner(agentId: string, params: { challengeId: string; signature: string }): Promise<{ bound_at: number }> {
  return request<{ bound_at: number }>(`/v1/agents/${encodeURIComponent(agentId)}/owner-binding/rebind`, {
    method: "POST",
    auth: true,
    body: {
      challenge_id: params.challengeId,
      signature: params.signature,
    },
  });
}

export async function startAgentSession(agentId: string, mode: TradingMode): Promise<{ session_id: string; started_at: number }> {
  return request<{ session_id: string; started_at: number }>(`/v1/agents/${encodeURIComponent(agentId)}/sessions`, {
    method: "POST",
    auth: true,
    body: { mode },
  });
}

export async function stopAgentSession(agentId: string, sessionId: string): Promise<void> {
  await request<void>(`/v1/agents/${encodeURIComponent(agentId)}/sessions/${encodeURIComponent(sessionId)}`, {
    method: "DELETE",
    auth: true,
  });
}

export async function killSwitch(agentIds: string[] | ["all"]): Promise<{ stopped_count: number }> {
  return request<{ stopped_count: number }>("/v1/safety/kill-switch", {
    method: "POST",
    auth: true,
    body: { agent_ids: agentIds },
  });
}

export async function getAgentRisk(agentId: string, signal?: AbortSignal): Promise<AgentRiskProfile> {
  return request<AgentRiskProfile>(`/v1/agents/${encodeURIComponent(agentId)}/risk`, { signal });
}

export async function patchAgentRisk(agentId: string, patch: Partial<AgentRiskProfile>): Promise<{ updated_risk_profile: AgentRiskProfile }> {
  return request<{ updated_risk_profile: AgentRiskProfile }>(`/v1/agents/${encodeURIComponent(agentId)}/risk`, {
    method: "PATCH",
    auth: true,
    body: patch,
  });
}

export async function getStrategyTemplates(signal?: AbortSignal): Promise<StrategyTemplateRecord[]> {
  return request<StrategyTemplateRecord[]>("/v1/strategy/templates", { signal });
}

export async function createStrategy(params: {
  name: string;
  entryRules: Record<string, unknown>;
  exitRules: Record<string, unknown>;
  riskDefaults: Record<string, unknown>;
}): Promise<{ strategy_id: string }> {
  return request<{ strategy_id: string }>("/v1/strategies", {
    method: "POST",
    auth: true,
    body: {
      name: params.name,
      entry_rules: params.entryRules,
      exit_rules: params.exitRules,
      risk_defaults: params.riskDefaults,
    },
  });
}

export async function getStrategy(strategyId: string, signal?: AbortSignal): Promise<StrategyRecord> {
  return request<StrategyRecord>(`/v1/strategies/${encodeURIComponent(strategyId)}`, { signal });
}

export async function patchStrategy(strategyId: string, patch: {
  name?: string;
  entryRules?: Record<string, unknown>;
  exitRules?: Record<string, unknown>;
}): Promise<StrategyRecord> {
  return request<StrategyRecord>(`/v1/strategies/${encodeURIComponent(strategyId)}`, {
    method: "PATCH",
    auth: true,
    body: {
      name: patch.name,
      entry_rules: patch.entryRules,
      exit_rules: patch.exitRules,
    },
  });
}

export async function publishStrategy(strategyId: string): Promise<{ published_at: number }> {
  return request<{ published_at: number }>(`/v1/strategies/${encodeURIComponent(strategyId)}/publish`, {
    method: "POST",
    auth: true,
  });
}

export async function getPortfolio(period: "7d" | "30d" | "all", signal?: AbortSignal): Promise<PortfolioSummary> {
  return request<PortfolioSummary>("/v1/portfolio", {
    query: { period },
    signal,
  });
}

export async function getAgentPortfolio(agentId: string, period: "7d" | "30d" | "all", signal?: AbortSignal): Promise<AgentPortfolioSummary> {
  return request<AgentPortfolioSummary>(`/v1/portfolio/agents/${encodeURIComponent(agentId)}`, {
    query: { period },
    signal,
  });
}

export async function getTrades(params: {
  agentId?: string;
  from?: number;
  to?: number;
  limit?: number;
  offset?: number;
}, signal?: AbortSignal): Promise<ListResponse<TradeRecord>> {
  return request<ListResponse<TradeRecord>>("/v1/trades", {
    query: {
      agent_id: params.agentId,
      from: params.from,
      to: params.to,
      limit: params.limit,
      offset: params.offset,
    },
    signal,
  });
}

export async function getLeaderboard(params: {
  metric: "win_rate" | "pnl_pct";
  period: "all_time" | "30d" | "7d";
  minTrades: number;
}, signal?: AbortSignal): Promise<LeaderboardItem[]> {
  const response = await request<LeaderboardResponse>("/v1/leaderboard", {
    query: {
      metric: params.metric,
      period: params.period,
      min_trades: params.minTrades,
    },
    signal,
  });
  return response.items;
}

export async function getPositions(params: {
  userMargin?: string;
  marketId?: number;
  limit?: number;
  offset?: number;
}, signal?: AbortSignal): Promise<ListResponse<Record<string, unknown>>> {
  return request<ListResponse<Record<string, unknown>>>("/api/v1/positions", {
    query: {
      user_margin: params.userMargin,
      market_id: params.marketId,
      limit: params.limit,
      offset: params.offset,
    },
    signal,
  });
}

export async function getOrders(params: {
  userMargin?: string;
  userPubkey?: string;
  marketId?: number;
  status?: string;
  limit?: number;
  offset?: number;
}, signal?: AbortSignal): Promise<ListResponse<Record<string, unknown>>> {
  return request<ListResponse<Record<string, unknown>>>("/api/v1/orders", {
    query: {
      user_margin: params.userMargin,
      user_pubkey: params.userPubkey,
      market_id: params.marketId,
      status: params.status,
      limit: params.limit,
      offset: params.offset,
    },
    signal,
  });
}

export async function getFills(params: {
  userMargin?: string;
  userPubkey?: string;
  marketId?: number;
  limit?: number;
  offset?: number;
}, signal?: AbortSignal): Promise<ListResponse<Record<string, unknown>>> {
  return request<ListResponse<Record<string, unknown>>>("/api/v1/fills", {
    query: {
      user_margin: params.userMargin,
      user_pubkey: params.userPubkey,
      market_id: params.marketId,
      limit: params.limit,
      offset: params.offset,
    },
    signal,
  });
}

export async function getPositionHistory(params: {
  userMargin?: string;
  marketId?: number;
  limit?: number;
  offset?: number;
}, signal?: AbortSignal): Promise<ListResponse<Record<string, unknown>>> {
  return request<ListResponse<Record<string, unknown>>>("/api/v1/position-history", {
    query: {
      user_margin: params.userMargin,
      market_id: params.marketId,
      limit: params.limit,
      offset: params.offset,
    },
    signal,
  });
}

export async function createAuthChallenge(params: {
  walletPubkey: string;
  intent: "owner_bind" | "session" | "live_stepup";
}): Promise<AuthChallengeResponse> {
  return request<AuthChallengeResponse>("/v1/auth/challenge", {
    method: "POST",
    body: {
      wallet_pubkey: params.walletPubkey,
      intent: params.intent,
    },
  });
}

export async function verifyAuthSignature(params: {
  challengeId: string;
  signature: string;
  walletPubkey: string;
}): Promise<AuthTokenResponse> {
  return request<AuthTokenResponse>("/v1/auth/verify-signature", {
    method: "POST",
    body: {
      challenge_id: params.challengeId,
      signature: params.signature,
      wallet_pubkey: params.walletPubkey,
    },
  });
}

export async function refreshAuthSession(token?: string): Promise<AuthTokenResponse> {
  return request<AuthTokenResponse>("/v1/auth/session/refresh", {
    method: "POST",
    auth: true,
    token,
  });
}

export interface WsMessageEnvelope<T = unknown> {
  type: string;
  channel?: string;
  data?: T;
  error?: string;
  ts?: number;
}

export function createChannelSocket(options: {
  channels: string[];
  onMessage: (message: WsMessageEnvelope) => void;
  onOpen?: () => void;
  onError?: (error: Event) => void;
}): () => void {
  const socket = new WebSocket(getWsUrl());

  socket.onopen = () => {
    for (const channel of options.channels) {
      socket.send(
        JSON.stringify({
          type: "subscribe",
          channel,
        }),
      );
    }
    options.onOpen?.();
  };

  socket.onmessage = (event) => {
    try {
      const payload = JSON.parse(String(event.data)) as WsMessageEnvelope;
      options.onMessage(payload);
    } catch {
      // Ignore malformed payload.
    }
  };

  socket.onerror = (event) => {
    options.onError?.(event);
  };

  return () => {
    if (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING) {
      socket.close();
    }
  };
}
