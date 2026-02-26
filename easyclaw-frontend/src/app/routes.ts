import { createBrowserRouter } from "react-router";
import { HomePage } from "./pages/HomePage";
import { OnboardingPage } from "./pages/OnboardingPage";
import { AppLayout } from "./components/AppLayout";
import { TradingChartPage } from "./pages/TradingChartPage";
import { SessionControlPage } from "./pages/SessionControlPage";
import { PortfolioPage } from "./pages/PortfolioPage";
import { AgentDetailPage } from "./pages/AgentDetailPage";
import { TradeHistoryPage } from "./pages/TradeHistoryPage";
import { LeaderboardPage } from "./pages/LeaderboardPage";
import { RiskSettingsPage } from "./pages/RiskSettingsPage";
import { SettingsPage } from "./pages/SettingsPage";
import { StrategyBuilderPage } from "./pages/StrategyBuilderPage";
import { NotFoundPage } from "./pages/NotFoundPage";

export const router = createBrowserRouter([
  {
    path: "/",
    Component: HomePage,
  },
  {
    path: "/onboarding",
    Component: OnboardingPage,
  },
  {
    path: "/app",
    Component: AppLayout,
    children: [
      { index: true, Component: TradingChartPage },
      { path: "chart", Component: TradingChartPage },
      { path: "sessions", Component: SessionControlPage },
      { path: "portfolio", Component: PortfolioPage },
      { path: "agents/:agentId", Component: AgentDetailPage },
      { path: "trades", Component: TradeHistoryPage },
      { path: "leaderboard", Component: LeaderboardPage },
      { path: "risk", Component: RiskSettingsPage },
      { path: "settings/connections", Component: SettingsPage },
      { path: "strategy-builder", Component: StrategyBuilderPage },
    ],
  },
  {
    path: "*",
    Component: NotFoundPage,
  },
]);
