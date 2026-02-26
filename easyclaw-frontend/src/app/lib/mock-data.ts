// Mock BTC price data for charts
export function generateCandleData(count: number = 100, locale?: string) {
  const data = [];
  let price = 67500;
  const now = Date.now();
  for (let i = count; i >= 0; i--) {
    const open = price;
    const change = (Math.random() - 0.48) * 400;
    const close = open + change;
    const high = Math.max(open, close) + Math.random() * 200;
    const low = Math.min(open, close) - Math.random() * 200;
    price = close;
    data.push({
      // Keep timestamps locale-aware so chart labels follow the selected app language.
      time: new Date(now - i * 3600000).toLocaleTimeString(locale, { hour: "2-digit", minute: "2-digit" }),
      timestamp: now - i * 3600000,
      open: Math.round(open),
      close: Math.round(close),
      high: Math.round(high),
      low: Math.round(low),
      volume: Math.round(Math.random() * 500 + 100),
    });
  }
  return data;
}

export const mockAgents = [
  { id: "agent-001", name: "Alpha Momentum", strategy_type: "모멘텀 추세추종", status: "live" as const, created_at: "2025-12-15", pnl_pct: 12.4, pnl_abs: 3240, win_rate: 67.2, total_trades: 156, drawdown: -4.2, equity: 28500 },
  { id: "agent-002", name: "Beta Reversal", strategy_type: "평균회귀", status: "paper" as const, created_at: "2026-01-08", pnl_pct: 8.7, pnl_abs: 1850, win_rate: 58.9, total_trades: 89, drawdown: -6.1, equity: 21200 },
  { id: "agent-003", name: "Gamma Breakout", strategy_type: "볼린저 브레이크아웃", status: "stopped" as const, created_at: "2026-02-01", pnl_pct: -2.3, pnl_abs: -460, win_rate: 42.1, total_trades: 34, drawdown: -8.5, equity: 15000 },
];

export const mockLeaderboard = [
  { rank: 1, agent_id: "lb-001", agent_name: "Thunder Strike", win_rate: 78.4, pnl_pct: 45.2, total_trades: 234, max_drawdown: -3.1, rank_change: 0, sparkline: [10, 12, 11, 15, 18, 22, 20, 25, 28, 30] },
  { rank: 2, agent_id: "lb-002", agent_name: "Solana Surfer", win_rate: 74.1, pnl_pct: 38.7, total_trades: 187, max_drawdown: -4.5, rank_change: 2, sparkline: [5, 8, 7, 12, 10, 15, 18, 20, 22, 25] },
  { rank: 3, agent_id: "lb-003", agent_name: "Night Owl", win_rate: 71.8, pnl_pct: 31.4, total_trades: 312, max_drawdown: -5.2, rank_change: -1, sparkline: [8, 10, 9, 11, 14, 13, 16, 18, 17, 20] },
  { rank: 4, agent_id: "lb-004", agent_name: "DCA Machine", win_rate: 69.5, pnl_pct: 28.9, total_trades: 456, max_drawdown: -3.8, rank_change: 1, sparkline: [3, 5, 7, 6, 9, 11, 10, 13, 15, 18] },
  { rank: 5, agent_id: "agent-001", agent_name: "Alpha Momentum", win_rate: 67.2, pnl_pct: 12.4, total_trades: 156, max_drawdown: -4.2, rank_change: -2, sparkline: [2, 4, 3, 6, 8, 7, 9, 10, 11, 12] },
  { rank: 6, agent_id: "lb-005", agent_name: "Whale Watch", win_rate: 65.3, pnl_pct: 22.1, total_trades: 98, max_drawdown: -6.7, rank_change: 0, sparkline: [1, 3, 5, 4, 7, 9, 8, 12, 14, 16] },
  { rank: 7, agent_id: "lb-006", agent_name: "Trend Hunter", win_rate: 63.8, pnl_pct: 19.4, total_trades: 267, max_drawdown: -5.9, rank_change: 3, sparkline: [4, 6, 5, 8, 7, 10, 12, 11, 14, 16] },
  { rank: 8, agent_id: "lb-007", agent_name: "Scalp King", win_rate: 61.2, pnl_pct: 15.8, total_trades: 1024, max_drawdown: -2.1, rank_change: -1, sparkline: [6, 7, 8, 7, 9, 10, 11, 12, 13, 14] },
  { rank: 9, agent_id: "lb-008", agent_name: "Moon Chaser", win_rate: 59.7, pnl_pct: 11.2, total_trades: 145, max_drawdown: -7.3, rank_change: 0, sparkline: [2, 3, 4, 5, 4, 6, 7, 8, 9, 10] },
  { rank: 10, agent_id: "lb-009", agent_name: "Risk Averse", win_rate: 58.1, pnl_pct: 9.8, total_trades: 78, max_drawdown: -1.5, rank_change: 1, sparkline: [1, 2, 3, 4, 5, 5, 6, 7, 8, 9] },
];

