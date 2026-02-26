import { Link } from "react-router";
import { ArrowLeft } from "lucide-react";
import { LanguageSwitcher } from "../components/LanguageSwitcher";
import { useI18n } from "../i18n";

export function NotFoundPage() {
  const { t } = useI18n();
  return (
    <div className="min-h-screen flex items-center justify-center bg-background" style={{ fontFamily: "'Inter', sans-serif" }}>
      <div className="fixed right-4 top-4">
        <LanguageSwitcher />
      </div>
      <div className="text-center">
        <p className="text-[4rem] text-primary" style={{ fontWeight: 700 }}>404</p>
        <p className="text-[1.125rem] text-muted-foreground mb-6">{t("페이지를 찾을 수 없습니다", "Page not found")}</p>
        <Link to="/" className="text-[0.875rem] text-primary hover:underline flex items-center gap-1.5 justify-center">
          <ArrowLeft className="w-4 h-4" /> {t("홈으로 돌아가기", "Back to Home")}
        </Link>
      </div>
    </div>
  );
}
