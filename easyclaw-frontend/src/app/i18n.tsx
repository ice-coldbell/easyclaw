import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

export type Locale = "ko" | "en";

const LOCALE_STORAGE_KEY = "easyclaw.locale";

const autoTranslations: Record<string, string> = {
  "모멘텀 추세추종": "Momentum Trend Following",
  "평균회귀": "Mean Reversion",
  "볼린저 브레이크아웃": "Bollinger Breakout",
  "커스텀 전략": "Custom Strategy",
  "RSI + MACD 기반 추세 방향 진입, ATR 기반 손절": "Trend-following entries via RSI + MACD with ATR-based stop loss",
  "볼린저 밴드 이탈 후 회귀 진입, 밴드 중앙 청산": "Reversion entries after Bollinger Band deviations, exit near band center",
  "볼린저 밴드 돌파 진입, 추세 지속 시 추가 진입": "Breakout entries on Bollinger Band break with scale-ins during trend continuation",
  "직접 진입/청산 규칙과 파라미터를 설정합니다": "Manually configure entry/exit rules and parameters",
  "사용자 정의": "Custom",
  "높음": "High",
  "중간": "Medium",
  "낮음": "Low",
  "방금 전": "Just now",
  "2분 전": "2 min ago",
  "3분 전": "3 min ago",
  "5분 전": "5 min ago",
  "12분 전": "12 min ago",
  "25분 전": "25 min ago",
};

interface I18nContextValue {
  locale: Locale;
  localeTag: string;
  setLocale: (locale: Locale) => void;
  t: (ko: string, en?: string) => string;
}

const I18nContext = createContext<I18nContextValue | null>(null);

function detectBrowserLocale(): Locale {
  if (typeof navigator === "undefined") {
    return "en";
  }
  const preferred = navigator.languages?.[0] ?? navigator.language;
  return preferred?.toLowerCase().startsWith("ko") ? "ko" : "en";
}

function getInitialLocale(): Locale {
  if (typeof window === "undefined") {
    return "en";
  }
  const stored = window.localStorage.getItem(LOCALE_STORAGE_KEY);
  if (stored === "ko" || stored === "en") {
    return stored;
  }
  return detectBrowserLocale();
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocale] = useState<Locale>(getInitialLocale);
  const localeTag = locale === "ko" ? "ko-KR" : "en-US";

  useEffect(() => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, locale);
    document.documentElement.lang = locale;
  }, [locale]);

  const t = useCallback(
    (ko: string, en?: string): string => {
      if (locale === "ko") {
        return ko;
      }
      return en ?? autoTranslations[ko] ?? ko;
    },
    [locale],
  );

  const value = useMemo<I18nContextValue>(
    () => ({
      locale,
      localeTag,
      setLocale,
      t,
    }),
    [locale, localeTag, setLocale, t],
  );

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n() {
  const context = useContext(I18nContext);
  if (!context) {
    throw new Error("useI18n must be used within I18nProvider");
  }
  return context;
}
