import { useEffect, useMemo, useState } from "react";
import { useParams, Link } from "react-router";
import { ArrowLeft, Bot, Calendar, Shield, TrendingUp } from "lucide-react";
import { AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer } from "recharts";
import { AgentPortfolioSummary, AgentRecord, TradeRecord, getAgent, getAgentPortfolio, getTrades } from "../lib/backend";
import { formatUnixDate, formatUnixTime } from "../lib/format";
import { useI18n } from "../i18n";
import { useWallet } from "../lib/wallet";

export function AgentDetailPage() {
  const { t, localeTag } = useI18n();
  const { walletPubkey } = useWallet();
  const { agentId } = useParams();
  const [agent, setAgent] = useState<AgentRecord | null>(null);
  const [portfolio, setPortfolio] = useState<AgentPortfolioSummary | null>(null);
  const [trades, setTrades] = useState<TradeRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!agentId) {
      setError("agent id is required");
      setLoading(false);
      return;
    }

    const controller = new AbortController();
    const load = async () => {
      try {
        setLoading(true);
        setError(null);
        const agentResponse = await getAgent(agentId, controller.signal);
        const [portfolioResponse, tradeResponse] = await Promise.all([
          getAgentPortfolio(agentId, "30d", controller.signal),
          getTrades({ agentId, limit: 100, offset: 0 }, controller.signal),
        ]);
        setAgent(agentResponse);
        setPortfolio(portfolioResponse);
        setTrades(tradeResponse.items);
      } catch (loadError) {
        const message = loadError instanceof Error ? loadError.message : String(loadError);
        setError(message);
      } finally {
        setLoading(false);
      }
    };

    void load();
    return () => controller.abort();
  }, [agentId]);

  const equityData = useMemo(() => {
    if (!portfolio) {
      return [];
    }
    if (portfolio.equity_history.length > 0) {
      return portfolio.equity_history.map((point) => ({
        day: formatUnixDate(point.ts, localeTag),
        equity: point.value,
      }));
    }
    return portfolio.daily_pnl.map((point) => ({
      day: point.date,
      equity: point.pnl,
    }));
  }, [localeTag, portfolio]);

  if (!agentId) {
    return null;
  }

  const canManageAgent = Boolean(agent && walletPubkey && agent.owner_pubkey === walletPubkey);
  const backPath = canManageAgent ? "/app/portfolio" : "/app/leaderboard";
  const backLabel = canManageAgent
    ? t("포트폴리오로 돌아가기", "Back to Portfolio")
    : t("리더보드로 돌아가기", "Back to Leaderboard");

  return (
    <div className="p-6 max-w-5xl mx-auto">
      <Link to={backPath} className="flex items-center gap-1.5 text-[0.8125rem] text-muted-foreground hover:text-foreground mb-6 transition-colors">
        <ArrowLeft className="w-3.5 h-3.5" /> {backLabel}
      </Link>

      {error && (
        <div className="mb-6 bg-destructive/10 border border-destructive/20 text-destructive rounded-lg px-4 py-3 text-[0.8125rem]">
          {t("에이전트 상세를 불러오지 못했습니다:", "Failed to load agent details:")} {error}
        </div>
      )}

      {agent && (
        <div className="bg-card border border-border rounded-xl p-6 mb-6">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-4">
              <div className={`w-12 h-12 rounded-xl flex items-center justify-center ${
                agent.status === "live" ? "bg-[#22c55e]/10" :
                  agent.status === "paper" ? "bg-[#3b82f6]/10" : "bg-muted"
              }`}>
                <Bot className={`w-6 h-6 ${
                  agent.status === "live" ? "text-[#22c55e]" :
                    agent.status === "paper" ? "text-[#3b82f6]" : "text-muted-foreground"
                }`} />
              </div>
              <div>
                <div className="flex items-center gap-3">
                  <h1 className="text-[1.25rem]" style={{ fontWeight: 600 }}>{agent.name}</h1>
                  <span className={`text-[0.6875rem] px-2.5 py-1 rounded-full ${
                    agent.status === "live" ? "bg-[#22c55e]/10 text-[#22c55e]" :
                      agent.status === "paper" ? "bg-[#3b82f6]/10 text-[#3b82f6]" :
                        "bg-muted text-muted-foreground"
                  }`}>
                    {agent.status.toUpperCase()}
                  </span>
                </div>
                <div className="flex items-center gap-4 text-[0.75rem] text-muted-foreground mt-1">
                  <span className="flex items-center gap-1"><TrendingUp className="w-3 h-3" />{agent.strategy_id}</span>
                  <span className="flex items-center gap-1"><Calendar className="w-3 h-3" />{formatUnixTime(agent.created_at, localeTag)}</span>
                </div>
              </div>
            </div>
            {canManageAgent && (
              <div className="flex gap-2">
                <Link to="/app/risk" className="text-[0.8125rem] text-muted-foreground hover:text-foreground border border-border px-3 py-2 rounded-lg flex items-center gap-1.5 transition-colors">
                  <Shield className="w-3.5 h-3.5" /> {t("리스크 설정", "Risk Settings")}
                </Link>
                <Link to="/app/sessions" className="text-[0.8125rem] text-primary border border-primary/30 px-3 py-2 rounded-lg hover:bg-primary/5 transition-colors">
                  {t("세션 제어", "Session Control")}
                </Link>
              </div>
            )}
          </div>

          <div className="grid grid-cols-5 gap-4 mt-6 pt-6 border-t border-border">
            {[
              { label: t("최대 포지션", "Max Position"), value: `$${agent.risk_profile.max_position_usdc.toLocaleString()}` },
              {
                label: "PnL",
                value: `${(portfolio?.pnl_abs ?? 0) >= 0 ? "+" : ""}$${(portfolio?.pnl_abs ?? 0).toLocaleString()}`,
                color: (portfolio?.pnl_abs ?? 0) >= 0 ? "#22c55e" : "#ef4444",
              },
              {
                label: t("수익률", "Return"),
                value: `${(portfolio?.pnl_pct ?? 0) >= 0 ? "+" : ""}${(portfolio?.pnl_pct ?? 0).toFixed(2)}%`,
                color: (portfolio?.pnl_pct ?? 0) >= 0 ? "#22c55e" : "#ef4444",
              },
              { label: t("승률", "Win Rate"), value: `${(portfolio?.win_rate ?? 0).toFixed(2)}%` },
              { label: t("드로다운", "Drawdown"), value: `${(portfolio?.drawdown ?? 0).toFixed(2)}%`, color: "#ef4444" },
            ].map((item) => (
              <div key={item.label}>
                <p className="text-[0.6875rem] text-muted-foreground mb-1">{item.label}</p>
                <p className="text-[1.125rem]" style={{ fontWeight: 600, color: item.color }}>{item.value}</p>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="bg-card border border-border rounded-xl p-5 mb-6">
        <h3 className="text-[0.9375rem] mb-4" style={{ fontWeight: 600 }}>{t("성과 추이 (30일)", "Performance Trend (30D)")}</h3>
        <ResponsiveContainer width="100%" height={220}>
          <AreaChart data={equityData} margin={{ top: 5, right: 10, left: 0, bottom: 0 }}>
            <defs>
              <linearGradient id="perfGrad" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="#00d4aa" stopOpacity={0.15} />
                <stop offset="100%" stopColor="#00d4aa" stopOpacity={0} />
              </linearGradient>
            </defs>
            <XAxis dataKey="day" stroke="#1f2937" tick={{ fill: "#6b7280", fontSize: 10 }} axisLine={false} tickLine={false} />
            <YAxis stroke="#1f2937" tick={{ fill: "#6b7280", fontSize: 10 }} axisLine={false} tickLine={false} tickFormatter={(value: number) => `$${(value / 1000).toFixed(1)}k`} width={50} />
            <Tooltip
              contentStyle={{ background: "#1a1b25", border: "1px solid rgba(255,255,255,0.1)", borderRadius: 8, fontSize: 11, color: "#e8eaed" }}
              formatter={(value: number) => [`$${value.toFixed(2)}`, t("자산", "Equity")]}
            />
            <Area type="monotone" dataKey="equity" stroke="#00d4aa" strokeWidth={1.5} fill="url(#perfGrad)" />
          </AreaChart>
        </ResponsiveContainer>
      </div>

      <div className="bg-card border border-border rounded-xl p-5">
        <h3 className="text-[0.9375rem] mb-4" style={{ fontWeight: 600 }}>{t("최근 시그널 / 체결", "Recent Signals / Fills")}</h3>
        <div className="overflow-x-auto">
          <table className="w-full text-[0.8125rem]">
            <thead>
              <tr className="text-[0.6875rem] text-muted-foreground border-b border-border">
                <th className="text-left py-2 pr-4">{t("방향", "Side")}</th>
                <th className="text-right py-2 pr-4">{t("진입가", "Entry")}</th>
                <th className="text-right py-2 pr-4">{t("청산가", "Exit")}</th>
                <th className="text-right py-2 pr-4">{t("수량", "Qty")}</th>
                <th className="text-right py-2 pr-4">PnL</th>
                <th className="text-right py-2">{t("시간", "Time")}</th>
              </tr>
            </thead>
            <tbody>
              {trades.slice(0, 50).map((trade) => (
                <tr key={trade.id} className="border-b border-border last:border-0">
                  <td className="py-3 pr-4">
                    <span className={`text-[0.75rem] px-2 py-0.5 rounded ${
                      trade.side === "long" ? "bg-[#22c55e]/10 text-[#22c55e]" : "bg-[#ef4444]/10 text-[#ef4444]"
                    }`}>
                      {trade.side.toUpperCase()}
                    </span>
                  </td>
                  <td className="text-right py-3 pr-4">${trade.entry_price.toLocaleString()}</td>
                  <td className="text-right py-3 pr-4">${trade.exit_price.toLocaleString()}</td>
                  <td className="text-right py-3 pr-4">{trade.qty}</td>
                  <td className={`text-right py-3 pr-4 ${trade.pnl >= 0 ? "text-[#22c55e]" : "text-[#ef4444]"}`}>
                    {trade.pnl >= 0 ? "+" : ""}${trade.pnl.toFixed(2)}
                  </td>
                  <td className="text-right py-3 text-muted-foreground text-[0.75rem]">{formatUnixTime(trade.exit_time, localeTag)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {loading && (
        <p className="mt-6 text-[0.8125rem] text-muted-foreground">{t("에이전트 상세 로딩 중...", "Loading agent details...")}</p>
      )}
    </div>
  );
}
