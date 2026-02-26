import { Globe } from "lucide-react";
import { useI18n } from "../i18n";
import { FancyDropdown } from "./FancyDropdown";
import { cn } from "./ui/utils";

interface LanguageSwitcherProps {
  className?: string;
}

export function LanguageSwitcher({ className }: LanguageSwitcherProps) {
  const { locale, setLocale, t } = useI18n();

  return (
    <div className={cn("inline-flex items-center gap-2.5 rounded-xl border border-border/80 bg-card/80 px-2.5 py-1.5 shadow-sm backdrop-blur", className)}>
      <div className="inline-flex items-center gap-1.5 px-1 text-[0.75rem] text-muted-foreground whitespace-nowrap">
        <Globe className="h-3.5 w-3.5" />
        <span>{t("언어", "Language")}</span>
      </div>
      <FancyDropdown
        ariaLabel={t("언어 선택", "Select language")}
        value={locale}
        onValueChange={(nextValue) => setLocale(nextValue as "ko" | "en")}
        options={[
          {
            value: "ko",
            label: t("한국어", "Korean"),
          },
          {
            value: "en",
            label: t("영어", "English"),
          },
        ]}
        triggerClassName="min-w-[108px]"
      />
    </div>
  );
}
