import { useCallback, useEffect, useMemo, useState } from "react";
import { AlertTriangle, Check, OctagonX, Save, Shield } from "lucide-react";
import { AgentRecord, filterAgentsByOwner, getAgentRisk, getAgents, patchAgentRisk } from "../lib/backend";
import { useI18n } from "../i18n";
import { useWallet } from "../lib/wallet";

export function RiskSettingsPage() {
  const { t } = useI18n();
  const { walletPubkey } = useWallet();
  const [agents, setAgents] = useState<AgentRecord[]>([]);
  const [selectedAgent, setSelectedAgent] = useState<string>("");
  const [maxPosition, setMaxPosition] = useState("0");
  const [dailyLoss, setDailyLoss] = useState("0");
  const [killSwitch, setKillSwitch] = useState(true);
  const [saved, setSaved] = useState(false);
  const [hasChanges, setHasChanges] = useState(false);
  const [loadingAgents, setLoadingAgents] = useState(true);
  const [loadingRisk, setLoadingRisk] = useState(false);
  const [savingRisk, setSavingRisk] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const ownedAgents = useMemo(
    () => filterAgentsByOwner(agents, walletPubkey),
    [agents, walletPubkey],
  );

  const selectedAgentName = useMemo(
    () => ownedAgents.find((agent) => agent.id === selectedAgent)?.name ?? "-",
    [ownedAgents, selectedAgent],
  );

  useEffect(() => {
    const controller = new AbortController();
    const loadAgents = async () => {
      try {
        setLoadingAgents(true);
        setError(null);
        const response = await getAgents(controller.signal);
        setAgents(response);
      } catch (loadError) {
        const message = loadError instanceof Error ? loadError.message : String(loadError);
        setError(message);
      } finally {
        setLoadingAgents(false);
      }
    };

    void loadAgents();
    return () => controller.abort();
  }, []);

  useEffect(() => {
    setSelectedAgent((current) => {
      if (ownedAgents.some((agent) => agent.id === current)) {
        return current;
      }
      return ownedAgents[0]?.id ?? "";
    });
  }, [ownedAgents]);

  const loadRisk = useCallback(async (agentID: string) => {
    if (!agentID) {
      return;
    }
    try {
      setLoadingRisk(true);
      setError(null);
      const risk = await getAgentRisk(agentID);
      setMaxPosition(String(risk.max_position_usdc));
      setDailyLoss(String(risk.daily_loss_limit_usdc));
      setKillSwitch(risk.kill_switch_enabled);
      setHasChanges(false);
    } catch (loadError) {
      const message = loadError instanceof Error ? loadError.message : String(loadError);
      setError(message);
    } finally {
      setLoadingRisk(false);
    }
  }, []);

  useEffect(() => {
    void loadRisk(selectedAgent);
  }, [loadRisk, selectedAgent]);

  const handleSave = useCallback(async () => {
    if (!selectedAgent) {
      return;
    }

    try {
      setSavingRisk(true);
      setError(null);
      const maxPositionUSDC = Number(maxPosition);
      const dailyLossUSDC = Number(dailyLoss);
      if (!Number.isFinite(maxPositionUSDC) || maxPositionUSDC < 0) {
        throw new Error("max_position_usdc must be a non-negative number");
      }
      if (!Number.isFinite(dailyLossUSDC) || dailyLossUSDC < 0) {
        throw new Error("daily_loss_limit_usdc must be a non-negative number");
      }

      await patchAgentRisk(selectedAgent, {
        max_position_usdc: maxPositionUSDC,
        daily_loss_limit_usdc: dailyLossUSDC,
        kill_switch_enabled: killSwitch,
      });

      setSaved(true);
      setHasChanges(false);
      window.setTimeout(() => setSaved(false), 2000);
    } catch (saveError) {
      const message = saveError instanceof Error ? saveError.message : String(saveError);
      setError(message);
    } finally {
      setSavingRisk(false);
    }
  }, [dailyLoss, killSwitch, maxPosition, selectedAgent]);

  const handleChange = (setter: (value: string) => void, value: string) => {
    setter(value);
    setHasChanges(true);
    setSaved(false);
  };

  return (
    <div className="p-6 max-w-3xl mx-auto">
      <div className="mb-8">
        <h1 className="text-[1.5rem]" style={{ fontWeight: 600 }}>{t("리스크 설정", "Risk Settings")}</h1>
        <p className="text-[0.875rem] text-muted-foreground mt-1">{t("Agent별 리스크 프로필 및 안전장치 관리", "Manage risk profiles and safeguards by agent")}</p>
      </div>

      {error && (
        <div className="mb-6 bg-destructive/10 border border-destructive/20 text-destructive rounded-lg px-4 py-3 text-[0.8125rem]">
          {t("리스크 설정을 불러오지 못했습니다:", "Failed to load risk settings:")} {error}
        </div>
      )}

      <div className="mb-6">
        <label className="text-[0.8125rem] text-muted-foreground mb-2 block">{t("에이전트 선택", "Select Agent")}</label>
        {!walletPubkey && (
          <p className="mb-2 text-[0.75rem] text-muted-foreground">
            {t("우측 상단에서 월렛을 연결하면 내 에이전트를 불러옵니다.", "Connect your wallet from the top-right to load your agents.")}
          </p>
        )}
        {walletPubkey && ownedAgents.length === 0 && (
          <p className="mb-2 text-[0.75rem] text-muted-foreground">
            {t("연결된 월렛 소유의 에이전트가 없습니다.", "No agents are owned by the connected wallet.")}
          </p>
        )}
        <div className="flex gap-2 flex-wrap">
          {ownedAgents.map((agent) => (
            <button
              key={agent.id}
              onClick={() => setSelectedAgent(agent.id)}
              className={`px-4 py-2 rounded-lg text-[0.8125rem] border transition-colors ${
                selectedAgent === agent.id
                  ? "border-primary bg-primary/5 text-primary"
                  : "border-border text-muted-foreground hover:text-foreground"
              }`}
            >
              {agent.name}
            </button>
          ))}
        </div>
      </div>

      <div className="space-y-6">
        <div className="bg-card border border-border rounded-xl p-6">
          <div className="flex items-center gap-2 mb-6">
            <Shield className="w-5 h-5 text-primary" />
            <h3 className="text-[1rem]" style={{ fontWeight: 600 }}>{t("포지션 관리", "Position Controls")}</h3>
          </div>

          <div className="space-y-5">
            <div>
              <label className="text-[0.8125rem] text-muted-foreground mb-2 block">{t("최대 포지션 (USD)", "Max Position (USD)")}</label>
              <div className="flex items-center gap-2">
                <span className="text-muted-foreground text-[0.875rem]">$</span>
                <input
                  type="number"
                  min={0}
                  value={maxPosition}
                  onChange={(event) => handleChange(setMaxPosition, event.target.value)}
                  className="bg-input-background border border-border rounded-lg px-4 py-2.5 text-[0.875rem] focus:border-primary focus:outline-none flex-1"
                />
              </div>
              <p className="text-[0.6875rem] text-muted-foreground mt-1.5">{t("이 금액을 초과하는 신규 주문은 차단됩니다.", "New orders above this amount are blocked.")}</p>
            </div>

            <div>
              <label className="text-[0.8125rem] text-muted-foreground mb-2 block">{t("일일 손실 한도 (USD)", "Daily Loss Limit (USD)")}</label>
              <div className="flex items-center gap-2">
                <span className="text-muted-foreground text-[0.875rem]">$</span>
                <input
                  type="number"
                  min={0}
                  value={dailyLoss}
                  onChange={(event) => handleChange(setDailyLoss, event.target.value)}
                  className="bg-input-background border border-border rounded-lg px-4 py-2.5 text-[0.875rem] focus:border-primary focus:outline-none flex-1"
                />
              </div>
              <p className="text-[0.6875rem] text-muted-foreground mt-1.5">{t("일일 손실이 이 금액에 도달하면 신규 진입이 차단됩니다.", "New entries are blocked once daily loss reaches this threshold.")}</p>
            </div>
          </div>
        </div>

        <div className="bg-card border border-border rounded-xl p-6">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <OctagonX className="w-5 h-5 text-destructive" />
              <div>
                <h3 className="text-[1rem]" style={{ fontWeight: 600 }}>Kill Switch</h3>
                <p className="text-[0.75rem] text-muted-foreground mt-0.5">{t("활성화하면 즉시 모든 세션이 정지됩니다.", "When enabled, all sessions are stopped immediately.")}</p>
              </div>
            </div>
            <button
              onClick={() => { setKillSwitch((prev) => !prev); setHasChanges(true); setSaved(false); }}
              className={`relative w-12 h-6 rounded-full transition-colors ${
                killSwitch ? "bg-primary" : "bg-switch-background"
              }`}
            >
              <div className={`absolute top-0.5 w-5 h-5 rounded-full bg-white transition-transform ${
                killSwitch ? "left-[calc(100%-22px)]" : "left-0.5"
              }`} />
            </button>
          </div>
        </div>

        {hasChanges && (
          <div className="bg-[#f59e0b]/5 border border-[#f59e0b]/20 rounded-xl p-4">
            <div className="flex items-center gap-2 mb-2">
              <AlertTriangle className="w-4 h-4 text-[#f59e0b]" />
              <span className="text-[0.8125rem] text-[#f59e0b]" style={{ fontWeight: 600 }}>{t("변경사항 미저장", "Unsaved Changes")}</span>
            </div>
            <p className="text-[0.75rem] text-muted-foreground">{t("저장하기 전에 변경점을 확인하세요.", "Review your changes before saving.")}</p>
          </div>
        )}

        <div className="flex items-center gap-3">
          <button
            onClick={() => void handleSave()}
            disabled={!hasChanges || savingRisk || !selectedAgent}
            className="bg-primary text-primary-foreground px-6 py-2.5 rounded-lg text-[0.875rem] hover:bg-primary/90 transition-colors disabled:opacity-30 disabled:cursor-not-allowed flex items-center gap-2"
          >
            {saved ? (
              <><Check className="w-4 h-4" /> {t("저장 완료", "Saved")}</>
            ) : (
              <><Save className="w-4 h-4" /> {savingRisk ? t("저장 중...", "Saving...") : t("리스크 설정 저장", "Save Risk Settings")}</>
            )}
          </button>
          <span className="text-[0.75rem] text-muted-foreground">
            {t("현재 에이전트:", "Current Agent:")} {selectedAgentName}
          </span>
        </div>
      </div>

      {(loadingAgents || loadingRisk) && (
        <p className="mt-6 text-[0.8125rem] text-muted-foreground">
          {loadingAgents ? t("에이전트 목록 로딩 중...", "Loading agents...") : t("리스크 프로필 로딩 중...", "Loading risk profile...")}
        </p>
      )}
    </div>
  );
}
