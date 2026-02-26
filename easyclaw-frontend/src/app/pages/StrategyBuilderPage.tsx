import { useEffect, useMemo, useState } from "react";
import { AlertTriangle, Check, Link2, Plus, Save, Trash2, Upload } from "lucide-react";
import {
  StrategyTemplateRecord,
  createStrategy,
  getStrategyTemplates,
  patchStrategy,
  publishStrategy,
} from "../lib/backend";
import { useI18n } from "../i18n";

interface Rule {
  id: string;
  type: "entry" | "exit";
  indicator: string;
  condition: string;
  value: string;
}

export function StrategyBuilderPage() {
  const { t } = useI18n();
  const [mode, setMode] = useState<"preset" | "custom">("preset");
  const [templates, setTemplates] = useState<StrategyTemplateRecord[]>([]);
  const [loadingTemplates, setLoadingTemplates] = useState(true);
  const [selectedPreset, setSelectedPreset] = useState<string | null>(null);
  const [strategyName, setStrategyName] = useState("");
  const [status, setStatus] = useState<"draft" | "published">("draft");
  const [strategyID, setStrategyID] = useState<string | null>(null);
  const [rules, setRules] = useState<Rule[]>([
    { id: "r1", type: "entry", indicator: "RSI", condition: "<", value: "30" },
    { id: "r2", type: "entry", indicator: "MACD", condition: "cross_above", value: "signal" },
    { id: "r3", type: "exit", indicator: "RSI", condition: ">", value: "70" },
  ]);
  const [riskMaxPosition, setRiskMaxPosition] = useState("5000");
  const [riskDailyLoss, setRiskDailyLoss] = useState("500");
  const [saved, setSaved] = useState(false);
  const [saving, setSaving] = useState(false);
  const [errors, setErrors] = useState<string[]>([]);
  const [requestError, setRequestError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    const loadTemplates = async () => {
      try {
        setLoadingTemplates(true);
        setRequestError(null);
        const response = await getStrategyTemplates(controller.signal);
        setTemplates(response);
        if (response.length > 0) {
          setSelectedPreset((current) => current ?? response[0].id);
        }
      } catch (loadError) {
        const message = loadError instanceof Error ? loadError.message : String(loadError);
        setRequestError(message);
      } finally {
        setLoadingTemplates(false);
      }
    };
    void loadTemplates();
    return () => controller.abort();
  }, []);

  const selectedTemplate = useMemo(
    () => templates.find((template) => template.id === selectedPreset) ?? null,
    [selectedPreset, templates],
  );

  useEffect(() => {
    if (mode === "preset" && selectedTemplate && strategyName.trim() === "") {
      setStrategyName(selectedTemplate.name);
    }
  }, [mode, selectedTemplate, strategyName]);

  const addRule = (type: "entry" | "exit") => {
    setRules((prev) => [...prev, { id: `r-${Date.now()}`, type, indicator: "", condition: "", value: "" }]);
    setSaved(false);
  };

  const removeRule = (id: string) => {
    setRules((prev) => prev.filter((rule) => rule.id !== id));
    setSaved(false);
  };

  const updateRule = (id: string, field: keyof Rule, value: string) => {
    setRules((prev) => prev.map((rule) => (rule.id === id ? { ...rule, [field]: value } : rule)));
    setSaved(false);
  };

  const validate = () => {
    const nextErrors: string[] = [];
    const parsedMaxPosition = Number(riskMaxPosition);
    const parsedDailyLoss = Number(riskDailyLoss);

    if (!Number.isFinite(parsedMaxPosition) || parsedMaxPosition < 0) {
      nextErrors.push(t("포지션 크기 기본값은 0 이상 숫자여야 합니다.", "Default position size must be a non-negative number."));
    }
    if (!Number.isFinite(parsedDailyLoss) || parsedDailyLoss < 0) {
      nextErrors.push(t("손실 한도 기본값은 0 이상 숫자여야 합니다.", "Default loss limit must be a non-negative number."));
    }

    if (mode === "custom" && !strategyName.trim()) {
      nextErrors.push(t("전략 이름을 입력하세요.", "Enter a strategy name."));
    }
    if (mode === "custom" && rules.filter((rule) => rule.type === "entry").length === 0) {
      nextErrors.push(t("최소 1개의 진입 규칙이 필요합니다.", "At least one entry rule is required."));
    }
    if (mode === "custom" && rules.filter((rule) => rule.type === "exit").length === 0) {
      nextErrors.push(t("최소 1개의 청산 규칙이 필요합니다.", "At least one exit rule is required."));
    }
    if (mode === "custom") {
      for (const rule of rules) {
        if (!rule.indicator || !rule.condition || !rule.value) {
          nextErrors.push(t("모든 규칙의 필드를 채워주세요.", "Fill in all fields for every rule."));
          break;
        }
      }
    }
    if (mode === "preset" && !selectedPreset) {
      nextErrors.push(t("전략 템플릿을 선택하세요.", "Select a strategy template."));
    }

    setErrors([...new Set(nextErrors)]);
    return nextErrors.length === 0;
  };

  const upsertStrategy = async (): Promise<string | null> => {
    if (!validate()) {
      return null;
    }

    try {
      setSaving(true);
      setRequestError(null);

      const riskDefaults = {
        max_position_usdc: Number(riskMaxPosition),
        daily_loss_limit_usdc: Number(riskDailyLoss),
      };

      let name = strategyName.trim();
      let entryRules: Record<string, unknown>;
      let exitRules: Record<string, unknown>;

      if (mode === "preset") {
        if (!selectedTemplate) {
          throw new Error("template not found");
        }
        if (!name) {
          name = selectedTemplate.name;
          setStrategyName(selectedTemplate.name);
        }
        entryRules = {
          mode: "preset",
          template_id: selectedTemplate.id,
          template_rules: selectedTemplate.rules,
        };
        exitRules = {
          mode: "preset",
        };
      } else {
        entryRules = {
          mode: "custom",
          rules: rules
            .filter((rule) => rule.type === "entry")
            .map((rule) => ({ indicator: rule.indicator, condition: rule.condition, value: rule.value })),
        };
        exitRules = {
          mode: "custom",
          rules: rules
            .filter((rule) => rule.type === "exit")
            .map((rule) => ({ indicator: rule.indicator, condition: rule.condition, value: rule.value })),
        };
      }

      let nextStrategyID = strategyID;
      if (nextStrategyID) {
        await patchStrategy(nextStrategyID, { name, entryRules, exitRules });
      } else {
        const created = await createStrategy({
          name,
          entryRules,
          exitRules,
          riskDefaults,
        });
        nextStrategyID = created.strategy_id;
        setStrategyID(nextStrategyID);
      }

      setStatus("draft");
      setSaved(true);
      window.setTimeout(() => setSaved(false), 2000);
      return nextStrategyID;
    } catch (saveError) {
      const message = saveError instanceof Error ? saveError.message : String(saveError);
      setRequestError(message);
      return null;
    } finally {
      setSaving(false);
    }
  };

  const handleSave = async () => {
    await upsertStrategy();
  };

  const handlePublish = async () => {
    const currentStrategyID = await upsertStrategy();
    if (!currentStrategyID) {
      return;
    }

    try {
      setSaving(true);
      setRequestError(null);
      await publishStrategy(currentStrategyID);
      setStatus("published");
      setSaved(true);
      window.setTimeout(() => setSaved(false), 2000);
    } catch (publishError) {
      const message = publishError instanceof Error ? publishError.message : String(publishError);
      setRequestError(message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="p-6 max-w-4xl mx-auto">
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-[1.5rem]" style={{ fontWeight: 600 }}>{t("전략 빌더", "Strategy Builder")}</h1>
          <p className="text-[0.875rem] text-muted-foreground mt-1">{t("트레이딩 전략 생성 및 관리", "Create and manage trading strategies")}</p>
        </div>
        <span className={`text-[0.75rem] px-3 py-1 rounded-full ${
          status === "draft" ? "bg-[#f59e0b]/10 text-[#f59e0b]" : "bg-[#22c55e]/10 text-[#22c55e]"
        }`}>
          {status === "draft" ? t("초안", "Draft") : t("게시됨", "Published")}
        </span>
      </div>

      {requestError && (
        <div className="bg-destructive/5 border border-destructive/20 rounded-xl p-4 mb-6 text-[0.8125rem] text-destructive">
          {t("전략 요청 실패:", "Strategy request failed:")} {requestError}
        </div>
      )}

      <div className="flex gap-2 mb-6">
        <button
          onClick={() => setMode("preset")}
          className={`px-4 py-2 rounded-lg text-[0.8125rem] border transition-colors ${
            mode === "preset" ? "border-primary bg-primary/5 text-primary" : "border-border text-muted-foreground"
          }`}
        >
          {t("프리셋 선택", "Choose Preset")}
        </button>
        <button
          onClick={() => setMode("custom")}
          className={`px-4 py-2 rounded-lg text-[0.8125rem] border transition-colors ${
            mode === "custom" ? "border-primary bg-primary/5 text-primary" : "border-border text-muted-foreground"
          }`}
        >
          {t("커스텀 전략", "Custom Strategy")}
        </button>
      </div>

      {mode === "preset" ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-8">
          {templates.map((template) => (
            <button
              key={template.id}
              onClick={() => {
                setSelectedPreset(template.id);
                setSaved(false);
              }}
              className={`text-left p-5 rounded-xl border transition-all ${
                selectedPreset === template.id ? "border-primary bg-primary/5" : "border-border bg-card hover:border-primary/30"
              }`}
            >
              <div className="flex items-center justify-between mb-2">
                <h3 className="text-[0.9375rem]" style={{ fontWeight: 600 }}>{template.name}</h3>
                <span className={`text-[0.6875rem] px-2 py-0.5 rounded-full ${
                  template.risk_level === "high" ? "bg-[#ef4444]/10 text-[#ef4444]" :
                    template.risk_level === "medium" ? "bg-[#f59e0b]/10 text-[#f59e0b]" :
                      "bg-[#22c55e]/10 text-[#22c55e]"
                }`}>
                  {t("리스크", "Risk")}: {template.risk_level}
                </span>
              </div>
              <p className="text-[0.8125rem] text-muted-foreground">{template.description}</p>
              {selectedPreset === template.id && (
                <div className="mt-3 pt-3 border-t border-border flex items-center gap-1.5 text-primary text-[0.75rem]">
                  <Check className="w-3 h-3" /> {t("선택됨", "Selected")}
                </div>
              )}
            </button>
          ))}
          {templates.length === 0 && !loadingTemplates && (
            <p className="text-[0.8125rem] text-muted-foreground">{t("템플릿이 없습니다.", "No templates available.")}</p>
          )}
        </div>
      ) : (
        <div className="space-y-6 mb-8">
          <div>
            <label className="text-[0.8125rem] text-muted-foreground mb-2 block">{t("전략 이름", "Strategy Name")}</label>
            <input
              type="text"
              value={strategyName}
              onChange={(event) => {
                setStrategyName(event.target.value);
                setSaved(false);
              }}
              placeholder={t("예: My RSI Momentum", "e.g. My RSI Momentum")}
              className="w-full max-w-sm bg-input-background border border-border rounded-lg px-4 py-2.5 text-[0.875rem] placeholder:text-muted-foreground/50 focus:border-primary focus:outline-none"
            />
          </div>

          <div className="bg-card border border-border rounded-xl p-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-[0.9375rem] flex items-center gap-2" style={{ fontWeight: 600 }}>
                <span className="w-2 h-2 rounded-full bg-[#22c55e]" /> {t("진입 조건", "Entry Conditions")}
              </h3>
              <button onClick={() => addRule("entry")} className="text-[0.75rem] text-primary flex items-center gap-1 hover:underline">
                <Plus className="w-3 h-3" /> {t("규칙 추가", "Add Rule")}
              </button>
            </div>
            <div className="space-y-2">
              {rules.filter((rule) => rule.type === "entry").map((rule) => (
                <div key={rule.id} className="flex items-center gap-2">
                  <select
                    value={rule.indicator}
                    onChange={(event) => updateRule(rule.id, "indicator", event.target.value)}
                    className="bg-input-background border border-border rounded-lg px-3 py-2 text-[0.8125rem] focus:outline-none focus:border-primary flex-1"
                  >
                    <option value="">{t("지표 선택", "Select Indicator")}</option>
                    <option value="RSI">RSI</option>
                    <option value="MACD">MACD</option>
                    <option value="EMA">EMA</option>
                    <option value="BB">Bollinger Band</option>
                    <option value="Volume">Volume</option>
                  </select>
                  <select
                    value={rule.condition}
                    onChange={(event) => updateRule(rule.id, "condition", event.target.value)}
                    className="bg-input-background border border-border rounded-lg px-3 py-2 text-[0.8125rem] focus:outline-none focus:border-primary w-36"
                  >
                    <option value="">{t("조건", "Condition")}</option>
                    <option value="<">{t("미만 (&lt;)", "Less than (&lt;)")}</option>
                    <option value=">">{t("초과 (&gt;)", "Greater than (&gt;)")}</option>
                    <option value="cross_above">{t("상향돌파", "Cross Above")}</option>
                    <option value="cross_below">{t("하향돌파", "Cross Below")}</option>
                  </select>
                  <input
                    type="text"
                    value={rule.value}
                    onChange={(event) => updateRule(rule.id, "value", event.target.value)}
                    placeholder={t("값", "Value")}
                    className="bg-input-background border border-border rounded-lg px-3 py-2 text-[0.8125rem] focus:outline-none focus:border-primary w-24"
                  />
                  <button onClick={() => removeRule(rule.id)} className="text-muted-foreground hover:text-destructive transition-colors p-1">
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              ))}
            </div>
          </div>

          <div className="bg-card border border-border rounded-xl p-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-[0.9375rem] flex items-center gap-2" style={{ fontWeight: 600 }}>
                <span className="w-2 h-2 rounded-full bg-[#ef4444]" /> {t("청산 조건", "Exit Conditions")}
              </h3>
              <button onClick={() => addRule("exit")} className="text-[0.75rem] text-primary flex items-center gap-1 hover:underline">
                <Plus className="w-3 h-3" /> {t("규칙 추가", "Add Rule")}
              </button>
            </div>
            <div className="space-y-2">
              {rules.filter((rule) => rule.type === "exit").map((rule) => (
                <div key={rule.id} className="flex items-center gap-2">
                  <select
                    value={rule.indicator}
                    onChange={(event) => updateRule(rule.id, "indicator", event.target.value)}
                    className="bg-input-background border border-border rounded-lg px-3 py-2 text-[0.8125rem] focus:outline-none focus:border-primary flex-1"
                  >
                    <option value="">{t("지표 선택", "Select Indicator")}</option>
                    <option value="RSI">RSI</option>
                    <option value="MACD">MACD</option>
                    <option value="EMA">EMA</option>
                    <option value="BB">Bollinger Band</option>
                    <option value="ATR">ATR ({t("손절", "Stop Loss")})</option>
                  </select>
                  <select
                    value={rule.condition}
                    onChange={(event) => updateRule(rule.id, "condition", event.target.value)}
                    className="bg-input-background border border-border rounded-lg px-3 py-2 text-[0.8125rem] focus:outline-none focus:border-primary w-36"
                  >
                    <option value="">{t("조건", "Condition")}</option>
                    <option value="<">{t("미만 (&lt;)", "Less than (&lt;)")}</option>
                    <option value=">">{t("초과 (&gt;)", "Greater than (&gt;)")}</option>
                    <option value="cross_above">{t("상향돌파", "Cross Above")}</option>
                    <option value="cross_below">{t("하향돌파", "Cross Below")}</option>
                  </select>
                  <input
                    type="text"
                    value={rule.value}
                    onChange={(event) => updateRule(rule.id, "value", event.target.value)}
                    placeholder={t("값", "Value")}
                    className="bg-input-background border border-border rounded-lg px-3 py-2 text-[0.8125rem] focus:outline-none focus:border-primary w-24"
                  />
                  <button onClick={() => removeRule(rule.id)} className="text-muted-foreground hover:text-destructive transition-colors p-1">
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      <div className="bg-card border border-border rounded-xl p-5 mb-6">
        <h3 className="text-[0.9375rem] mb-4" style={{ fontWeight: 600 }}>{t("리스크 파라미터", "Risk Parameters")}</h3>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="text-[0.75rem] text-muted-foreground mb-1.5 block">{t("포지션 크기 기본값 (USD)", "Default Position Size (USD)")}</label>
            <div className="flex items-center gap-2">
              <span className="text-muted-foreground text-[0.8125rem]">$</span>
              <input
                type="number"
                min={0}
                value={riskMaxPosition}
                onChange={(event) => {
                  setRiskMaxPosition(event.target.value);
                  setSaved(false);
                }}
                className="bg-input-background border border-border rounded-lg px-3 py-2 text-[0.8125rem] focus:border-primary focus:outline-none flex-1"
              />
            </div>
          </div>
          <div>
            <label className="text-[0.75rem] text-muted-foreground mb-1.5 block">{t("손실 한도 기본값 (USD)", "Default Loss Limit (USD)")}</label>
            <div className="flex items-center gap-2">
              <span className="text-muted-foreground text-[0.8125rem]">$</span>
              <input
                type="number"
                min={0}
                value={riskDailyLoss}
                onChange={(event) => {
                  setRiskDailyLoss(event.target.value);
                  setSaved(false);
                }}
                className="bg-input-background border border-border rounded-lg px-3 py-2 text-[0.8125rem] focus:border-primary focus:outline-none flex-1"
              />
            </div>
          </div>
        </div>
      </div>

      {errors.length > 0 && (
        <div className="bg-destructive/5 border border-destructive/20 rounded-xl p-4 mb-6">
          <div className="flex items-center gap-2 mb-2">
            <AlertTriangle className="w-4 h-4 text-destructive" />
            <span className="text-[0.8125rem] text-destructive" style={{ fontWeight: 600 }}>{t("유효성 검사 실패", "Validation Failed")}</span>
          </div>
          <ul className="space-y-1">
            {errors.map((errorText, index) => (
              <li key={`${errorText}-${index}`} className="text-[0.75rem] text-muted-foreground flex items-center gap-1.5">
                <span className="w-1 h-1 rounded-full bg-destructive" /> {errorText}
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="flex items-center gap-3 flex-wrap">
        <button
          onClick={() => void handleSave()}
          disabled={saving || loadingTemplates}
          className="bg-secondary text-secondary-foreground px-5 py-2.5 rounded-lg text-[0.875rem] hover:bg-secondary/80 transition-colors flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {saved && status === "draft" ? <><Check className="w-4 h-4 text-primary" /> {t("저장됨", "Saved")}</> : <><Save className="w-4 h-4" /> {saving ? t("저장 중...", "Saving...") : t("전략 저장", "Save Strategy")}</>}
        </button>
        <button
          onClick={() => void handlePublish()}
          disabled={saving || loadingTemplates}
          className="bg-primary text-primary-foreground px-5 py-2.5 rounded-lg text-[0.875rem] hover:bg-primary/90 transition-colors flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {saved && status === "published" ? <><Check className="w-4 h-4" /> {t("게시됨", "Published")}</> : <><Upload className="w-4 h-4" /> {saving ? t("게시 중...", "Publishing...") : t("전략 게시", "Publish Strategy")}</>}
        </button>
        <button className="text-muted-foreground hover:text-foreground border border-border px-4 py-2.5 rounded-lg text-[0.875rem] transition-colors flex items-center gap-2">
          <Link2 className="w-4 h-4" /> {t("Agent에 연결", "Connect to Agent")}
        </button>
        {strategyID && (
          <span className="text-[0.75rem] text-muted-foreground" style={{ fontFamily: "'JetBrains Mono', monospace" }}>
            strategy_id: {strategyID}
          </span>
        )}
      </div>

      {loadingTemplates && (
        <p className="mt-4 text-[0.8125rem] text-muted-foreground">{t("전략 템플릿 로딩 중...", "Loading strategy templates...")}</p>
      )}
    </div>
  );
}
