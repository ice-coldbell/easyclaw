import { useCallback, useEffect, useMemo, useState } from "react";
import { AlertTriangle, OctagonX, Pause, Play, Shield } from "lucide-react";
import {
  AgentRecord,
  TradingMode,
  filterAgentsByOwner,
  getAgents,
  killSwitch,
  startAgentSession,
  stopAgentSession,
} from "../lib/backend";
import { useI18n } from "../i18n";
import { useWallet } from "../lib/wallet";

export function SessionControlPage() {
  const { t } = useI18n();
  const { walletPubkey } = useWallet();
  const [agents, setAgents] = useState<AgentRecord[]>([]);
  const [sessionIdsByAgent, setSessionIdsByAgent] = useState<Record<string, string>>({});
  const [killConfirm, setKillConfirm] = useState(false);
  const [loading, setLoading] = useState(true);
  const [pendingAgentId, setPendingAgentId] = useState<string | null>(null);
  const [pendingKillSwitch, setPendingKillSwitch] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const ownedAgents = useMemo(
    () => filterAgentsByOwner(agents, walletPubkey),
    [agents, walletPubkey],
  );

  const ownedAgentIDs = useMemo(
    () => ownedAgents.map((agent) => agent.id),
    [ownedAgents],
  );

  const loadAgents = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const response = await getAgents();
      setAgents(response);
    } catch (loadError) {
      const message = loadError instanceof Error ? loadError.message : String(loadError);
      setError(message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadAgents();
  }, [loadAgents]);

  const toggleMode = useCallback(async (agent: AgentRecord, mode: TradingMode) => {
    try {
      setPendingAgentId(agent.id);
      setError(null);

      if (agent.status === mode) {
        const knownSessionID = sessionIdsByAgent[agent.id];
        if (knownSessionID) {
          await stopAgentSession(agent.id, knownSessionID);
          setSessionIdsByAgent((prev) => {
            const next = { ...prev };
            delete next[agent.id];
            return next;
          });
        } else {
          await killSwitch([agent.id]);
        }
      } else {
        const started = await startAgentSession(agent.id, mode);
        setSessionIdsByAgent((prev) => ({
          ...prev,
          [agent.id]: started.session_id,
        }));
      }

      await loadAgents();
    } catch (actionError) {
      const message = actionError instanceof Error ? actionError.message : String(actionError);
      setError(message);
    } finally {
      setPendingAgentId(null);
    }
  }, [loadAgents, sessionIdsByAgent]);

  const handleKillSwitch = useCallback(async () => {
    try {
      setPendingKillSwitch(true);
      setError(null);
      if (ownedAgentIDs.length > 0) {
        await killSwitch(ownedAgentIDs);
      }
      setKillConfirm(false);
      setSessionIdsByAgent({});
      await loadAgents();
    } catch (actionError) {
      const message = actionError instanceof Error ? actionError.message : String(actionError);
      setError(message);
    } finally {
      setPendingKillSwitch(false);
    }
  }, [loadAgents, ownedAgentIDs]);

  return (
    <div className="p-6 max-w-5xl mx-auto">
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-[1.5rem]" style={{ fontWeight: 600 }}>{t("세션 제어", "Session Control")}</h1>
          <p className="text-[0.875rem] text-muted-foreground mt-1">{t("Agent Paper/Live 실행 상태 관리", "Manage Paper/Live execution status by agent")}</p>
        </div>
        <button
          onClick={() => setKillConfirm(true)}
          disabled={pendingKillSwitch || ownedAgentIDs.length === 0}
          className="bg-destructive/10 text-destructive border border-destructive/20 px-4 py-2 rounded-lg text-[0.8125rem] hover:bg-destructive/20 transition-colors flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <OctagonX className="w-4 h-4" /> Kill Switch
        </button>
      </div>

      {error && (
        <div className="mb-6 bg-destructive/10 border border-destructive/20 text-destructive rounded-lg px-4 py-3 text-[0.8125rem]">
          {t("세션 제어 중 오류가 발생했습니다:", "Session control failed:")} {error}
        </div>
      )}

      {!walletPubkey && (
        <div className="bg-card border border-border rounded-xl p-5 mb-6 text-[0.8125rem] text-muted-foreground">
          {t("우측 상단에서 월렛을 연결하면 내 에이전트만 표시됩니다.", "Connect your wallet from the top-right to see your agents only.")}
        </div>
      )}

      {walletPubkey && ownedAgents.length === 0 && (
        <div className="bg-card border border-border rounded-xl p-5 mb-6 text-[0.8125rem] text-muted-foreground">
          {t("연결된 월렛 소유의 에이전트가 없습니다.", "No agents are owned by the connected wallet.")}
        </div>
      )}

      {killConfirm && ownedAgentIDs.length > 0 && (
        <div className="bg-destructive/5 border border-destructive/20 rounded-xl p-5 mb-6">
          <div className="flex items-start gap-3">
            <AlertTriangle className="w-5 h-5 text-destructive flex-shrink-0 mt-0.5" />
            <div className="flex-1">
              <h3 className="text-[0.9375rem] text-destructive" style={{ fontWeight: 600 }}>{t("Kill Switch 확인", "Confirm Kill Switch")}</h3>
              <p className="text-[0.8125rem] text-muted-foreground mt-1">
                {t(
                  "모든 활성 에이전트가 즉시 정지됩니다. 열린 포지션은 현재 가격으로 청산됩니다.",
                  "All active agents will stop immediately. Open positions will be closed at current market price.",
                )}
              </p>
              <div className="flex gap-3 mt-4">
                <button
                  onClick={() => void handleKillSwitch()}
                  disabled={pendingKillSwitch}
                  className="bg-destructive text-destructive-foreground px-4 py-2 rounded-lg text-[0.8125rem] hover:bg-destructive/90 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {pendingKillSwitch ? t("정지 실행 중...", "Stopping...") : t("전체 정지 실행", "Execute Full Stop")}
                </button>
                <button onClick={() => setKillConfirm(false)} className="text-muted-foreground text-[0.8125rem] hover:text-foreground">
                  {t("취소", "Cancel")}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      <div className="space-y-4">
        {ownedAgents.map((agent) => {
          const pending = pendingAgentId === agent.id;
          return (
            <div key={agent.id} className="bg-card border border-border rounded-xl p-5">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-4">
                  <div className={`w-10 h-10 rounded-lg flex items-center justify-center ${
                    agent.status === "live" ? "bg-[#22c55e]/10" :
                    agent.status === "paper" ? "bg-[#3b82f6]/10" :
                    "bg-muted"
                  }`}>
                    {agent.status === "live" ? <Play className="w-4 h-4 text-[#22c55e]" /> :
                      agent.status === "paper" ? <Play className="w-4 h-4 text-[#3b82f6]" /> :
                        <Pause className="w-4 h-4 text-muted-foreground" />}
                  </div>
                  <div>
                    <h3 className="text-[0.9375rem]" style={{ fontWeight: 600 }}>{agent.name}</h3>
                    <p className="text-[0.75rem] text-muted-foreground">{agent.strategy_id}</p>
                  </div>
                  <span className={`text-[0.6875rem] px-2.5 py-1 rounded-full ${
                    agent.status === "live" ? "bg-[#22c55e]/10 text-[#22c55e]" :
                      agent.status === "paper" ? "bg-[#3b82f6]/10 text-[#3b82f6]" :
                        "bg-muted text-muted-foreground"
                  }`}>
                    {agent.status === "live" ? "LIVE" : agent.status === "paper" ? "PAPER" : t("정지", "STOPPED")}
                  </span>
                </div>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => void toggleMode(agent, "paper")}
                    disabled={pending}
                    className={`px-4 py-2 rounded-lg text-[0.8125rem] border transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${
                      agent.status === "paper"
                        ? "bg-[#3b82f6]/10 border-[#3b82f6]/30 text-[#3b82f6]"
                        : "border-border text-muted-foreground hover:text-foreground hover:border-[#3b82f6]/30"
                    }`}
                  >
                    {agent.status === "paper" ? t("Paper 중지", "Stop Paper") : t("Paper 시작", "Start Paper")}
                  </button>
                  <button
                    onClick={() => void toggleMode(agent, "live")}
                    disabled={pending}
                    className={`px-4 py-2 rounded-lg text-[0.8125rem] border transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${
                      agent.status === "live"
                        ? "bg-[#22c55e]/10 border-[#22c55e]/30 text-[#22c55e]"
                        : "border-border text-muted-foreground hover:text-foreground hover:border-[#22c55e]/30"
                    }`}
                  >
                    {agent.status === "live" ? t("Live 중지", "Stop Live") : t("Live 시작", "Start Live")}
                  </button>
                </div>
              </div>

              <div className="mt-4 pt-4 border-t border-border grid grid-cols-3 gap-4">
                <div className="flex items-center gap-2">
                  <Shield className="w-3.5 h-3.5 text-muted-foreground" />
                  <span className="text-[0.75rem] text-muted-foreground">{t("최대 포지션:", "Max Position:")}</span>
                  <span className="text-[0.75rem]">${agent.risk_profile.max_position_usdc.toLocaleString()}</span>
                </div>
                <div className="flex items-center gap-2">
                  <Shield className="w-3.5 h-3.5 text-muted-foreground" />
                  <span className="text-[0.75rem] text-muted-foreground">{t("일일 손실 한도:", "Daily Loss Limit:")}</span>
                  <span className="text-[0.75rem]">${agent.risk_profile.daily_loss_limit_usdc.toLocaleString()}</span>
                </div>
                <div className="flex items-center gap-2">
                  <Shield className="w-3.5 h-3.5 text-muted-foreground" />
                  <span className="text-[0.75rem] text-muted-foreground">Kill Switch:</span>
                  <span className={`text-[0.75rem] ${agent.risk_profile.kill_switch_enabled ? "text-[#22c55e]" : "text-muted-foreground"}`}>
                    {agent.risk_profile.kill_switch_enabled ? t("활성", "Enabled") : t("비활성", "Disabled")}
                  </span>
                </div>
              </div>
            </div>
          );
        })}
      </div>

      {loading && (
        <p className="mt-6 text-[0.8125rem] text-muted-foreground">{t("세션 상태 로딩 중...", "Loading session states...")}</p>
      )}
    </div>
  );
}
