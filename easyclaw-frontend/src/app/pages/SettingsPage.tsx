import { useCallback, useEffect, useMemo, useState } from "react";
import { Check, RefreshCw, Shield, Wallet, Wifi } from "lucide-react";
import {
  AgentBindingRecord,
  AgentRecord,
  AuthChallengeResponse,
  SystemStatus,
  createAuthChallenge,
  createChannelSocket,
  filterAgentsByOwner,
  getAgentBinding,
  getAgents,
  getSession,
  getSystemStatus,
  rebindAgentOwner,
  refreshAuthSession,
  setSessionToken,
} from "../lib/backend";
import { formatUnixTime } from "../lib/format";
import { useI18n } from "../i18n";
import { useWallet } from "../lib/wallet";

export function SettingsPage() {
  const { t, localeTag } = useI18n();
  const { walletPubkey } = useWallet();
  const [agents, setAgents] = useState<AgentRecord[]>([]);
  const [bindingsByAgent, setBindingsByAgent] = useState<Record<string, AgentBindingRecord>>({});
  const [selectedAgentID, setSelectedAgentID] = useState<string>("");
  const [systemStatus, setSystemStatus] = useState<SystemStatus | null>(null);
  const [wsConnected, setWsConnected] = useState(false);
  const [loading, setLoading] = useState(true);
  const [refreshingSession, setRefreshingSession] = useState(false);
  const [requestError, setRequestError] = useState<string | null>(null);

  const [ownerWalletPubkey, setOwnerWalletPubkey] = useState("");
  const [ownerChallenge, setOwnerChallenge] = useState<AuthChallengeResponse | null>(null);
  const [ownerSignature, setOwnerSignature] = useState("");
  const [creatingChallenge, setCreatingChallenge] = useState(false);
  const [rebindingOwner, setRebindingOwner] = useState(false);

  const loadSettings = useCallback(async () => {
    try {
      setLoading(true);
      setRequestError(null);

      const [systemResponse, allAgents] = await Promise.all([
        getSystemStatus(),
        walletPubkey ? getAgents() : Promise.resolve([] as AgentRecord[]),
      ]);
      const ownedAgents = filterAgentsByOwner(allAgents, walletPubkey);

      setAgents(ownedAgents);
      setSystemStatus(systemResponse);
      setSelectedAgentID((current) => {
        if (ownedAgents.some((agent) => agent.id === current)) {
          return current;
        }
        return ownedAgents[0]?.id || "";
      });

      const bindingEntries = await Promise.all(
        ownedAgents.map(async (agent) => {
          try {
            const binding = await getAgentBinding(agent.id);
            return [agent.id, binding] as const;
          } catch {
            return [agent.id, {
              bound: false,
              owner_pubkey: "",
            }] as const;
          }
        }),
      );
      setBindingsByAgent(Object.fromEntries(bindingEntries));
    } catch (loadError) {
      const message = loadError instanceof Error ? loadError.message : String(loadError);
      setRequestError(message);
    } finally {
      setLoading(false);
    }
  }, [walletPubkey]);

  useEffect(() => {
    void loadSettings();
  }, [loadSettings]);

  useEffect(() => {
    if (!walletPubkey) {
      return;
    }
    setOwnerWalletPubkey((current) => current || walletPubkey);
  }, [walletPubkey]);

  useEffect(() => createChannelSocket({
    channels: ["system.status"],
    onOpen: () => setWsConnected(true),
    onError: () => setWsConnected(false),
    onMessage: (message) => {
      if (message.channel === "system.status" && message.data && typeof message.data === "object") {
        setSystemStatus(message.data as SystemStatus);
      }
    },
  }), []);

  const selectedBinding = useMemo(
    () => (selectedAgentID ? bindingsByAgent[selectedAgentID] : undefined),
    [bindingsByAgent, selectedAgentID],
  );

  const boundAgentsCount = useMemo(
    () => Object.values(bindingsByAgent).filter((binding) => binding.bound).length,
    [bindingsByAgent],
  );

  const session = getSession();
  const sessionExpiresAt = session?.expiresAt;
  const sessionIssuedAt = sessionExpiresAt ? sessionExpiresAt - (24 * 60 * 60) : undefined;
  const sessionValid = Boolean(session && sessionExpiresAt && sessionExpiresAt > Math.floor(Date.now() / 1000));

  const handleSessionRefresh = useCallback(async () => {
    try {
      setRefreshingSession(true);
      setRequestError(null);
      const refreshed = await refreshAuthSession();
      setSessionToken(refreshed.session_token, refreshed.expires_at);
      await loadSettings();
    } catch (refreshError) {
      const message = refreshError instanceof Error ? refreshError.message : String(refreshError);
      setRequestError(message);
    } finally {
      setRefreshingSession(false);
    }
  }, [loadSettings]);

  const handleCreateChallenge = useCallback(async () => {
    if (!ownerWalletPubkey.trim()) {
      setRequestError(t("Owner 지갑 주소를 입력하세요.", "Enter owner wallet pubkey."));
      return;
    }

    try {
      setCreatingChallenge(true);
      setRequestError(null);
      const challenge = await createAuthChallenge({
        walletPubkey: ownerWalletPubkey.trim(),
        intent: "owner_bind",
      });
      setOwnerChallenge(challenge);
    } catch (challengeError) {
      const message = challengeError instanceof Error ? challengeError.message : String(challengeError);
      setRequestError(message);
    } finally {
      setCreatingChallenge(false);
    }
  }, [ownerWalletPubkey, t]);

  const handleOwnerRebind = useCallback(async () => {
    if (!selectedAgentID) {
      setRequestError(t("에이전트를 선택하세요.", "Select an agent."));
      return;
    }
    if (!ownerChallenge?.challenge_id) {
      setRequestError(t("먼저 챌린지를 생성하세요.", "Create challenge first."));
      return;
    }
    if (!ownerSignature.trim()) {
      setRequestError(t("서명을 입력하세요.", "Enter signature."));
      return;
    }

    try {
      setRebindingOwner(true);
      setRequestError(null);
      await rebindAgentOwner(selectedAgentID, {
        challengeId: ownerChallenge.challenge_id,
        signature: ownerSignature.trim(),
      });
      setOwnerSignature("");
      setOwnerChallenge(null);
      await loadSettings();
    } catch (rebindError) {
      const message = rebindError instanceof Error ? rebindError.message : String(rebindError);
      setRequestError(message);
    } finally {
      setRebindingOwner(false);
    }
  }, [loadSettings, ownerChallenge, ownerSignature, selectedAgentID, t]);

  return (
    <div className="p-6 max-w-3xl mx-auto">
      <div className="mb-8">
        <h1 className="text-[1.5rem]" style={{ fontWeight: 600 }}>{t("계정 및 연결 설정", "Account & Connection Settings")}</h1>
        <p className="text-[0.875rem] text-muted-foreground mt-1">{t("Owner 바인딩, 지갑, 세션 상태 관리", "Manage owner binding, wallets, and session status")}</p>
      </div>

      {requestError && (
        <div className="mb-6 bg-destructive/10 border border-destructive/20 text-destructive rounded-lg px-4 py-3 text-[0.8125rem]">
          {requestError}
        </div>
      )}

      {!walletPubkey && (
        <div className="mb-6 bg-card border border-border rounded-lg px-4 py-3 text-[0.8125rem] text-muted-foreground">
          {t("우측 상단에서 월렛을 연결하면 내 에이전트 설정만 표시됩니다.", "Connect your wallet from the top-right to show only your agent settings.")}
        </div>
      )}

      {walletPubkey && agents.length === 0 && (
        <div className="mb-6 bg-card border border-border rounded-lg px-4 py-3 text-[0.8125rem] text-muted-foreground">
          {t("연결된 월렛 소유의 에이전트가 없습니다.", "No agents are owned by the connected wallet.")}
        </div>
      )}

      <div className="space-y-6">
        <div className="bg-card border border-border rounded-xl p-6">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-primary/10 flex items-center justify-center">
                <Shield className="w-5 h-5 text-primary" />
              </div>
              <div>
                <h3 className="text-[0.9375rem]" style={{ fontWeight: 600 }}>{t("Agent-Owner 바인딩", "Agent-Owner Binding")}</h3>
                <p className="text-[0.75rem] text-muted-foreground">{t("에이전트 지갑 소유권 인증 상태", "Agent wallet ownership verification status")}</p>
              </div>
            </div>
            <span className={`text-[0.75rem] px-3 py-1 rounded-full flex items-center gap-1.5 ${
              selectedBinding?.bound ? "text-[#22c55e] bg-[#22c55e]/10" : "text-[#ef4444] bg-[#ef4444]/10"
            }`}>
              <Check className="w-3 h-3" /> {selectedBinding?.bound ? t("연결됨", "Connected") : t("미연결", "Not Bound")}
            </span>
          </div>
          <div className="mb-3">
            <select
              value={selectedAgentID}
              onChange={(event) => setSelectedAgentID(event.target.value)}
              disabled={agents.length === 0}
              className="bg-card border border-border rounded px-2 py-1 text-[0.75rem] focus:outline-none"
            >
              {agents.map((agent) => (
                <option key={agent.id} value={agent.id}>{agent.name}</option>
              ))}
            </select>
          </div>
          <div className="space-y-2 text-[0.8125rem]">
            <div className="flex justify-between py-2 border-b border-border">
              <span className="text-muted-foreground">{t("바인딩된 에이전트", "Bound Agents")}</span>
              <span>{boundAgentsCount}</span>
            </div>
            <div className="flex justify-between py-2 border-b border-border">
              <span className="text-muted-foreground">{t("최종 검증", "Last Verification")}</span>
              <span>{formatUnixTime(selectedBinding?.bound_at, localeTag)}</span>
            </div>
            <div className="flex justify-between py-2">
              <span className="text-muted-foreground">Owner Pubkey</span>
              <span className="text-[0.75rem]" style={{ fontFamily: "'JetBrains Mono', monospace" }}>{selectedBinding?.owner_pubkey || "-"}</span>
            </div>
          </div>

          <div className="mt-4 grid grid-cols-1 gap-3">
            <input
              value={ownerWalletPubkey}
              onChange={(event) => setOwnerWalletPubkey(event.target.value)}
              placeholder={t("Owner wallet pubkey", "Owner wallet pubkey")}
              className="w-full bg-input-background border border-border rounded-lg px-3 py-2 text-[0.8125rem] focus:border-primary focus:outline-none"
              style={{ fontFamily: "'JetBrains Mono', monospace" }}
            />
            <div className="flex items-center gap-2">
              <button
                onClick={() => void handleCreateChallenge()}
                disabled={creatingChallenge}
                className="text-[0.8125rem] text-muted-foreground hover:text-foreground border border-border px-4 py-2 rounded-lg transition-colors flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <RefreshCw className="w-3.5 h-3.5" /> {creatingChallenge ? t("챌린지 생성 중...", "Creating challenge...") : t("Owner 재바인딩 챌린지", "Create Rebind Challenge")}
              </button>
            </div>
            {ownerChallenge && (
              <div className="bg-secondary/50 border border-border rounded-lg p-3 text-[0.75rem]">
                <p className="text-muted-foreground mb-1">challenge_id: {ownerChallenge.challenge_id}</p>
                <p className="text-muted-foreground break-all" style={{ fontFamily: "'JetBrains Mono', monospace" }}>{ownerChallenge.message}</p>
              </div>
            )}
            <input
              value={ownerSignature}
              onChange={(event) => setOwnerSignature(event.target.value)}
              placeholder={t("Signature (base58/base64/hex)", "Signature (base58/base64/hex)")}
              className="w-full bg-input-background border border-border rounded-lg px-3 py-2 text-[0.8125rem] focus:border-primary focus:outline-none"
            />
            <button
              onClick={() => void handleOwnerRebind()}
              disabled={rebindingOwner}
              className="text-[0.8125rem] text-primary border border-primary/30 px-4 py-2 rounded-lg hover:bg-primary/5 transition-colors flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <RefreshCw className="w-3.5 h-3.5" /> {rebindingOwner ? t("재바인딩 중...", "Rebinding...") : t("Owner 재바인딩 실행", "Execute Owner Rebind")}
            </button>
          </div>
        </div>

        <div className="bg-card border border-border rounded-xl p-6">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-[#3b82f6]/10 flex items-center justify-center">
                <Wallet className="w-5 h-5 text-[#3b82f6]" />
              </div>
              <div>
                <h3 className="text-[0.9375rem]" style={{ fontWeight: 600 }}>{t("세션 상태", "Session Status")}</h3>
                <p className="text-[0.75rem] text-muted-foreground">{t("지갑 세션 서명 및 인증 상태", "Wallet session signature and auth status")}</p>
              </div>
            </div>
            <span className={`text-[0.75rem] px-3 py-1 rounded-full flex items-center gap-1.5 ${
              sessionValid ? "text-[#22c55e] bg-[#22c55e]/10" : "text-[#ef4444] bg-[#ef4444]/10"
            }`}>
              <Check className="w-3 h-3" /> {sessionValid ? t("유효", "Valid") : t("만료", "Expired")}
            </span>
          </div>
          <div className="space-y-2 text-[0.8125rem]">
            <div className="flex justify-between py-2 border-b border-border">
              <span className="text-muted-foreground">{t("세션 발급 시간", "Session Issued At")}</span>
              <span>{formatUnixTime(sessionIssuedAt, localeTag)}</span>
            </div>
            <div className="flex justify-between py-2 border-b border-border">
              <span className="text-muted-foreground">{t("만료 시간", "Expires At")}</span>
              <span>{formatUnixTime(sessionExpiresAt, localeTag)}</span>
            </div>
            <div className="flex justify-between py-2">
              <span className="text-muted-foreground">Live Step-up</span>
              <span className={sessionValid ? "text-[#22c55e]" : "text-muted-foreground"}>{sessionValid ? t("승인됨", "Approved") : t("미승인", "Not Approved")}</span>
            </div>
          </div>
          <button
            onClick={() => void handleSessionRefresh()}
            disabled={refreshingSession}
            className="mt-4 text-[0.8125rem] text-primary border border-primary/30 px-4 py-2 rounded-lg hover:bg-primary/5 transition-colors flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <RefreshCw className="w-3.5 h-3.5" /> {refreshingSession ? t("세션 갱신 중...", "Refreshing session...") : t("세션 재서명", "Re-sign Session")}
          </button>
        </div>

        <div className="bg-card border border-border rounded-xl p-6">
          <div className="flex items-center gap-3 mb-4">
            <div className="w-10 h-10 rounded-lg bg-[#22c55e]/10 flex items-center justify-center">
              <Wifi className="w-5 h-5 text-[#22c55e]" />
            </div>
            <div>
              <h3 className="text-[0.9375rem]" style={{ fontWeight: 600 }}>{t("시스템 상태", "System Status")}</h3>
              <p className="text-[0.75rem] text-muted-foreground">{t("DEX 연결 및 WebSocket 상태", "DEX connectivity and WebSocket status")}</p>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            {[
              {
                label: t("DEX 연결", "DEX Connection"),
                status: systemStatus?.dex_connected ? t("정상", "Healthy") : t("장애", "Down"),
                color: systemStatus?.dex_connected ? "#22c55e" : "#ef4444",
              },
              {
                label: "WebSocket",
                status: wsConnected ? t("정상", "Healthy") : t("끊김", "Disconnected"),
                color: wsConnected ? "#22c55e" : "#ef4444",
              },
              {
                label: t("WS 지연", "WS Latency"),
                status: systemStatus ? `${systemStatus.ws_latency_ms}ms` : "--",
                color: "#22c55e",
              },
              {
                label: t("Indexer 지연", "Indexer Lag"),
                status: systemStatus ? `${systemStatus.indexer_lag_ms}ms` : "--",
                color: "#22c55e",
              },
            ].map((item) => (
              <div key={item.label} className="flex items-center justify-between py-2">
                <span className="text-[0.8125rem] text-muted-foreground">{item.label}</span>
                <span className="text-[0.8125rem] flex items-center gap-1.5" style={{ color: item.color }}>
                  <div className="w-1.5 h-1.5 rounded-full" style={{ backgroundColor: item.color }} />
                  {item.status}
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>

      {loading && (
        <p className="mt-6 text-[0.8125rem] text-muted-foreground">{t("설정 로딩 중...", "Loading settings...")}</p>
      )}
    </div>
  );
}
