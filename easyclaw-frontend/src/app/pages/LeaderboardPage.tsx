import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router";
import { ArrowDown, ArrowUp, Crown, Minus, Star, TrendingUp, Trophy } from "lucide-react";
import { LineChart, Line, ResponsiveContainer } from "recharts";
import { LeaderboardItem, createChannelSocket, filterAgentsByOwner, getAgents, getLeaderboard } from "../lib/backend";
import { useI18n } from "../i18n";
import { useWallet } from "../lib/wallet";

type LeaderboardMetric = "win_rate" | "pnl_pct";
type LeaderboardPeriod = "all_time" | "30d" | "7d";

interface RankChangeItem {
  agent_id: string;
  old_rank: number;
  new_rank: number;
}

export function LeaderboardPage() {
  const { t } = useI18n();
  const { walletPubkey } = useWallet();
  const [metric, setMetric] = useState<LeaderboardMetric>("win_rate");
  const [period, setPeriod] = useState<LeaderboardPeriod>("all_time");
  const [minTrades, setMinTrades] = useState(20);
  const [items, setItems] = useState<LeaderboardItem[]>([]);
  const [ownedAgentIDs, setOwnedAgentIDs] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [recentUpdates, setRecentUpdates] = useState<{ text: string; time: string }[]>([]);

  const loadLeaderboard = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const [response, ownedIDs] = await Promise.all([
        getLeaderboard({ metric, period, minTrades }),
        walletPubkey
          ? getAgents().then((allAgents) => filterAgentsByOwner(allAgents, walletPubkey).map((agent) => agent.id))
          : Promise.resolve([] as string[]),
      ]);
      setItems(response);
      setOwnedAgentIDs(ownedIDs);
    } catch (loadError) {
      const message = loadError instanceof Error ? loadError.message : String(loadError);
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [metric, minTrades, period, walletPubkey]);

  useEffect(() => {
    void loadLeaderboard();
  }, [loadLeaderboard]);

  useEffect(() => createChannelSocket({
    channels: ["leaderboard.updates"],
    onMessage: (message) => {
      if (message.channel !== "leaderboard.updates" || !message.data || typeof message.data !== "object") {
        return;
      }
      const payload = message.data as { rank_changes?: RankChangeItem[] };
      if (Array.isArray(payload.rank_changes) && payload.rank_changes.length > 0) {
        const updates = payload.rank_changes.slice(0, 4).map((rankChange) => ({
          text: t(
            `에이전트 ${rankChange.agent_id} 순위 ${rankChange.old_rank} → ${rankChange.new_rank}`,
            `Agent ${rankChange.agent_id} rank ${rankChange.old_rank} -> ${rankChange.new_rank}`,
          ),
          time: t("방금 전", "Just now"),
        }));
        setRecentUpdates(updates);
      }
      void loadLeaderboard();
    },
  }), [loadLeaderboard, t]);

  const topAgent = useMemo(() => items[0], [items]);

  return (
    <div className="p-6">
      <div className="max-w-6xl mx-auto">
        <div className="bg-gradient-to-r from-primary/10 via-card to-card border border-primary/20 rounded-xl p-6 mb-8">
          <div className="flex items-center justify-between">
            <div>
              <div className="flex items-center gap-2 mb-2">
                <Trophy className="w-5 h-5 text-primary" />
                <h1 className="text-[1.5rem]" style={{ fontWeight: 600 }}>{t("에이전트 리더보드", "Agent Leaderboard")}</h1>
              </div>
              <p className="text-[0.875rem] text-muted-foreground">
                {t(
                  `${period.toUpperCase()} · 최소 ${minTrades}건 체결 · ${metric}`,
                  `${period.toUpperCase()} · minimum ${minTrades} fills · ${metric}`,
                )}
              </p>
            </div>
            {topAgent && (
              <div className="text-right">
                <div className="flex items-center gap-2 mb-1">
                  <Crown className="w-4 h-4 text-[#f59e0b]" />
                  <span className="text-[0.8125rem] text-muted-foreground">{t("현재 1위", "Current #1")}</span>
                </div>
                <p className="text-[1.125rem]" style={{ fontWeight: 600 }}>{topAgent.agent_name}</p>
                <p className="text-[0.875rem] text-[#22c55e]">
                  {topAgent.pnl_pct >= 0 ? "+" : ""}{topAgent.pnl_pct.toFixed(2)}% · {topAgent.total_trades} {t("거래", "trades")}
                </p>
              </div>
            )}
          </div>
        </div>

        {error && (
          <div className="mb-6 bg-destructive/10 border border-destructive/20 text-destructive rounded-lg px-4 py-3 text-[0.8125rem]">
            {t("리더보드를 불러오지 못했습니다:", "Failed to load leaderboard:")} {error}
          </div>
        )}

        <div className="flex gap-6">
          <div className="flex-1">
            <div className="flex items-center justify-between mb-4 gap-4 flex-wrap">
              <span className="text-[0.8125rem] text-muted-foreground">
                {t(
                  "정렬: 선택 metric 기준 · 동률: pnl_pct → max_drawdown",
                  "Sort: selected metric · tie-breakers: pnl_pct -> max_drawdown",
                )}
              </span>
              <div className="flex items-center gap-2">
                <select
                  value={metric}
                  onChange={(event) => setMetric(event.target.value as LeaderboardMetric)}
                  className="bg-card border border-border rounded px-2 py-1 text-[0.75rem] focus:outline-none"
                >
                  <option value="win_rate">win_rate</option>
                  <option value="pnl_pct">pnl_pct</option>
                </select>
                <select
                  value={period}
                  onChange={(event) => setPeriod(event.target.value as LeaderboardPeriod)}
                  className="bg-card border border-border rounded px-2 py-1 text-[0.75rem] focus:outline-none"
                >
                  <option value="all_time">all_time</option>
                  <option value="30d">30d</option>
                  <option value="7d">7d</option>
                </select>
                <span className="text-[0.75rem] text-muted-foreground">{t("최소 체결:", "Min fills:")}</span>
                <select
                  value={minTrades}
                  onChange={(event) => setMinTrades(Number(event.target.value))}
                  className="bg-card border border-border rounded px-2 py-1 text-[0.75rem] focus:outline-none"
                >
                  <option value={10}>{t("10건", "10")}</option>
                  <option value={20}>{t("20건", "20")}</option>
                  <option value={50}>{t("50건", "50")}</option>
                  <option value={100}>{t("100건", "100")}</option>
                </select>
              </div>
            </div>

            <div className="bg-card border border-border rounded-xl overflow-hidden">
              <table className="w-full text-[0.8125rem]">
                <thead>
                  <tr className="bg-secondary/50 text-[0.6875rem] text-muted-foreground">
                    <th className="text-left py-3 px-4 w-16">{t("순위", "Rank")}</th>
                    <th className="text-left py-3 px-4">{t("에이전트", "Agent")}</th>
                    <th className="text-right py-3 px-4">{t("승률", "Win Rate")}</th>
                    <th className="text-right py-3 px-4">PnL %</th>
                    <th className="text-right py-3 px-4">{t("거래수", "Trades")}</th>
                    <th className="text-right py-3 px-4">{t("드로다운", "Drawdown")}</th>
                    <th className="text-right py-3 px-4 w-24">{t("추이", "Trend")}</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((entry) => {
                    const isMe = ownedAgentIDs.includes(entry.agent_id);
                    const sparkData = entry.sparkline.map((value, index) => ({ i: index, v: value }));
                    return (
                      <tr
                        key={entry.agent_id}
                        className={`border-t border-border transition-colors ${
                          isMe ? "bg-primary/5 hover:bg-primary/10" : "hover:bg-secondary/20"
                        }`}
                      >
                        <td className="py-3 px-4">
                          <div className="flex items-center gap-1.5">
                            {entry.rank <= 3 ? (
                              <span className={`text-[0.875rem] ${
                                entry.rank === 1 ? "text-[#f59e0b]" :
                                  entry.rank === 2 ? "text-[#94a3b8]" : "text-[#cd7f32]"
                              }`} style={{ fontWeight: 700 }}>
                                {entry.rank}
                              </span>
                            ) : (
                              <span className="text-muted-foreground">{entry.rank}</span>
                            )}
                            {entry.rank_change > 0 && <ArrowUp className="w-3 h-3 text-[#22c55e]" />}
                            {entry.rank_change < 0 && <ArrowDown className="w-3 h-3 text-[#ef4444]" />}
                            {entry.rank_change === 0 && <Minus className="w-3 h-3 text-muted-foreground/30" />}
                          </div>
                        </td>
                        <td className="py-3 px-4">
                          <Link to={`/app/agents/${entry.agent_id}`} className="flex items-center gap-2 hover:text-primary transition-colors">
                            {isMe && <Star className="w-3 h-3 text-primary" />}
                            <span style={{ fontWeight: isMe ? 600 : 400 }}>{entry.agent_name}</span>
                            {isMe && <span className="text-[0.6875rem] text-primary bg-primary/10 px-1.5 py-0.5 rounded">{t("내 에이전트", "My Agent")}</span>}
                          </Link>
                        </td>
                        <td className="text-right py-3 px-4" style={{ fontWeight: 600 }}>{entry.win_rate.toFixed(2)}%</td>
                        <td className={`text-right py-3 px-4 ${entry.pnl_pct >= 0 ? "text-[#22c55e]" : "text-[#ef4444]"}`}>
                          {entry.pnl_pct >= 0 ? "+" : ""}{entry.pnl_pct.toFixed(2)}%
                        </td>
                        <td className="text-right py-3 px-4 text-muted-foreground">{entry.total_trades}</td>
                        <td className="text-right py-3 px-4 text-[#ef4444]">{entry.max_drawdown.toFixed(2)}%</td>
                        <td className="py-3 px-4">
                          <ResponsiveContainer width={80} height={28}>
                            <LineChart data={sparkData}>
                              <Line type="monotone" dataKey="v" stroke="#00d4aa" strokeWidth={1} dot={false} />
                            </LineChart>
                          </ResponsiveContainer>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>

          <div className="w-72 flex-shrink-0">
            <div className="bg-card border border-border rounded-xl p-4 sticky top-6">
              <h3 className="text-[0.8125rem] mb-4" style={{ fontWeight: 600 }}>{t("최근 순위 변동", "Recent Ranking Changes")}</h3>
              <div className="space-y-3">
                {recentUpdates.length === 0 && (
                  <p className="text-[0.75rem] text-muted-foreground">{t("실시간 변동 대기 중...", "Waiting for live updates...")}</p>
                )}
                {recentUpdates.map((update, index) => (
                  <div key={`${update.text}-${index}`} className="flex gap-2 pb-3 border-b border-border last:border-0 last:pb-0">
                    <div className="w-1.5 h-1.5 rounded-full bg-primary mt-2 flex-shrink-0" />
                    <div>
                      <p className="text-[0.75rem]">{update.text}</p>
                      <p className="text-[0.6875rem] text-muted-foreground mt-0.5">{update.time}</p>
                    </div>
                  </div>
                ))}
              </div>
              <div className="mt-4 pt-4 border-t border-border">
                <Link to="/app/strategy-builder" className="text-[0.75rem] text-primary hover:underline flex items-center gap-1">
                  <TrendingUp className="w-3 h-3" /> {t("전략 수정하여 순위 올리기", "Improve your rank by updating strategy")}
                </Link>
              </div>
            </div>
          </div>
        </div>

        {loading && (
          <p className="mt-4 text-[0.8125rem] text-muted-foreground">{t("리더보드 로딩 중...", "Loading leaderboard...")}</p>
        )}
      </div>
    </div>
  );
}
