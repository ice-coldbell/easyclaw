import { useEffect, useMemo, useState } from "react";
import { Link, NavLink, Outlet, useLocation } from "react-router";
import {
  Bot, History, LayoutDashboard,
  LineChart, Lock, Play, Settings, Shield, Trophy, Wrench, Zap
} from "lucide-react";
import { useI18n } from "../i18n";
import { LanguageSwitcher } from "./LanguageSwitcher";
import { AgentRecord, filterAgentsByOwner, getAgents } from "../lib/backend";
import { useWallet } from "../lib/wallet";
import { WalletConnectButton } from "./WalletConnectButton";

export function AppLayout() {
  const { t } = useI18n();
  const { walletPubkey, connecting, openWalletPicker } = useWallet();
  const location = useLocation();
  const [ownedAgents, setOwnedAgents] = useState<AgentRecord[]>([]);
  const [loadingOwnedAgents, setLoadingOwnedAgents] = useState(false);
  const isPublicRoute =
    location.pathname === "/app/leaderboard" ||
    location.pathname.startsWith("/app/agents/");
  const showWalletGate = !walletPubkey && !isPublicRoute;

  useEffect(() => {
    if (!walletPubkey) {
      setOwnedAgents([]);
      setLoadingOwnedAgents(false);
      return undefined;
    }

    const controller = new AbortController();
    const loadOwnedAgents = async () => {
      try {
        setLoadingOwnedAgents(true);
        const response = await getAgents(controller.signal);
        setOwnedAgents(filterAgentsByOwner(response, walletPubkey));
      } catch {
        setOwnedAgents([]);
      } finally {
        setLoadingOwnedAgents(false);
      }
    };

    void loadOwnedAgents();
    return () => controller.abort();
  }, [walletPubkey]);

  const liveCount = useMemo(
    () => ownedAgents.filter((agent) => agent.status === "live").length,
    [ownedAgents],
  );

  const paperCount = useMemo(
    () => ownedAgents.filter((agent) => agent.status === "paper").length,
    [ownedAgents],
  );

  const navItems = [
    { to: "/app/chart", icon: LineChart, label: t("트레이딩 차트", "Trading Chart"), requiresWallet: true },
    { to: "/app/sessions", icon: Play, label: t("세션 제어", "Session Control"), requiresWallet: true },
    { to: "/app/portfolio", icon: LayoutDashboard, label: t("포트폴리오", "Portfolio"), requiresWallet: true },
    { to: "/app/trades", icon: History, label: t("거래 내역", "Trade History"), requiresWallet: true },
    { to: "/app/leaderboard", icon: Trophy, label: t("리더보드", "Leaderboard"), requiresWallet: false },
    { to: "/app/strategy-builder", icon: Wrench, label: t("전략 빌더", "Strategy Builder"), requiresWallet: true },
    { to: "/app/risk", icon: Shield, label: t("리스크 설정", "Risk Settings"), requiresWallet: true },
    { to: "/app/settings/connections", icon: Settings, label: t("설정", "Settings"), requiresWallet: true },
  ];

  return (
    <div className="flex h-screen bg-background overflow-hidden" style={{ fontFamily: "'Inter', sans-serif" }}>
      {/* Sidebar */}
      <aside className="w-[220px] flex-shrink-0 bg-sidebar border-r border-sidebar-border flex flex-col">
        <div className="p-4 flex items-center gap-2 border-b border-sidebar-border">
          <div className="w-8 h-8 rounded-lg bg-primary flex items-center justify-center">
            <Zap className="w-4 h-4 text-primary-foreground" />
          </div>
          <span className="text-[1rem] text-sidebar-foreground" style={{ fontWeight: 600 }}>EasyClaw</span>
        </div>

        <nav className="flex-1 p-3 space-y-0.5 overflow-y-auto">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              onClick={(event) => {
                if (!walletPubkey && item.requiresWallet) {
                  event.preventDefault();
                  if (connecting) {
                    return;
                  }
                  openWalletPicker(item.to);
                }
              }}
              className={({ isActive }) =>
                `flex items-center gap-2.5 px-3 py-2 rounded-lg text-[0.8125rem] transition-colors ${
                  !walletPubkey && item.requiresWallet
                    ? "opacity-70"
                    : ""
                } ${
                  isActive
                    ? "bg-sidebar-accent text-primary"
                    : "text-muted-foreground hover:text-sidebar-foreground hover:bg-sidebar-accent/50"
                }`
              }
            >
              <item.icon className="w-4 h-4" />
              {item.label}
            </NavLink>
          ))}
        </nav>

        <div className="p-3 border-t border-sidebar-border">
          <div className="flex items-center gap-2 px-3 py-2">
            <div className="w-7 h-7 rounded-full bg-primary/20 flex items-center justify-center">
              <Bot className="w-3.5 h-3.5 text-primary" />
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-[0.75rem] text-sidebar-foreground truncate">
                {walletPubkey
                  ? t(`${ownedAgents.length} 에이전트`, `${ownedAgents.length} Agents`)
                  : t("월렛 미연결", "Wallet Not Connected")}
              </p>
              <p className="text-[0.6875rem] text-muted-foreground">
                {walletPubkey
                  ? t(`${liveCount} 라이브 · ${paperCount} 페이퍼`, `${liveCount} Live · ${paperCount} Paper`)
                  : t("우측 상단에서 월렛 연결", "Connect wallet in top-right")}
              </p>
            </div>
          </div>
          <div className="mt-2 flex items-center gap-1.5 px-3 text-[0.6875rem] text-muted-foreground">
            <div className={`w-1.5 h-1.5 rounded-full ${walletPubkey && !loadingOwnedAgents ? "bg-[#22c55e] animate-pulse" : "bg-muted-foreground/50"}`} />
            {walletPubkey
              ? (loadingOwnedAgents ? t("내 에이전트 동기화 중...", "Syncing my agents...") : t("내 에이전트만 표시", "Showing my agents only"))
              : t("월렛 연결 필요", "Wallet connection required")}
          </div>
        </div>
      </aside>

      {/* Main Content */}
      <main className="flex-1 overflow-y-auto">
        <div className="sticky top-0 z-20 border-b border-border bg-background/85 px-4 py-2 backdrop-blur">
          <div className="flex justify-end items-center gap-2">
            <WalletConnectButton />
            <LanguageSwitcher />
          </div>
        </div>
        {showWalletGate ? (
          <div className="p-6 max-w-3xl mx-auto">
            <div className="bg-card border border-border rounded-xl p-6">
              <div className="flex items-center gap-2 mb-2">
                <Lock className="w-4 h-4 text-primary" />
                <h2 className="text-[1rem]" style={{ fontWeight: 600 }}>
                  {t("월렛 연결 필요", "Wallet Connection Required")}
                </h2>
              </div>
              <p className="text-[0.8125rem] text-muted-foreground mb-4">
                {t(
                  "리더보드/에이전트 상세를 제외한 모든 페이지는 월렛 연결 후 접근할 수 있습니다. 우측 상단에서 월렛을 연결하세요.",
                  "All pages except leaderboard/agent details require wallet connection. Connect your wallet from the top-right.",
                )}
              </p>
              <Link
                to="/app/leaderboard"
                className="inline-flex items-center rounded-lg border border-border px-3 py-2 text-[0.8125rem] text-muted-foreground transition-colors hover:text-foreground"
              >
                {t("리더보드로 이동", "Go to Leaderboard")}
              </Link>
            </div>
          </div>
        ) : (
          <Outlet />
        )}
      </main>
    </div>
  );
}
