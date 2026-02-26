import { useEffect, useMemo, useState } from "react";
import { AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer, ReferenceDot } from "recharts";
import { CandleRecord, MarketPricePayload, createChannelSocket, getChartCandles } from "../lib/backend";
import { generateCandleData, mockSignals } from "../lib/mock-data";
import { formatUnixClock } from "../lib/format";
import { useI18n } from "../i18n";

interface BtcChartProps {
  height?: number;
  showSignals?: boolean;
  mini?: boolean;
}

const MARKET_CODE = "BTCUSDT";
const CANDLE_INTERVAL_SEC = 60;
const MAX_CANDLES = 240;

function mergeLivePriceIntoCandles(
  candles: CandleRecord[],
  price: number,
  publishTime: number,
): CandleRecord[] {
  if (!Number.isFinite(price) || price <= 0 || !Number.isFinite(publishTime) || publishTime <= 0) {
    return candles;
  }

  const bucket = publishTime - (publishTime % CANDLE_INTERVAL_SEC);
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
  if (next.length <= MAX_CANDLES) {
    return next;
  }
  return next.slice(next.length - MAX_CANDLES);
}

export function BtcChart({ height = 400, showSignals = true, mini = false }: BtcChartProps) {
  const { t, localeTag } = useI18n();
  const [candles, setCandles] = useState<CandleRecord[]>([]);

  useEffect(() => {
    const controller = new AbortController();
    getChartCandles(
      {
        market: MARKET_CODE,
        timeframe: "1m",
        limit: mini ? 96 : 180,
      },
      controller.signal,
    )
      .then((payload) => {
        if (Array.isArray(payload.candles)) {
          setCandles(payload.candles);
        }
      })
      .catch(() => {
        // Keep fallback chart if backend is unavailable.
      });

    return () => {
      controller.abort();
    };
  }, [mini]);

  useEffect(() => createChannelSocket({
    channels: [`market.price.${MARKET_CODE}`],
    onMessage: (message) => {
      if (message.channel !== `market.price.${MARKET_CODE}` || !message.data || typeof message.data !== "object") {
        return;
      }
      const payload = message.data as MarketPricePayload;
      if (typeof payload.price !== "number" || typeof payload.publish_time !== "number") {
        return;
      }
      setCandles((prev) => mergeLivePriceIntoCandles(prev, payload.price, payload.publish_time));
    },
  }), []);

  const data = useMemo(() => {
    if (candles.length === 0) {
      return generateCandleData(mini ? 48 : 100, localeTag);
    }
    return candles.map((candle) => ({
      ts: candle.ts,
      time: formatUnixClock(candle.ts, localeTag),
      open: candle.open,
      close: candle.close,
      high: candle.high,
      low: candle.low,
      volume: candle.volume,
    }));
  }, [candles, localeTag, mini]);

  const currentPrice = data[data.length - 1]?.close ?? 0;
  const prevPrice = data[data.length - 2]?.close ?? currentPrice;
  const isUp = currentPrice >= prevPrice;
  const priceChangePct = prevPrice === 0 ? 0 : ((currentPrice - prevPrice) / prevPrice * 100);

  return (
    <div className="relative w-full">
      {!mini && (
        <div className="flex items-center justify-between mb-3 px-1">
          <div className="flex items-center gap-3">
            <span className="text-[#f7931a] text-[0.875rem]">BTC</span>
            <span className="text-foreground text-[1.125rem]">${currentPrice.toLocaleString(localeTag)}</span>
            <span className={`text-[0.75rem] ${isUp ? "text-[#22c55e]" : "text-[#ef4444]"}`}>
              {isUp ? "+" : ""}
              {priceChangePct.toFixed(2)}%
            </span>
          </div>
          <span className="text-[0.75rem] text-muted-foreground">BTC/USD Perpetual</span>
        </div>
      )}
      <ResponsiveContainer width="100%" height={height}>
        <AreaChart data={data} margin={{ top: 5, right: mini ? 0 : 10, left: mini ? 0 : 10, bottom: 0 }}>
          <defs>
            <linearGradient id="chartGradient" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="#00d4aa" stopOpacity={0.3} />
              <stop offset="100%" stopColor="#00d4aa" stopOpacity={0} />
            </linearGradient>
          </defs>
          {!mini && <XAxis dataKey="time" stroke="#374151" tick={{ fill: "#6b7280", fontSize: 11 }} axisLine={false} tickLine={false} interval={Math.floor(data.length / 6)} />}
          {!mini && <YAxis stroke="#374151" tick={{ fill: "#6b7280", fontSize: 11 }} axisLine={false} tickLine={false} domain={["auto", "auto"]} tickFormatter={(v) => `$${(v / 1000).toFixed(1)}k`} width={60} />}
          <Tooltip
            contentStyle={{ background: "#1a1b25", border: "1px solid rgba(255,255,255,0.1)", borderRadius: 8, fontSize: 12, color: "#e8eaed" }}
            labelStyle={{ color: "#6b7280" }}
            formatter={(value: number) => [`$${value.toLocaleString(localeTag)}`, t("가격", "Price")]}
          />
          <Area type="monotone" dataKey="close" stroke="#00d4aa" strokeWidth={2} fill="url(#chartGradient)" />
          {showSignals && !mini && mockSignals.slice(0, 3).map((s, i) => {
            const idx = data.length - 5 - i * 8;
            if (idx < 0) return null;
            return (
              <ReferenceDot
                key={i}
                x={data[idx]?.time}
                y={data[idx]?.close}
                r={6}
                fill={s.side === "long" ? "#22c55e" : "#ef4444"}
                stroke="none"
              />
            );
          })}
        </AreaChart>
      </ResponsiveContainer>
      {showSignals && !mini && (
        <div className="absolute top-12 right-4 w-56 space-y-1.5">
          {mockSignals.slice(0, 4).map((s, i) => (
            <div key={i} className="flex items-center gap-2 bg-[#12131a]/90 backdrop-blur-sm border border-white/5 rounded-md px-2.5 py-1.5 text-[0.75rem]">
              <div className={`w-1.5 h-1.5 rounded-full ${s.side === "long" ? "bg-[#22c55e]" : "bg-[#ef4444]"}`} />
              <span className="text-muted-foreground truncate">{s.agent_name}</span>
              <span className={`ml-auto ${s.side === "long" ? "text-[#22c55e]" : "text-[#ef4444]"}`}>
                {s.side === "long" ? "LONG" : "SHORT"}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
