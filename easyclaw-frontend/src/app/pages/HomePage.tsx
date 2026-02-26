import { useEffect, useState } from "react";
import { Link } from "react-router";
import { ArrowRight, Bot, BarChart3, Shield, Trophy, Zap, Activity, TrendingUp } from "lucide-react";
import { BtcChart } from "../components/BtcChart";
import { LanguageSwitcher } from "../components/LanguageSwitcher";
import { TradingSignal, createChannelSocket } from "../lib/backend";
import { formatUnixClock } from "../lib/format";
import { useI18n } from "../i18n";
import { motion } from "motion/react";

export function HomePage() {
  const { t, localeTag } = useI18n();
  const [signals, setSignals] = useState<TradingSignal[]>([]);

  useEffect(() => createChannelSocket({
    channels: ["agent.signals"],
    onMessage: (message) => {
      if (message.channel !== "agent.signals" || !Array.isArray(message.data)) {
        return;
      }
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
        .slice(0, 6);
      setSignals(parsed);
    },
  }), []);

  const features = [
    {
      icon: Bot,
      title: t("에이전트 자동 거래", "Autonomous Agent Trading"),
      desc: t(
        "AI 에이전트가 24/7 시장을 모니터링하고 전략에 따라 자동으로 거래합니다.",
        "AI agents monitor markets 24/7 and execute trades automatically based on your strategy.",
      ),
    },
    {
      icon: Zap,
      title: t("전략 빌더", "Strategy Builder"),
      desc: t(
        "프리셋 선택 또는 커스텀 규칙으로 나만의 트레이딩 전략을 쉽게 만드세요.",
        "Create your own trading strategy quickly with presets or custom rules.",
      ),
    },
    {
      icon: Shield,
      title: t("리스크 제어", "Risk Controls"),
      desc: t(
        "포지션 한도, 일일 손실 한도, Kill Switch로 자산을 안전하게 보호합니다.",
        "Protect your capital with position limits, daily loss limits, and a kill switch.",
      ),
    },
    {
      icon: BarChart3,
      title: t("성과 대시보드", "Performance Dashboard"),
      desc: t(
        "실시간 PnL, 승률, 드로다운을 한눈에 확인하고 전략을 최적화하세요.",
        "Track real-time PnL, win rate, and drawdown at a glance to optimize your strategy.",
      ),
    },
  ];

  const stats = [
    { label: t("활성 에이전트", "Active Agents"), value: "2,847", suffix: "+" },
    { label: t("총 거래량", "Total Volume"), value: "$142M", suffix: "" },
    { label: t("평균 승률", "Average Win Rate"), value: "64.2", suffix: "%" },
    { label: t("24H 거래", "24H Trades"), value: "18,432", suffix: "" },
  ];

  return (
    <div className="min-h-screen bg-background" style={{ fontFamily: "'Inter', sans-serif" }}>
      {/* Header */}
      <header className="fixed top-0 w-full z-50 bg-background/80 backdrop-blur-xl border-b border-border">
        <div className="max-w-7xl mx-auto px-6 h-16 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="w-8 h-8 rounded-lg bg-primary flex items-center justify-center">
              <Zap className="w-4 h-4 text-primary-foreground" />
            </div>
            <span className="text-[1.125rem]" style={{ fontWeight: 600 }}>EasyClaw</span>
          </div>
          <div className="flex items-center gap-4">
            <LanguageSwitcher />
            <Link to="/onboarding" className="text-[0.875rem] text-muted-foreground hover:text-foreground transition-colors">
              {t("로그인", "Log In")}
            </Link>
            <Link
              to="/onboarding"
              className="bg-primary text-primary-foreground px-4 py-2 rounded-lg text-[0.875rem] hover:bg-primary/90 transition-colors flex items-center gap-2"
            >
              {t("시작하기", "Get Started")} <ArrowRight className="w-4 h-4" />
            </Link>
          </div>
        </div>
      </header>

      {/* Hero */}
      <section className="pt-32 pb-16 px-6">
        <div className="max-w-7xl mx-auto">
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-12 items-center">
            <motion.div
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.6 }}
            >
              <div className="inline-flex items-center gap-2 bg-primary/10 text-primary px-3 py-1 rounded-full text-[0.75rem] mb-6">
                <Activity className="w-3 h-3" />
                Solana DEX · BTC Perpetual
              </div>
              <h1 className="text-[2.5rem] tracking-tight mb-4" style={{ fontWeight: 700, lineHeight: 1.1 }}>
                {t("지금 시장에,", "Bring your agent to")}{" "}
                <span className="text-primary">{t("내 에이전트", "the market")}</span>
                <br />
                {t("바로 올리세요.", "right now.")}
              </h1>
              <p className="text-[1.0625rem] text-muted-foreground mb-8 max-w-lg" style={{ lineHeight: 1.7 }}>
                {t(
                  "실시간 진입 시점을 보면서 전략을 연결하고, 몇 분 안에 에이전트가 자동으로 거래를 시작합니다.",
                  "Connect your strategy with real-time entries and let your agent start trading automatically in minutes.",
                )}
              </p>
              <div className="flex items-center gap-4">
                <Link
                  to="/onboarding"
                  className="bg-primary text-primary-foreground px-6 py-3 rounded-lg text-[0.9375rem] hover:bg-primary/90 transition-all flex items-center gap-2 shadow-lg shadow-primary/20"
                >
                  {t("시작하기", "Get Started")} <ArrowRight className="w-4 h-4" />
                </Link>
                <Link
                  to="/app/leaderboard"
                  className="text-muted-foreground hover:text-foreground px-4 py-3 text-[0.875rem] transition-colors flex items-center gap-2"
                >
                  <Trophy className="w-4 h-4" /> {t("리더보드 보기", "View Leaderboard")}
                </Link>
              </div>
            </motion.div>

            <motion.div
              initial={{ opacity: 0, x: 20 }}
              animate={{ opacity: 1, x: 0 }}
              transition={{ duration: 0.6, delay: 0.2 }}
              className="bg-card border border-border rounded-xl p-4"
            >
              <BtcChart height={320} showSignals />
            </motion.div>
          </div>
        </div>
      </section>

      {/* Stats */}
      <section className="py-12 px-6 border-y border-border bg-card/30">
        <div className="max-w-7xl mx-auto grid grid-cols-2 md:grid-cols-4 gap-6">
          {stats.map((stat) => (
            <motion.div
              key={stat.label}
              initial={{ opacity: 0, y: 10 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.4 }}
              className="text-center"
            >
              <div className="text-[1.75rem] text-foreground" style={{ fontWeight: 700 }}>
                {stat.value}<span className="text-primary">{stat.suffix}</span>
              </div>
              <p className="text-[0.8125rem] text-muted-foreground mt-1">{stat.label}</p>
            </motion.div>
          ))}
        </div>
      </section>

      {/* Features */}
      <section className="py-20 px-6">
        <div className="max-w-7xl mx-auto">
          <h2 className="text-center text-[1.75rem] mb-3" style={{ fontWeight: 700 }}>{t("핵심 기능", "Core Features")}</h2>
          <p className="text-center text-muted-foreground text-[0.9375rem] mb-12">
            {t("에이전트 트레이딩에 필요한 모든 것을 하나의 플랫폼에서", "Everything you need for agent trading in one platform")}
          </p>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
            {features.map((f, i) => (
              <motion.div
                key={f.title}
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ duration: 0.4, delay: i * 0.1 }}
                className="bg-card border border-border rounded-xl p-6 hover:border-primary/30 transition-colors group"
              >
                <div className="w-10 h-10 rounded-lg bg-primary/10 flex items-center justify-center mb-4 group-hover:bg-primary/20 transition-colors">
                  <f.icon className="w-5 h-5 text-primary" />
                </div>
                <h3 className="text-[0.9375rem] mb-2" style={{ fontWeight: 600 }}>{f.title}</h3>
                <p className="text-[0.8125rem] text-muted-foreground" style={{ lineHeight: 1.6 }}>{f.desc}</p>
              </motion.div>
            ))}
          </div>
        </div>
      </section>

      {/* Live Feed */}
      <section className="py-16 px-6 bg-card/30 border-t border-border">
        <div className="max-w-7xl mx-auto">
          <div className="flex items-center gap-3 mb-8">
            <div className="w-2 h-2 rounded-full bg-[#22c55e] animate-pulse" />
            <h2 className="text-[1.25rem]" style={{ fontWeight: 600 }}>{t("실시간 에이전트 활동", "Live Agent Activity")}</h2>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {signals.map((s, i) => (
              <motion.div
                key={i}
                initial={{ opacity: 0, x: -10 }}
                animate={{ opacity: 1, x: 0 }}
                transition={{ duration: 0.3, delay: i * 0.1 }}
                className="bg-card border border-border rounded-lg p-4 flex items-center gap-4"
              >
                <div className={`w-10 h-10 rounded-lg flex items-center justify-center ${s.side === "long" ? "bg-[#22c55e]/10" : "bg-[#ef4444]/10"}`}>
                  <TrendingUp className={`w-5 h-5 ${s.side === "long" ? "text-[#22c55e]" : "text-[#ef4444] rotate-180"}`} />
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-[0.8125rem] truncate">{s.agent_name}</p>
                  <p className="text-[0.75rem] text-muted-foreground">${s.price.toLocaleString(localeTag)} · {formatUnixClock(s.ts, localeTag)}</p>
                </div>
                <span className={`text-[0.75rem] px-2 py-0.5 rounded ${s.side === "long" ? "bg-[#22c55e]/10 text-[#22c55e]" : "bg-[#ef4444]/10 text-[#ef4444]"}`}>
                  {s.side.toUpperCase()}
                </span>
              </motion.div>
            ))}
            {signals.length === 0 && (
              <p className="text-[0.8125rem] text-muted-foreground">{t("실시간 에이전트 활동 대기 중...", "Waiting for live agent activity...")}</p>
            )}
          </div>
        </div>
      </section>

      {/* CTA */}
      <section className="py-20 px-6">
        <div className="max-w-2xl mx-auto text-center">
          <h2 className="text-[1.75rem] mb-4" style={{ fontWeight: 700 }}>{t("지금 바로 시작하세요", "Start Now")}</h2>
          <p className="text-muted-foreground text-[0.9375rem] mb-8">
            {t("에이전트 설치부터 자동 거래까지, 10분이면 충분합니다.", "From setup to automated trading, it takes only 10 minutes.")}
          </p>
          <Link
            to="/onboarding"
            className="inline-flex items-center gap-2 bg-primary text-primary-foreground px-8 py-3.5 rounded-lg text-[1rem] hover:bg-primary/90 transition-all shadow-lg shadow-primary/20"
          >
            {t("에이전트 시작하기", "Launch Agent")} <ArrowRight className="w-4 h-4" />
          </Link>
        </div>
      </section>

      {/* Footer */}
      <footer className="border-t border-border py-8 px-6">
        <div className="max-w-7xl mx-auto flex items-center justify-between text-[0.75rem] text-muted-foreground">
          <div className="flex items-center gap-2">
            <Zap className="w-3.5 h-3.5 text-primary" />
            <span>EasyClaw v1</span>
          </div>
          <span>Solana DEX · BTC Perpetual</span>
        </div>
      </footer>
    </div>
  );
}
