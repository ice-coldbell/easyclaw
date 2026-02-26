import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router";
import { ArrowUpRight } from "lucide-react";
import { AreaChart, Area, ResponsiveContainer } from "recharts";
import { AgentRecord, PortfolioSummary, createChannelSocket, filterAgentsByOwner, getAgents, getPortfolio } from "../lib/backend";
import { useI18n } from "../i18n";
import { useWallet } from "../lib/wallet";

type PortfolioPeriod = "7d" | "30d" | "all";

const PERIOD_OPTIONS: { value: PortfolioPeriod; labelKey: [string, string] }[] = [
  { value: "7d", labelKey: ["7일", "7D"] },
  { value: "30d", labelKey: ["30일", "30D"] },
  { value: "all", labelKey: ["전체", "All"] },
];

export function PortfolioPage() {
  const { t } = useI18n();
  const { walletPubkey } = useWallet();
  const [period, setPeriod] = useState<PortfolioPeriod>("all");
  const [portfolio, setPortfolio] = useState<PortfolioSummary | null>(null);
  const [agents, setAgents] = useState<AgentRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const ownedAgents = useMemo(
    () => filterAgentsByOwner(agents, walletPubkey),
    [agents, walletPubkey],
  );

  const ownedAgentIDSet = useMemo(
    () => new Set(ownedAgents.map((agent) => agent.id)),
    [ownedAgents],
  );

  const loadPortfolio = useCallback(async () => {
    if (!walletPubkey) {
      setPortfolio(null);
      setAgents([]);
      setLoading(false);
      setError(null);
      return;
    }

    try {
      setLoading(true);
      setError(null);
      const [portfolioResponse, agentsResponse] = await Promise.all([
        getPortfolio(period),
        getAgents(),
      ]);
      setPortfolio(portfolioResponse);
      setAgents(agentsResponse);
    } catch (loadError) {
      const message = loadError instanceof Error ? loadError.message : String(loadError);
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [period, walletPubkey]);

  useEffect(() => {
    void loadPortfolio();
  }, [loadPortfolio]);

  useEffect(() => {
    if (!portfolio) {
      return undefined;
    }

    return createChannelSocket({
      channels: ["portfolio.updates"],
      onMessage: () => {
        void loadPortfolio();
      },
    });
  }, [loadPortfolio, portfolio]);

  const agentsById = useMemo(() => {
    const out = new Map<string, AgentRecord>();
    for (const agent of ownedAgents) {
      out.set(agent.id, agent);
    }
    return out;
  }, [ownedAgents]);

  const equityData = useMemo(() => {
    if (!walletPubkey || !portfolio || portfolio.equity_history.length === 0) {
      return [];
    }

    return portfolio.equity_history.map((point, index) => ({
      day: index + 1,
      value: point.value,
    }));
  }, [portfolio, walletPubkey]);

  const agentItems = useMemo(
    () => (portfolio?.agents ?? []).filter((agent) => ownedAgentIDSet.has(agent.id)),
    [ownedAgentIDSet, portfolio],
  );

  const totalEquity = useMemo(
    () => agentItems.reduce((sum, agent) => sum + agent.equity, 0),
    [agentItems],
  );

  const totalPnl = useMemo(
    () => agentItems.reduce((sum, agent) => sum + agent.pnl_abs, 0),
    [agentItems],
  );

  const totalPnlPct = useMemo(() => {
    const baseEquity = totalEquity - totalPnl;
    if (baseEquity === 0) {
      return 0;
    }
    return (totalPnl / baseEquity) * 100;
  }, [totalEquity, totalPnl]);

  const activeAgents = useMemo(
    () => ownedAgents.filter((agent) => agent.status !== "stopped").length,
    [ownedAgents],
  );

  const maxDrawdown = useMemo(() => {
    if (agentItems.length === 0) {
      return 0;
    }
    return Math.min(...agentItems.map((agent) => agent.drawdown));
  }, [agentItems]);

  return (
    <div className="p-6 max-w-6xl mx-auto">
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-[1.5rem]" style={{ fontWeight: 600 }}>{t("포트폴리오", "Portfolio")}</h1>
          <p className="text-[0.875rem] text-muted-foreground mt-1">{t("계정 및 에이전트 성과 종합", "Combined account and agent performance")}</p>
        </div>
        <div className="flex gap-1 bg-secondary rounded-lg p-0.5">
          {PERIOD_OPTIONS.map((p) => (
            <button
              key={p.value}
              onClick={() => setPeriod(p.value)}
              className={`px-3 py-1.5 rounded-md text-[0.75rem] transition-colors ${
                period === p.value ? "bg-card text-foreground" : "text-muted-foreground hover:text-foreground"
              }`}
            >
              {t(...p.labelKey)}
            </button>
          ))}
        </div>
      </div>

      {!walletPubkey && (
        <div className="mb-6 bg-card border border-border rounded-lg px-4 py-3 text-[0.8125rem] text-muted-foreground">
          {t("우측 상단에서 월렛을 연결하면 내 에이전트 포트폴리오만 표시됩니다.", "Connect your wallet from the top-right to show only your agent portfolio.")}
        </div>
      )}

      {walletPubkey && ownedAgents.length === 0 && (
        <div className="mb-6 bg-card border border-border rounded-lg px-4 py-3 text-[0.8125rem] text-muted-foreground">
          {t("연결된 월렛 소유의 에이전트가 없습니다.", "No agents are owned by the connected wallet.")}
        </div>
      )}

      {error && (
        <div className="mb-6 bg-destructive/10 border border-destructive/20 text-destructive rounded-lg px-4 py-3 text-[0.8125rem]">
          {t("포트폴리오를 불러오지 못했습니다:", "Failed to load portfolio:")} {error}
        </div>
      )}

      {/* Summary Cards */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-8">
        <div className="bg-card border border-border rounded-xl p-5">
          <p className="text-[0.75rem] text-muted-foreground mb-1">{t("총 자산", "Total Equity")}</p>
          <p className="text-[1.5rem]" style={{ fontWeight: 700 }}>${totalEquity.toLocaleString()}</p>
        </div>
        <div className="bg-card border border-border rounded-xl p-5">
          <p className="text-[0.75rem] text-muted-foreground mb-1">{t("총 PnL", "Total PnL")}</p>
          <p className={`text-[1.5rem] ${totalPnl >= 0 ? "text-[#22c55e]" : "text-[#ef4444]"}`} style={{ fontWeight: 700 }}>
            {totalPnl >= 0 ? "+" : ""}${totalPnl.toLocaleString()}
          </p>
          <span className={`text-[0.75rem] ${totalPnl >= 0 ? "text-[#22c55e]" : "text-[#ef4444]"}`}>
            {totalPnl >= 0 ? "+" : ""}{totalPnlPct.toFixed(2)}%
          </span>
        </div>
        <div className="bg-card border border-border rounded-xl p-5">
          <p className="text-[0.75rem] text-muted-foreground mb-1">{t("활성 에이전트", "Active Agents")}</p>
          <p className="text-[1.5rem]" style={{ fontWeight: 700 }}>{activeAgents}</p>
          <span className="text-[0.75rem] text-muted-foreground">/ {ownedAgents.length} {t("전체", "total")}</span>
        </div>
        <div className="bg-card border border-border rounded-xl p-5">
          <p className="text-[0.75rem] text-muted-foreground mb-1">{t("최대 드로다운", "Max Drawdown")}</p>
          <p className="text-[1.5rem] text-[#ef4444]" style={{ fontWeight: 700 }}>{maxDrawdown.toFixed(2)}%</p>
        </div>
      </div>

      {/* Equity Chart */}
      <div className="bg-card border border-border rounded-xl p-5 mb-8">
        <h3 className="text-[0.9375rem] mb-4" style={{ fontWeight: 600 }}>{t("자산 추이", "Equity Trend")}</h3>
        <ResponsiveContainer width="100%" height={200}>
          <AreaChart data={equityData} margin={{ top: 5, right: 5, left: 5, bottom: 0 }}>
            <defs>
              <linearGradient id="eqGradient" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="#00d4aa" stopOpacity={0.2} />
                <stop offset="100%" stopColor="#00d4aa" stopOpacity={0} />
              </linearGradient>
            </defs>
            <Area type="monotone" dataKey="value" stroke="#00d4aa" strokeWidth={1.5} fill="url(#eqGradient)" />
          </AreaChart>
        </ResponsiveContainer>
      </div>

      {/* Agent Cards */}
      <h3 className="text-[0.9375rem] mb-4" style={{ fontWeight: 600 }}>{t("에이전트별 성과", "Performance by Agent")}</h3>
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {agentItems.map((agent) => {
          const metadata = agentsById.get(agent.id);
          const status = metadata?.status ?? "stopped";
          const sparkData = Array.from({ length: 14 }, (_, i) => ({
            d: i,
            v: agent.pnl_abs > 0 ? i * (agent.pnl_abs / 14) + Math.random() * 120 : agent.pnl_abs + i * 20 + Math.random() * 80,
          }));

          return (
            <Link
              key={agent.id}
              to={`/app/agents/${agent.id}`}
              className="bg-card border border-border rounded-xl p-5 hover:border-primary/30 transition-colors group"
            >
              <div className="flex items-center justify-between mb-3">
                <div>
                  <h4 className="text-[0.875rem]" style={{ fontWeight: 600 }}>{agent.name}</h4>
                  <p className="text-[0.6875rem] text-muted-foreground">{metadata?.strategy_id ?? "-"}</p>
                </div>
                <span className={`text-[0.6875rem] px-2 py-0.5 rounded-full ${
                  status === "live" ? "bg-[#22c55e]/10 text-[#22c55e]" :
                  status === "paper" ? "bg-[#3b82f6]/10 text-[#3b82f6]" :
                  "bg-muted text-muted-foreground"
                }`}>
                  {status.toUpperCase()}
                </span>
              </div>

              <div className="grid grid-cols-2 gap-3 mb-3">
                <div>
                  <p className="text-[0.6875rem] text-muted-foreground">PnL</p>
                  <p className={`text-[0.9375rem] ${agent.pnl_abs >= 0 ? "text-[#22c55e]" : "text-[#ef4444]"}`} style={{ fontWeight: 600 }}>
                    {agent.pnl_abs >= 0 ? "+" : ""}${agent.pnl_abs.toLocaleString()}
                  </p>
                </div>
                <div>
                  <p className="text-[0.6875rem] text-muted-foreground">{t("승률", "Win Rate")}</p>
                  <p className="text-[0.9375rem]" style={{ fontWeight: 600 }}>{agent.win_rate.toFixed(2)}%</p>
                </div>
              </div>

              <ResponsiveContainer width="100%" height={50}>
                <AreaChart data={sparkData} margin={{ top: 2, right: 0, left: 0, bottom: 0 }}>
                  <Area
                    type="monotone"
                    dataKey="v"
                    stroke={agent.pnl_abs >= 0 ? "#22c55e" : "#ef4444"}
                    strokeWidth={1}
                    fill={agent.pnl_abs >= 0 ? "#22c55e" : "#ef4444"}
                    fillOpacity={0.05}
                  />
                </AreaChart>
              </ResponsiveContainer>

              <div className="flex items-center justify-between mt-3 pt-3 border-t border-border text-[0.6875rem] text-muted-foreground">
                <span>{t("드로다운", "Drawdown")}: {agent.drawdown.toFixed(2)}%</span>
                <span className="flex items-center gap-1 text-primary opacity-0 group-hover:opacity-100 transition-opacity">
                  {t("상세 보기", "View Details")} <ArrowUpRight className="w-3 h-3" />
                </span>
              </div>
            </Link>
          );
        })}
      </div>

      {loading && (
        <p className="mt-6 text-[0.8125rem] text-muted-foreground">{t("포트폴리오 로딩 중...", "Loading portfolio...")}</p>
      )}
    </div>
  );
}