export const mockTrades = [
  { id: "t-001", agent_id: "agent-001", agent_name: "Alpha Momentum", side: "long" as const, entry_price: 67200, exit_price: 67850, qty: 0.15, fee: 2.45, pnl: 95.25, tx_sig: "5xKm...3fQp", entry_time: "2026-02-25 09:15", exit_time: "2026-02-25 11:42" },
  { id: "t-002", agent_id: "agent-001", agent_name: "Alpha Momentum", side: "short" as const, entry_price: 67900, exit_price: 67450, qty: 0.12, fee: 1.98, pnl: 52.02, tx_sig: "8rTn...7bWq", entry_time: "2026-02-25 12:30", exit_time: "2026-02-25 14:15" },
  { id: "t-003", agent_id: "agent-002", agent_name: "Beta Reversal", side: "long" as const, entry_price: 67100, exit_price: 66800, qty: 0.2, fee: 3.12, pnl: -63.12, tx_sig: "2mXp...9kLs", entry_time: "2026-02-24 16:45", exit_time: "2026-02-24 18:30" },
  { id: "t-004", agent_id: "agent-001", agent_name: "Alpha Momentum", side: "long" as const, entry_price: 66950, exit_price: 67400, qty: 0.18, fee: 2.78, pnl: 78.22, tx_sig: "4pQr...1nMs", entry_time: "2026-02-24 08:00", exit_time: "2026-02-24 10:45" },
  { id: "t-005", agent_id: "agent-002", agent_name: "Beta Reversal", side: "short" as const, entry_price: 67500, exit_price: 67100, qty: 0.1, fee: 1.55, pnl: 38.45, tx_sig: "7kLm...5vBn", entry_time: "2026-02-23 20:15", exit_time: "2026-02-23 22:00" },
  { id: "t-006", agent_id: "agent-003", agent_name: "Gamma Breakout", side: "long" as const, entry_price: 67800, exit_price: 67350, qty: 0.25, fee: 3.89, pnl: -116.39, tx_sig: "9wXy...3zAb", entry_time: "2026-02-23 14:30", exit_time: "2026-02-23 16:45" },
  { id: "t-007", agent_id: "agent-001", agent_name: "Alpha Momentum", side: "short" as const, entry_price: 68100, exit_price: 67750, qty: 0.14, fee: 2.19, pnl: 46.81, tx_sig: "1aBc...8dEf", entry_time: "2026-02-22 11:00", exit_time: "2026-02-22 13:30" },
  { id: "t-008", agent_id: "agent-002", agent_name: "Beta Reversal", side: "long" as const, entry_price: 66700, exit_price: 67200, qty: 0.16, fee: 2.45, pnl: 77.55, tx_sig: "6gHi...4jKl", entry_time: "2026-02-22 07:45", exit_time: "2026-02-22 09:15" },
];

export const mockSignals = [
  { agent_name: "Alpha Momentum", side: "long" as const, price: 67842, time: "09:15:32", confidence: 0.87 },
  { agent_name: "Beta Reversal", side: "short" as const, price: 67901, time: "09:12:18", confidence: 0.72 },
  { agent_name: "Thunder Strike", side: "long" as const, price: 67780, time: "09:08:45", confidence: 0.91 },
  { agent_name: "Night Owl", side: "short" as const, price: 67856, time: "09:05:12", confidence: 0.65 },
  { agent_name: "Scalp King", side: "long" as const, price: 67790, time: "09:02:33", confidence: 0.78 },
];

export const strategyPresets = [
  { id: "preset-1", name: "모멘텀 추세추종", description: "RSI + MACD 기반 추세 방향 진입, ATR 기반 손절", icon: "TrendingUp", risk: "중간" },
  { id: "preset-2", name: "평균회귀", description: "볼린저 밴드 이탈 후 회귀 진입, 밴드 중앙 청산", icon: "RefreshCw", risk: "낮음" },
  { id: "preset-3", name: "볼린저 브레이크아웃", description: "볼린저 밴드 돌파 진입, 추세 지속 시 추가 진입", icon: "Zap", risk: "높음" },
  { id: "preset-4", name: "커스텀 전략", description: "직접 진입/청산 규칙과 파라미터를 설정합니다", icon: "Settings", risk: "사용자 정의" },
];
