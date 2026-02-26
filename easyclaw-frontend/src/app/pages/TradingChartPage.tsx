import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router";
import { Activity, ArrowUpRight, Bot, Eye, Wifi } from "lucide-react";
import { AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer, ReferenceDot, BarChart, Bar } from "recharts";
import {
  CandleRecord,
  ChartTimeframe,
  MarketPricePayload,
  SystemStatus,
  TradingSignal,
  createChannelSocket,
  getChartCandles,
} from "../lib/backend";
import { formatUnixClock } from "../lib/format";
import { useI18n } from "../i18n";

const TIMEFRAMES = ["1M", "5M", "15M", "1H", "4H", "1D"] as const;
const MARKET_CODE = "BTCUSDT";
const MAX_CHART_CANDLES = 240;

const TIMEFRAME_TO_QUERY: Record<(typeof TIMEFRAMES)[number], ChartTimeframe> = {
  "1M": "1m",
  "5M": "5m",
  "15M": "15m",
  "1H": "1h",
  "4H": "4h",
  "1D": "1d",
};

const TIMEFRAME_TO_SECONDS: Record<(typeof TIMEFRAMES)[number], number> = {
  "1M": 60,
  "5M": 5 * 60,
  "15M": 15 * 60,
  "1H": 60 * 60,
  "4H": 4 * 60 * 60,
  "1D": 24 * 60 * 60,
};

function mergeLivePriceIntoCandles(
  candles: CandleRecord[],
  price: number,
  publishTime: number,
  intervalSec: number,
): CandleRecord[] {
  if (!Number.isFinite(price) || price <= 0 || !Number.isFinite(publishTime) || publishTime <= 0) {
    return candles;
  }

  const bucket = publishTime - (publishTime % intervalSec);
  if (bucket <= 0) {
    return candles;
  }

  if (candles.length === 0) {
    return [{ ts: bucket, open: price, high: price, low: price, close: price, volume: 1 }];
  }

  const next = [...candles];
  const lastIndex = next.length - 1;
  const last = next[lastIndex];

  if (bucket < last.ts) {
    return candles;
  }

  if (bucket === last.ts) {
    next[lastIndex] = {
      ...last,
      high: Math.max(last.high, price),
      low: Math.min(last.low, price),
      close: price,
      volume: Math.max(0, last.volume) + 1,
    };
    return next;
  }

  next.push({ ts: bucket, open: price, high: price, low: price, close: price, volume: 1 });
  if (next.length <= MAX_CHART_CANDLES) {
    return next;
  }
  return next.slice(next.length - MAX_CHART_CANDLES);
}

