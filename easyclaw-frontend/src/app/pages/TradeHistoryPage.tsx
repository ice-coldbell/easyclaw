import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router";
import { ExternalLink, Search } from "lucide-react";
import { AgentRecord, TradeRecord, filterAgentsByOwner, getAgents, getTrades } from "../lib/backend";
import { formatUnixTime } from "../lib/format";
import { useI18n } from "../i18n";
import { useWallet } from "../lib/wallet";

export function TradeHistoryPage() {
  const { t, localeTag } = useI18n();
  const { walletPubkey } = useWallet();
  const [agentFilter, setAgentFilter] = useState("all");
  const [searchText, setSearchText] = useState("");
  const [fromDate, setFromDate] = useState("");
  const [toDate, setToDate] = useState("");
  const [agents, setAgents] = useState<AgentRecord[]>([]);
  const [trades, setTrades] = useState<TradeRecord[]>([]);
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

  useEffect(() => {
    setAgentFilter((current) => {
      if (current === "all" || ownedAgentIDSet.has(current)) {
        return current;
      }
      return "all";
    });
  }, [ownedAgentIDSet]);

  useEffect(() => {
    const controller = new AbortController();
    const loadAgents = async () => {
      try {
        const response = await getAgents(controller.signal);
        setAgents(response);
      } catch {
        // Keep the UI usable even when agent lookup fails.
      }
    };

    void loadAgents();
    return () => controller.abort();
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    const loadTrades = async () => {
      if (!walletPubkey || ownedAgents.length === 0) {
        setTrades([]);
        setLoading(false);
        return;
      }

      try {
        setLoading(true);
        setError(null);
        const fromUnix = fromDate ? Math.floor(new Date(`${fromDate}T00:00:00`).getTime() / 1000) : undefined;
        const toUnix = toDate ? Math.floor(new Date(`${toDate}T23:59:59`).getTime() / 1000) : undefined;
        const response = await getTrades(
          {
            agentId: agentFilter !== "all" ? agentFilter : undefined,
            from: fromUnix,
            to: toUnix,
            limit: 500,
            offset: 0,
          },
          controller.signal,
        );
        setTrades(response.items.filter((trade) => ownedAgentIDSet.has(trade.agent_id)));
      } catch (loadError) {
        const message = loadError instanceof Error ? loadError.message : String(loadError);
        setError(message);
      } finally {
        setLoading(false);
      }
    };

    void loadTrades();
    return () => controller.abort();
  }, [agentFilter, fromDate, ownedAgentIDSet, ownedAgents.length, toDate, walletPubkey]);

  const filtered = useMemo(
    () =>
      trades.filter((trade) => {
        if (searchText && !trade.tx_sig.toLowerCase().includes(searchText.toLowerCase())) {
          return false;
        }
        return true;
      }),
    [searchText, trades],
  );

  const totalPnl = filtered.reduce((sum, trade) => sum + trade.pnl, 0);
  const totalFee = filtered.reduce((sum, trade) => sum + trade.fee, 0);
  const winCount = filtered.filter((trade) => trade.pnl > 0).length;

  return (
    <div className="p-6 max-w-6xl mx-auto">
      <div className="mb-8">
        <h1 className="text-[1.5rem]" style={{ fontWeight: 600 }}>{t("거래 내역", "Trade History")}</h1>
        <p className="text-[0.875rem] text-muted-foreground mt-1">{t("Agent/기간별 거래 조회", "Search trades by agent and date range")}</p>
      </div>

      {/* Filters */}
      <div className="flex items-center gap-3 mb-6">
        <div className="flex items-center gap-2 bg-card border border-border rounded-lg px-3 py-2 flex-1 max-w-xs">
          <Search className="w-4 h-4 text-muted-foreground" />
          <input
            type="text"
            placeholder={t("TX 서명 검색...", "Search TX signature...")}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            className="bg-transparent text-[0.8125rem] placeholder:text-muted-foreground/50 focus:outline-none flex-1"
          />
        </div>
        <select
          value={agentFilter}
          onChange={(e) => setAgentFilter(e.target.value)}
          className="bg-card border border-border rounded-lg px-3 py-2 text-[0.8125rem] focus:outline-none focus:border-primary"
        >
          <option value="all">{t("내 에이전트 전체", "All My Agents")}</option>
          {ownedAgents.map((agent) => (
            <option key={agent.id} value={agent.id}>{agent.name}</option>
          ))}
        </select>
        <input
          type="date"
          value={fromDate}
          onChange={(event) => setFromDate(event.target.value)}
          className="bg-card border border-border rounded-lg px-3 py-2 text-[0.8125rem] focus:outline-none focus:border-primary"
        />
        <input
          type="date"
          value={toDate}
          onChange={(event) => setToDate(event.target.value)}
          className="bg-card border border-border rounded-lg px-3 py-2 text-[0.8125rem] focus:outline-none focus:border-primary"
        />
      </div>

      {!walletPubkey && (
        <div className="mb-6 bg-card border border-border rounded-lg px-4 py-3 text-[0.8125rem] text-muted-foreground">
          {t("우측 상단에서 월렛을 연결하면 내 거래 내역만 조회됩니다.", "Connect your wallet from the top-right to view only your trade history.")}
        </div>
      )}

      {walletPubkey && ownedAgents.length === 0 && (
        <div className="mb-6 bg-card border border-border rounded-lg px-4 py-3 text-[0.8125rem] text-muted-foreground">
          {t("연결된 월렛 소유의 에이전트가 없습니다.", "No agents are owned by the connected wallet.")}
        </div>
      )}

      {error && (
        <div className="mb-6 bg-destructive/10 border border-destructive/20 text-destructive rounded-lg px-4 py-3 text-[0.8125rem]">
          {t("거래 내역을 불러오지 못했습니다:", "Failed to load trades:")} {error}
        </div>
      )}

      {/* Summary */}
      <div className="grid grid-cols-4 gap-4 mb-6">
        <div className="bg-card border border-border rounded-lg p-4">
          <p className="text-[0.6875rem] text-muted-foreground">{t("총 거래", "Total Trades")}</p>
          <p className="text-[1.125rem]" style={{ fontWeight: 600 }}>{filtered.length}</p>
        </div>
        <div className="bg-card border border-border rounded-lg p-4">
          <p className="text-[0.6875rem] text-muted-foreground">{t("총 PnL", "Total PnL")}</p>
          <p className={`text-[1.125rem] ${totalPnl >= 0 ? "text-[#22c55e]" : "text-[#ef4444]"}`} style={{ fontWeight: 600 }}>
            {totalPnl >= 0 ? "+" : ""}${totalPnl.toFixed(2)}
          </p>
        </div>
        <div className="bg-card border border-border rounded-lg p-4">
          <p className="text-[0.6875rem] text-muted-foreground">{t("승률", "Win Rate")}</p>
          <p className="text-[1.125rem]" style={{ fontWeight: 600 }}>
            {filtered.length > 0 ? ((winCount / filtered.length) * 100).toFixed(1) : 0}%
          </p>
        </div>
        <div className="bg-card border border-border rounded-lg p-4">
          <p className="text-[0.6875rem] text-muted-foreground">{t("총 수수료", "Total Fees")}</p>
          <p className="text-[1.125rem] text-muted-foreground" style={{ fontWeight: 600 }}>${totalFee.toFixed(2)}</p>
        </div>
      </div>

      {/* Trade Table */}
      <div className="bg-card border border-border rounded-xl overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-[0.8125rem]">
            <thead>
              <tr className="bg-secondary/50 text-[0.6875rem] text-muted-foreground">
                <th className="text-left py-3 px-4">{t("에이전트", "Agent")}</th>
                <th className="text-left py-3 px-4">{t("방향", "Side")}</th>
                <th className="text-right py-3 px-4">{t("진입가", "Entry")}</th>
                <th className="text-right py-3 px-4">{t("청산가", "Exit")}</th>
                <th className="text-right py-3 px-4">{t("수량", "Qty")}</th>
                <th className="text-right py-3 px-4">{t("수수료", "Fee")}</th>
                <th className="text-right py-3 px-4">PnL</th>
                <th className="text-right py-3 px-4">TX</th>
                <th className="text-right py-3 px-4">{t("시간", "Time")}</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((t) => (
                <tr key={t.id} className="border-t border-border hover:bg-secondary/20 transition-colors">
                  <td className="py-3 px-4">
                    <Link to={`/app/agents/${t.agent_id}`} className="text-foreground hover:text-primary transition-colors">
                      {t.agent_name}
                    </Link>
                  </td>
                  <td className="py-3 px-4">
                    <span className={`text-[0.75rem] px-2 py-0.5 rounded ${
                      t.side === "long" ? "bg-[#22c55e]/10 text-[#22c55e]" : "bg-[#ef4444]/10 text-[#ef4444]"
                    }`}>
                      {t.side.toUpperCase()}
                    </span>
                  </td>
                  <td className="text-right py-3 px-4">${t.entry_price.toLocaleString()}</td>
                  <td className="text-right py-3 px-4">${t.exit_price.toLocaleString()}</td>
                  <td className="text-right py-3 px-4">{t.qty}</td>
                  <td className="text-right py-3 px-4 text-muted-foreground">${t.fee.toFixed(2)}</td>
                  <td className={`text-right py-3 px-4 ${t.pnl >= 0 ? "text-[#22c55e]" : "text-[#ef4444]"}`}>
                    {t.pnl >= 0 ? "+" : ""}${t.pnl.toFixed(2)}
                  </td>
                  <td className="text-right py-3 px-4">
                    <span className="text-muted-foreground text-[0.75rem] flex items-center justify-end gap-1" style={{ fontFamily: "'JetBrains Mono', monospace" }}>
                      {t.tx_sig.length > 14 ? `${t.tx_sig.slice(0, 8)}...${t.tx_sig.slice(-4)}` : t.tx_sig} <ExternalLink className="w-3 h-3" />
                    </span>
                  </td>
                  <td className="text-right py-3 px-4 text-muted-foreground text-[0.75rem]">{formatUnixTime(t.exit_time, localeTag)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {loading && (
        <p className="mt-4 text-[0.8125rem] text-muted-foreground">{t("거래 내역 로딩 중...", "Loading trades...")}</p>
      )}
    </div>
  );
}
