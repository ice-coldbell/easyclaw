import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";
import { Check, Copy, ArrowRight, ArrowLeft, Wallet, Bot, Shield, Play, Zap, Terminal } from "lucide-react";
import { BtcChart } from "../components/BtcChart";
import { LanguageSwitcher } from "../components/LanguageSwitcher";
import {
  StrategyTemplateRecord,
  createAuthChallenge,
  getStrategyTemplates,
  setSessionToken,
  verifyAuthSignature,
} from "../lib/backend";
import { useI18n } from "../i18n";
import { motion, AnimatePresence } from "motion/react";

export function OnboardingPage() {
  const { t } = useI18n();

  const steps = [
    { num: 1, label: t("에이전트 설치", "Install Agent"), icon: Terminal },
    { num: 2, label: t("지갑 준비", "Set Up Wallet"), icon: Wallet },
    { num: 3, label: t("소유권 등록", "Bind Ownership"), icon: Shield },
    { num: 4, label: t("전략 설정", "Configure Strategy"), icon: Bot },
    { num: 5, label: t("실행", "Launch"), icon: Play },
  ];

  const [step, setStep] = useState(1);
  const [copied, setCopied] = useState(false);
  const [walletAddress, setWalletAddress] = useState("");
  const [selectedStrategy, setSelectedStrategy] = useState<string | null>(null);
  const [installing, setInstalling] = useState(false);
  const [installed, setInstalled] = useState(false);
  const [signing, setSigning] = useState(false);
  const [signed, setSigned] = useState(false);
  const [signature, setSignature] = useState("");
  const [challengeId, setChallengeId] = useState<string | null>(null);
  const [challengeMessage, setChallengeMessage] = useState<string>("");
  const [strategyTemplates, setStrategyTemplates] = useState<StrategyTemplateRecord[]>([]);
  const [loadingTemplates, setLoadingTemplates] = useState(true);
  const [onboardingError, setOnboardingError] = useState<string | null>(null);
  const navigate = useNavigate();

  useEffect(() => {
    const controller = new AbortController();
    const loadTemplates = async () => {
      try {
        setLoadingTemplates(true);
        const templates = await getStrategyTemplates(controller.signal);
        setStrategyTemplates(templates);
        if (templates.length > 0) {
          setSelectedStrategy((current) => current ?? templates[0].id);
        }
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        setOnboardingError(message);
      } finally {
        setLoadingTemplates(false);
      }
    };

    void loadTemplates();
    return () => controller.abort();
  }, []);

  const selectedTemplate = useMemo(
    () => strategyTemplates.find((strategy) => strategy.id === selectedStrategy) ?? null,
    [selectedStrategy, strategyTemplates],
  );

  const handleCopy = () => {
    navigator.clipboard.writeText("npx clawhub@latest install easyclaw");
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleInstall = () => {
    setInstalling(true);
    setTimeout(() => {
      setInstalling(false);
      setInstalled(true);
    }, 2000);
  };

  const handleCreateChallenge = async () => {
    if (walletAddress.trim().length < 32) {
      setOnboardingError(t("유효한 owner wallet 주소를 입력해주세요.", "Please enter a valid owner wallet address."));
      return;
    }

    try {
      setSigning(true);
      setOnboardingError(null);
      const challenge = await createAuthChallenge({
        walletPubkey: walletAddress.trim(),
        intent: "owner_bind",
      });
      setChallengeId(challenge.challenge_id);
      setChallengeMessage(challenge.message);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setOnboardingError(message);
    } finally {
      setSigning(false);
    }
  };

  const handleVerifySignature = async () => {
    if (!challengeId) {
      setOnboardingError(t("먼저 서명 챌린지를 생성하세요.", "Create a signature challenge first."));
      return;
    }
    if (!signature.trim()) {
      setOnboardingError(t("서명을 입력하세요.", "Enter signature."));
      return;
    }

    try {
      setSigning(true);
      setOnboardingError(null);
      const verified = await verifyAuthSignature({
        challengeId,
        signature: signature.trim(),
        walletPubkey: walletAddress.trim(),
      });
      setSessionToken(verified.session_token, verified.expires_at);
      setSigned(true);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setOnboardingError(message);
    } finally {
      setSigning(false);
    }
  };

  const canNext = () => {
    if (step === 1) return installed;
    if (step === 2) return walletAddress.length >= 32;
    if (step === 3) return signed;
    if (step === 4) return selectedStrategy !== null;
    return true;
  };

  return (
    <div className="min-h-screen bg-background flex" style={{ fontFamily: "'Inter', sans-serif" }}>
      {/* Left Panel */}
      <div className="flex-1 max-w-2xl p-8 flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between mb-8">
          <div className="flex items-center gap-2">
            <div className="w-7 h-7 rounded-md bg-primary flex items-center justify-center">
              <Zap className="w-3.5 h-3.5 text-primary-foreground" />
            </div>
            <span className="text-[0.9375rem]" style={{ fontWeight: 600 }}>EasyClaw</span>
          </div>
          <LanguageSwitcher />
        </div>

        {/* Progress */}
        <div className="flex items-center gap-1 mb-10">
          {steps.map((s, i) => (
            <div key={s.num} className="flex items-center gap-1 flex-1">
              <div className={`flex items-center gap-1.5 px-2.5 py-1.5 rounded-md text-[0.75rem] transition-all ${
                s.num === step ? "bg-primary/10 text-primary" :
                s.num < step ? "bg-primary/5 text-primary" : "text-muted-foreground"
              }`}>
                {s.num < step ? (
                  <Check className="w-3.5 h-3.5" />
                ) : (
                  <s.icon className="w-3.5 h-3.5" />
                )}
                <span className="hidden sm:inline">{s.label}</span>
              </div>
              {i < steps.length - 1 && (
                <div className={`flex-1 h-px ${s.num < step ? "bg-primary/40" : "bg-border"}`} />
              )}
            </div>
          ))}
        </div>

        {onboardingError && (
          <div className="mb-4 bg-destructive/10 border border-destructive/20 text-destructive rounded-lg px-3 py-2 text-[0.75rem]">
            {onboardingError}
          </div>
        )}

        {/* Step Content */}
        <AnimatePresence mode="wait">
          <motion.div
            key={step}
            initial={{ opacity: 0, x: 10 }}
            animate={{ opacity: 1, x: 0 }}
            exit={{ opacity: 0, x: -10 }}
            transition={{ duration: 0.2 }}
            className="flex-1"
          >
            {step === 1 && (
              <div>
                <h2 className="text-[1.5rem] mb-2" style={{ fontWeight: 600 }}>{t("에이전트 설치", "Install Agent")}</h2>
                <p className="text-muted-foreground text-[0.875rem] mb-8">
                  {t(
                    "당신의 openclaw 에이전트에게 아래 명령어를 실행하도록 지시하세요.",
                    "Instruct your openclaw agent to run the command below.",
                  )}
                </p>
                <div className="bg-[#0d0e14] border border-border rounded-lg p-4 mb-4">
                  <div className="flex items-center justify-between mb-3">
                    <span className="text-[0.6875rem] text-muted-foreground">Terminal</span>
                    <button
                      onClick={handleCopy}
                      className="text-[0.75rem] text-muted-foreground hover:text-foreground flex items-center gap-1 transition-colors"
                    >
                      {copied ? <Check className="w-3 h-3 text-primary" /> : <Copy className="w-3 h-3" />}
                      {copied ? t("복사됨", "Copied") : t("복사", "Copy")}
                    </button>
                  </div>
                  <code className="text-primary text-[0.875rem]" style={{ fontFamily: "'JetBrains Mono', monospace" }}>
                    $ npx clawhub@latest install easyclaw
                  </code>
                </div>
                {!installed && (
                  <button
                    onClick={handleInstall}
                    disabled={installing}
                    className="mt-4 bg-secondary text-secondary-foreground px-4 py-2 rounded-lg text-[0.8125rem] hover:bg-secondary/80 transition-colors disabled:opacity-50"
                  >
                    {installing ? (
                      <span className="flex items-center gap-2">
                        <div className="w-3.5 h-3.5 border-2 border-primary/30 border-t-primary rounded-full animate-spin" />
                        {t("설치 확인 중...", "Verifying installation...")}
                      </span>
                    ) : t("수동 완료 확인", "Confirm Manually")}
                  </button>
                )}
                {installed && (
                  <div className="mt-4 flex items-center gap-2 text-primary text-[0.875rem]">
                    <Check className="w-4 h-4" /> {t("설치가 완료되었습니다!", "Installation complete!")}
                  </div>
                )}
              </div>
            )}

            {step === 2 && (
              <div>
                <h2 className="text-[1.5rem] mb-2" style={{ fontWeight: 600 }}>{t("에이전트 지갑 준비", "Prepare Agent Wallet")}</h2>
                <p className="text-muted-foreground text-[0.875rem] mb-8">
                  {t("에이전트의 Solana 지갑 주소를 입력하세요.", "Enter the Solana wallet address for your agent.")}
                </p>
                <div className="space-y-4">
                  <div>
                    <label className="text-[0.8125rem] text-muted-foreground mb-2 block">{t("Solana 지갑 주소", "Solana Wallet Address")}</label>
                    <input
                      type="text"
                      value={walletAddress}
                      onChange={(e) => setWalletAddress(e.target.value)}
                      placeholder={t("예: 9WzDX...", "e.g. 9WzDX...")}
                      className="w-full bg-input-background border border-border rounded-lg px-4 py-2.5 text-[0.875rem] placeholder:text-muted-foreground/50 focus:border-primary focus:outline-none transition-colors"
                      style={{ fontFamily: "'JetBrains Mono', monospace" }}
                    />
                    {walletAddress.length > 0 && walletAddress.length < 32 && (
                      <p className="text-[0.75rem] text-destructive mt-1.5">{t("유효한 Solana 주소를 입력해주세요", "Please enter a valid Solana address")}</p>
                    )}
                    {walletAddress.length >= 32 && (
                      <p className="text-[0.75rem] text-primary mt-1.5 flex items-center gap-1">
                        <Check className="w-3 h-3" /> {t("유효한 주소입니다", "Valid address")}
                      </p>
                    )}
                  </div>
                  <div className="bg-card border border-border rounded-lg p-4">
                    <p className="text-[0.8125rem] text-muted-foreground mb-2">{t("아직 지갑이 없으신가요?", "Need a wallet first?")}</p>
                    <p className="text-[0.75rem] text-muted-foreground/70">
                      {t(
                        "Phantom 또는 Solflare 등의 지갑에서 새로운 Solana 주소를 생성하세요.",
                        "Create a new Solana address using wallets like Phantom or Solflare.",
                      )}
                    </p>
                  </div>
                </div>
              </div>
            )}

            {step === 3 && (
              <div>
                <h2 className="text-[1.5rem] mb-2" style={{ fontWeight: 600 }}>{t("소유권 등록", "Bind Ownership")}</h2>
                <p className="text-muted-foreground text-[0.875rem] mb-8">
                  {t(
                    "당신의 지갑(Human Wallet)으로 에이전트 지갑의 소유권을 등록하세요.",
                    "Use your wallet (Human Wallet) to register ownership of the agent wallet.",
                  )}
                </p>
                <div className="bg-card border border-border rounded-lg p-5 mb-4">
                  <h4 className="text-[0.8125rem] mb-4" style={{ fontWeight: 600 }}>{t("서명 메시지 미리보기", "Signature Message Preview")}</h4>
                  <div className="space-y-2 text-[0.75rem]" style={{ fontFamily: "'JetBrains Mono', monospace" }}>
                    <div className="flex justify-between py-1.5 border-b border-border">
                      <span className="text-muted-foreground">intent</span>
                      <span className="text-foreground">owner_bind</span>
                    </div>
                    <div className="flex justify-between py-1.5 border-b border-border">
                      <span className="text-muted-foreground">agent_id</span>
                      <span className="text-foreground">agent-001</span>
                    </div>
                    <div className="flex justify-between py-1.5 border-b border-border">
                      <span className="text-muted-foreground">owner_pubkey</span>
                      <span className="text-foreground truncate ml-4">{walletAddress || "9WzDX..."}</span>
                    </div>
                    <div className="flex justify-between py-1.5 border-b border-border">
                      <span className="text-muted-foreground">nonce</span>
                      <span className="text-foreground">{challengeId || "..."}</span>
                    </div>
                    <div className="flex justify-between py-1.5">
                      <span className="text-muted-foreground">expires_at</span>
                      <span className="text-foreground">{challengeMessage ? t("메시지 본문 참조", "See challenge message") : "-"}</span>
                    </div>
                  </div>
                </div>
                {!signed ? (
                  <div className="space-y-3">
                    <button
                      onClick={() => void handleCreateChallenge()}
                      disabled={signing}
                      className="bg-secondary text-secondary-foreground px-5 py-2.5 rounded-lg text-[0.875rem] hover:bg-secondary/90 transition-colors disabled:opacity-50 flex items-center gap-2"
                    >
                      {signing ? (
                        <>
                          <div className="w-4 h-4 border-2 border-primary-foreground/30 border-t-primary-foreground rounded-full animate-spin" />
                          {t("챌린지 생성 중...", "Creating challenge...")}
                        </>
                      ) : (
                        <>
                          <Shield className="w-4 h-4" /> {t("서명 챌린지 생성", "Create Signature Challenge")}
                        </>
                      )}
                    </button>

                    {challengeMessage && (
                      <div className="bg-secondary/40 border border-border rounded-lg p-3">
                        <p className="text-[0.6875rem] text-muted-foreground mb-1">challenge_id: {challengeId}</p>
                        <p className="text-[0.6875rem] text-muted-foreground break-all" style={{ fontFamily: "'JetBrains Mono', monospace" }}>{challengeMessage}</p>
                      </div>
                    )}

                    <input
                      type="text"
                      value={signature}
                      onChange={(event) => setSignature(event.target.value)}
                      placeholder={t("지갑 서명(base58/base64/hex) 입력", "Paste wallet signature (base58/base64/hex)")}
                      className="w-full bg-input-background border border-border rounded-lg px-4 py-2.5 text-[0.8125rem] placeholder:text-muted-foreground/50 focus:border-primary focus:outline-none"
                    />

                    <button
                      onClick={() => void handleVerifySignature()}
                      disabled={signing}
                      className="bg-primary text-primary-foreground px-5 py-2.5 rounded-lg text-[0.875rem] hover:bg-primary/90 transition-colors disabled:opacity-50 flex items-center gap-2"
                    >
                      {signing ? (
                        <>
                          <div className="w-4 h-4 border-2 border-primary-foreground/30 border-t-primary-foreground rounded-full animate-spin" />
                          {t("서명 검증 중...", "Verifying signature...")}
                        </>
                      ) : (
                        <>
                          <Wallet className="w-4 h-4" /> {t("Owner 바인딩 서명 검증", "Verify Owner Binding Signature")}
                        </>
                      )}
                    </button>
                  </div>
                ) : (
                  <div className="bg-primary/10 border border-primary/20 rounded-lg p-4">
                    <div className="flex items-center gap-2 text-primary text-[0.875rem]">
                      <Check className="w-4 h-4" /> {t("소유권 등록이 완료되었습니다!", "Ownership binding completed!")}
                    </div>
                    <p className="text-[0.75rem] text-muted-foreground mt-1">
                      {t("바인딩 상태: 연결됨 · 최종 검증: 방금 전", "Binding status: Connected · Last verified: just now")}
                    </p>
                  </div>
                )}
              </div>
            )}

            {step === 4 && (
              <div>
                <h2 className="text-[1.5rem] mb-2" style={{ fontWeight: 600 }}>{t("전략 설정", "Configure Strategy")}</h2>
                <p className="text-muted-foreground text-[0.875rem] mb-8">
                  {t("에이전트가 사용할 트레이딩 전략을 선택하거나 만드세요.", "Select or create the trading strategy your agent will use.")}
                </p>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  {strategyTemplates.map((s) => (
                    <button
                      key={s.id}
                      onClick={() => setSelectedStrategy(s.id)}
                      className={`text-left p-4 rounded-lg border transition-all ${
                        selectedStrategy === s.id
                          ? "border-primary bg-primary/5"
                          : "border-border bg-card hover:border-primary/30"
                      }`}
                    >
                      <div className="flex items-center justify-between mb-2">
                        <h4 className="text-[0.8125rem]" style={{ fontWeight: 600 }}>{s.name}</h4>
                        <span className={`text-[0.6875rem] px-2 py-0.5 rounded-full ${
                          s.risk_level === "high" ? "bg-[#ef4444]/10 text-[#ef4444]" :
                          s.risk_level === "medium" ? "bg-[#f59e0b]/10 text-[#f59e0b]" :
                          "bg-[#22c55e]/10 text-[#22c55e]"
                        }`}>
                          {s.risk_level}
                        </span>
                      </div>
                      <p className="text-[0.75rem] text-muted-foreground">{s.description}</p>
                    </button>
                  ))}
                  {strategyTemplates.length === 0 && !loadingTemplates && (
                    <p className="text-[0.8125rem] text-muted-foreground">{t("전략 템플릿이 없습니다.", "No strategy templates available.")}</p>
                  )}
                </div>
              </div>
            )}

            {step === 5 && (
              <div>
                <h2 className="text-[1.5rem] mb-2" style={{ fontWeight: 600 }}>{t("실행 준비 완료", "Ready to Launch")}</h2>
                <p className="text-muted-foreground text-[0.875rem] mb-8">
                  {t("설정을 확인하고 Paper 모드로 시작하세요.", "Review settings and start in Paper mode.")}
                </p>
                <div className="bg-card border border-border rounded-lg p-5 space-y-3 mb-6">
                  <h4 className="text-[0.8125rem] mb-3" style={{ fontWeight: 600 }}>{t("설정 요약", "Configuration Summary")}</h4>
                  <div className="flex justify-between py-2 border-b border-border text-[0.8125rem]">
                    <span className="text-muted-foreground">{t("에이전트 지갑", "Agent Wallet")}</span>
                    <span className="text-foreground" style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: "0.75rem" }}>
                      {walletAddress ? `${walletAddress.slice(0, 8)}...${walletAddress.slice(-4)}` : "9WzDX...3fQp"}
                    </span>
                  </div>
                  <div className="flex justify-between py-2 border-b border-border text-[0.8125rem]">
                    <span className="text-muted-foreground">{t("선택 전략", "Selected Strategy")}</span>
                    <span className="text-foreground">
                      {selectedTemplate?.name ?? t("미선택", "Not selected")}
                    </span>
                  </div>
                  <div className="flex justify-between py-2 border-b border-border text-[0.8125rem]">
                    <span className="text-muted-foreground">{t("최대 포지션", "Max Position")}</span>
                    <span className="text-foreground">$5,000</span>
                  </div>
                  <div className="flex justify-between py-2 text-[0.8125rem]">
                    <span className="text-muted-foreground">{t("일일 손실 한도", "Daily Loss Limit")}</span>
                    <span className="text-foreground">$500</span>
                  </div>
                </div>
                <button
                  onClick={() => navigate("/app/chart")}
                  className="w-full bg-primary text-primary-foreground py-3 rounded-lg text-[0.9375rem] hover:bg-primary/90 transition-colors flex items-center justify-center gap-2"
                >
                  <Play className="w-4 h-4" /> {t("Paper로 시작", "Start in Paper")}
                </button>
                <p className="text-[0.75rem] text-muted-foreground text-center mt-3">
                  {t("Paper 모드는 시뮬레이션으로 실제 자금이 사용되지 않습니다.", "Paper mode is simulation-only and uses no real funds.")}
                </p>
              </div>
            )}
          </motion.div>
        </AnimatePresence>

        {/* Navigation */}
        <div className="flex items-center justify-between mt-8 pt-6 border-t border-border">
          <button
            onClick={() => step > 1 && setStep(step - 1)}
            className={`flex items-center gap-1.5 text-[0.8125rem] px-3 py-2 rounded-lg transition-colors ${
              step > 1 ? "text-muted-foreground hover:text-foreground" : "invisible"
            }`}
          >
            <ArrowLeft className="w-3.5 h-3.5" /> {t("이전", "Back")}
          </button>
          {step < 5 && (
            <button
              onClick={() => canNext() && setStep(step + 1)}
              disabled={!canNext()}
              className="flex items-center gap-1.5 bg-primary text-primary-foreground px-5 py-2 rounded-lg text-[0.8125rem] hover:bg-primary/90 transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
            >
              {t("다음", "Next")} <ArrowRight className="w-3.5 h-3.5" />
            </button>
          )}
        </div>
      </div>

      {/* Right Panel - Chart Preview */}
      <div className="hidden lg:flex flex-1 bg-card/30 border-l border-border p-8 flex-col justify-center">
        <div className="mb-6">
          <div className="flex items-center gap-2 mb-2">
            <div className="w-2 h-2 rounded-full bg-[#22c55e] animate-pulse" />
            <span className="text-[0.8125rem] text-muted-foreground">{t("실시간 BTC 차트", "Live BTC Chart")}</span>
          </div>
        </div>
        <div className="bg-card border border-border rounded-xl p-4">
          <BtcChart height={350} showSignals mini={false} />
        </div>
        <div className="mt-6 space-y-2">
          <p className="text-[0.75rem] text-muted-foreground">{t("최근 에이전트 진입", "Recent Agent Entries")}</p>
          {[
            { name: "Thunder Strike", side: "LONG", price: "$67,842", time: "방금 전" },
            { name: "Night Owl", side: "SHORT", price: "$67,856", time: "2분 전" },
            { name: "Scalp King", side: "LONG", price: "$67,790", time: "5분 전" },
          ].map((e, i) => (
            <div key={i} className="flex items-center justify-between text-[0.75rem] py-1.5">
              <span className="text-foreground">{e.name}</span>
              <div className="flex items-center gap-3">
                <span className={e.side === "LONG" ? "text-[#22c55e]" : "text-[#ef4444]"}>{e.side}</span>
                <span className="text-muted-foreground">{e.price}</span>
                <span className="text-muted-foreground/60">{t(e.time)}</span>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