export function TradingChartPage() {
  const { t, localeTag } = useI18n();
  const [timeframe, setTimeframe] = useState<(typeof TIMEFRAMES)[number]>("1H");
  const [candles, setCandles] = useState<CandleRecord[]>([]);
  const [signals, setSignals] = useState<TradingSignal[]>([]);
  const [systemStatus, setSystemStatus] = useState<SystemStatus | null>(null);
  const [wsConnected, setWsConnected] = useState(false);
  const candleIntervalRef = useRef<number>(TIMEFRAME_TO_SECONDS["1H"]);

  useEffect(() => {
    candleIntervalRef.current = TIMEFRAME_TO_SECONDS[timeframe];
  }, [timeframe]);

  useEffect(() => {
    const controller = new AbortController();
    getChartCandles(
      {
        market: MARKET_CODE,
        timeframe: TIMEFRAME_TO_QUERY[timeframe],
        limit: 120,
      },
      controller.signal,
    )
      .then((payload) => {
        if (Array.isArray(payload.candles)) {
          setCandles(payload.candles);
        }
      })
      .catch(() => {
        // Ignore fetch errors so websocket stream can still hydrate the chart.
      });

    return () => {
      controller.abort();
    };
  }, [timeframe]);

  useEffect(() => createChannelSocket({
    channels: [
      `market.price.${MARKET_CODE}`,
      "agent.signals",
      "system.status",
    ],
    onOpen: () => setWsConnected(true),
    onError: () => setWsConnected(false),
    onMessage: (message) => {
      if (message.channel === `market.price.${MARKET_CODE}` && message.data && typeof message.data === "object") {
        const payload = message.data as MarketPricePayload;
        if (typeof payload.price === "number" && typeof payload.publish_time === "number") {
          setCandles((prev) => mergeLivePriceIntoCandles(prev, payload.price, payload.publish_time, candleIntervalRef.current));
        }
        return;
      }

      if (message.channel === "agent.signals" && Array.isArray(message.data)) {
        const parsed = message.data
          .filter((item): item is TradingSignal => (
            typeof item === "object" &&
            item !== null &&
            "agent_name" in item &&
            "side" in item &&
            "price" in item &&
            "confidence" in item &&
            "ts" in item
          ))
          .slice(0, 8);
        setSignals(parsed);
        return;
      }

      if (message.channel === "system.status" && message.data && typeof message.data === "object") {
        setSystemStatus(message.data as SystemStatus);
      }
    },
  }), []);

  const chartData = useMemo(() => candles.map((candle) => ({
    ts: candle.ts,
    time: formatUnixClock(candle.ts, localeTag),
    close: candle.close,
    volume: candle.volume,
  })), [candles, localeTag]);

  const currentPrice = chartData[chartData.length - 1]?.close ?? 0;
  const prevPrice = chartData[chartData.length - 2]?.close ?? currentPrice;
  const priceChange = prevPrice === 0 ? 0 : ((currentPrice - prevPrice) / prevPrice) * 100;
  const isUp = currentPrice >= prevPrice;

  const markerIndexes = useMemo(() => {
    if (chartData.length === 0) {
      return [];
    }
    return [chartData.length - 5, chartData.length - 15, chartData.length - 30].filter((index) => index >= 0);
  }, [chartData.length]);

  return (
    <div className="h-full flex flex-col p-4 gap-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <div className="flex items-center gap-2">
            <span className="text-[#f7931a] text-[0.875rem]">₿</span>
            <h2 className="text-[1.25rem]" style={{ fontWeight: 600 }}>BTC/USD Perp</h2>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-[1.125rem]" style={{ fontWeight: 600 }}>
              {chartData.length > 0 ? `$${currentPrice.toLocaleString(localeTag)}` : "--"}
            </span>
            <span className={`text-[0.8125rem] ${isUp ? "text-[#22c55e]" : "text-[#ef4444]"}`}>
              {chartData.length > 1 ? `${isUp ? "+" : ""}${priceChange.toFixed(2)}%` : "--"}
            </span>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1.5 text-[0.75rem] text-muted-foreground">
            <Wifi className={`w-3 h-3 ${wsConnected ? "text-[#22c55e]" : "text-[#ef4444]"}`} />
            {wsConnected ? t("WS 정상", "WS Healthy") : t("WS 끊김", "WS Disconnected")}
          </div>
          <div className="flex items-center gap-1.5 text-[0.75rem] text-muted-foreground">
            <Activity className={`w-3 h-3 ${systemStatus?.dex_connected ? "text-[#22c55e]" : "text-[#ef4444]"}`} />
            {systemStatus?.dex_connected ? t("DEX 연결됨", "DEX Connected") : t("DEX 연결 끊김", "DEX Disconnected")}
          </div>
          <Link to="/app/sessions" className="text-[0.75rem] text-primary hover:underline flex items-center gap-1">
            {t("세션 제어", "Session Control")} <ArrowUpRight className="w-3 h-3" />
          </Link>
        </div>
      </div>

      <div className="flex items-center gap-1">
        {TIMEFRAMES.map((tf) => (
          <button
            key={tf}
            onClick={() => setTimeframe(tf)}
            className={`px-3 py-1 rounded text-[0.75rem] transition-colors ${
              timeframe === tf ? "bg-primary/10 text-primary" : "text-muted-foreground hover:text-foreground"
            }`}
          >
            {tf}
          </button>
        ))}
      </div>

      <div className="flex-1 flex gap-4 min-h-0">
        <div className="flex-1 bg-card border border-border rounded-xl p-4 flex flex-col min-h-0">
          <div className="flex-1 min-h-0">
            <ResponsiveContainer width="100%" height="75%">
              <AreaChart data={chartData} margin={{ top: 10, right: 10, left: 0, bottom: 0 }}>
                <defs>
                  <linearGradient id="mainGradient" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="#00d4aa" stopOpacity={0.2} />
                    <stop offset="100%" stopColor="#00d4aa" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <XAxis dataKey="time" stroke="#1f2937" tick={{ fill: "#6b7280", fontSize: 10 }} axisLine={false} tickLine={false} interval="preserveStartEnd" />
                <YAxis stroke="#1f2937" tick={{ fill: "#6b7280", fontSize: 10 }} axisLine={false} tickLine={false} domain={["auto", "auto"]} tickFormatter={(value) => `$${(value / 1000).toFixed(1)}k`} width={55} />
                <Tooltip
                  contentStyle={{ background: "#1a1b25", border: "1px solid rgba(255,255,255,0.1)", borderRadius: 8, fontSize: 11, color: "#e8eaed" }}
                  formatter={(value: number) => [`$${value.toLocaleString(localeTag)}`, t("가격", "Price")]}
                />
                <Area type="monotone" dataKey="close" stroke="#00d4aa" strokeWidth={1.5} fill="url(#mainGradient)" />
                {markerIndexes.map((index) => (
                  <ReferenceDot key={chartData[index].ts} x={chartData[index].time} y={chartData[index].close} r={5} fill={index % 2 === 0 ? "#22c55e" : "#ef4444"} stroke="none" />
                ))}
              </AreaChart>
            </ResponsiveContainer>
            <ResponsiveContainer width="100%" height="20%">
              <BarChart data={chartData} margin={{ top: 0, right: 10, left: 0, bottom: 0 }}>
                <XAxis dataKey="time" hide />
                <YAxis hide />
                <Bar dataKey="volume" fill="#00d4aa" opacity={0.15} radius={[1, 1, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>

        <div className="w-72 flex flex-col gap-4">
          <div className="bg-card border border-border rounded-xl p-4 flex-1">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-[0.8125rem]" style={{ fontWeight: 600 }}>{t("시그널 / 체결", "Signals / Fills")}</h3>
              <div className={`w-1.5 h-1.5 rounded-full ${wsConnected ? "bg-[#22c55e] animate-pulse" : "bg-muted-foreground/50"}`} />
            </div>
            <div className="space-y-2">
              {signals.map((signal, index) => (
                <div key={`${signal.agent_name}-${signal.ts}-${index}`} className="flex items-center gap-2 py-2 border-b border-border last:border-0">
                  <div className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${signal.side === "long" ? "bg-[#22c55e]" : "bg-[#ef4444]"}`} />
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center justify-between">
                      <span className="text-[0.75rem] truncate">{signal.agent_name}</span>
                      <span className={`text-[0.6875rem] ${signal.side === "long" ? "text-[#22c55e]" : "text-[#ef4444]"}`}>
                        {signal.side.toUpperCase()}
                      </span>
                    </div>
                    <div className="flex items-center justify-between text-[0.6875rem] text-muted-foreground">
                      <span>${signal.price.toLocaleString(localeTag)}</span>
                      <span>{formatUnixClock(signal.ts, localeTag)}</span>
                    </div>
                  </div>
                </div>
              ))}
              {signals.length === 0 && (
                <p className="text-[0.75rem] text-muted-foreground">{t("실시간 시그널 대기 중...", "Waiting for live signals...")}</p>
              )}
            </div>
          </div>

          <div className="bg-card border border-border rounded-xl p-4">
            <h3 className="text-[0.8125rem] mb-3" style={{ fontWeight: 600 }}>{t("빠른 동작", "Quick Actions")}</h3>
            <div className="space-y-2">
              <Link to="/app/portfolio" className="flex items-center gap-2 text-[0.75rem] text-muted-foreground hover:text-foreground py-1.5 transition-colors">
                <Eye className="w-3.5 h-3.5" /> {t("포트폴리오 보기", "View Portfolio")}
              </Link>
              <Link to="/app/sessions" className="flex items-center gap-2 text-[0.75rem] text-muted-foreground hover:text-foreground py-1.5 transition-colors">
                <Bot className="w-3.5 h-3.5" /> {t("세션 제어 열기", "Open Session Control")}
              </Link>
            </div>
            <div className="mt-4 pt-4 border-t border-border text-[0.75rem] text-muted-foreground space-y-1">
              <p>WS Latency: {systemStatus ? `${systemStatus.ws_latency_ms}ms` : "--"}</p>
              <p>Indexer Lag: {systemStatus ? `${systemStatus.indexer_lag_ms}ms` : "--"}</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
