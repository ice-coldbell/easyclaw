import { useCallback } from "react";
import { CheckCircle2, Sparkles, Wallet } from "lucide-react";
import { useNavigate } from "react-router";
import { useI18n } from "../i18n";
import { formatWalletAddress, useWallet } from "../lib/wallet";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "./ui/dialog";

const RECOMMENDED_WALLET_IDS = new Set(["phantom", "solflare", "backpack"]);

export function WalletConnectButton() {
  const { t } = useI18n();
  const navigate = useNavigate();
  const {
    walletPubkey,
    walletInstalled,
    walletConnected,
    connecting,
    error,
    walletOptions,
    selectedWalletID,
    connectWalletByID,
    disconnectWallet,
    walletPickerOpen,
    walletPickerTargetPath,
    openWalletPicker,
    closeWalletPicker,
  } = useWallet();

  const handleWalletSelect = useCallback(async (walletID: string) => {
    const targetPath = walletPickerTargetPath;
    const connected = await connectWalletByID(walletID);

    if (!connected) {
      return;
    }

    closeWalletPicker();
    if (targetPath) {
      navigate(targetPath);
    }
  }, [closeWalletPicker, connectWalletByID, navigate, walletPickerTargetPath]);

  if (walletConnected && walletPubkey) {
    return (
      <div className="flex items-center gap-2">
        <div className="rounded-lg border border-border bg-card px-3 py-1.5 text-[0.75rem] text-foreground" style={{ fontFamily: "'JetBrains Mono', monospace" }}>
          {formatWalletAddress(walletPubkey)}
        </div>
        <button
          type="button"
          onClick={() => void disconnectWallet()}
          disabled={connecting}
          className="rounded-lg border border-border px-3 py-1.5 text-[0.75rem] text-muted-foreground transition-colors hover:text-foreground disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {connecting ? t("연결 해제 중...", "Disconnecting...") : t("연결 해제", "Disconnect")}
        </button>
      </div>
    );
  }

  return (
    <>
      <div className="flex flex-col items-end gap-1">
        <button
          type="button"
          onClick={() => openWalletPicker()}
          disabled={connecting}
          className="rounded-lg border border-primary/35 bg-primary/10 px-3 py-1.5 text-[0.75rem] text-primary transition-colors hover:bg-primary/15 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1.5"
        >
          <Wallet className="w-3.5 h-3.5" />
          {connecting ? t("연결 중...", "Connecting...") : t("월렛 연결", "Connect Wallet")}
        </button>
        {!walletInstalled && (
          <span className="text-[0.6875rem] text-muted-foreground">{t("Phantom/Solflare 지갑 필요", "Phantom/Solflare wallet required")}</span>
        )}
        {error && (
          <span className="text-[0.6875rem] text-destructive max-w-64 text-right">{error}</span>
        )}
      </div>

      <Dialog
        open={walletPickerOpen}
        onOpenChange={(open) => {
          if (!open) {
            closeWalletPicker();
          }
        }}
      >
        <DialogContent className="sm:max-w-md border-border/80 bg-card/95 backdrop-blur-xl p-0 overflow-hidden">
          <div className="border-b border-border/70 px-5 pt-5 pb-4">
            <DialogTitle className="text-[1rem] font-semibold">
              {t("지갑 선택", "Choose Wallet")}
            </DialogTitle>
            <DialogDescription className="text-[0.75rem] mt-1">
              {t("사용할 Solana 지갑을 선택하세요.", "Select the Solana wallet you want to use.")}
            </DialogDescription>
          </div>

          <div className="p-3 max-h-80 overflow-y-auto space-y-2">
            {walletOptions.length === 0 && (
              <div className="rounded-xl border border-dashed border-border px-4 py-5 text-[0.75rem] text-muted-foreground text-center">
                {t("감지된 지갑이 없습니다. Phantom 또는 Solflare 확장 프로그램을 설치해주세요.", "No wallet detected. Install Phantom or Solflare browser extension.")}
              </div>
            )}

            {walletOptions.map((wallet) => {
              const isSelected = wallet.id === selectedWalletID;
              const isRecommended = RECOMMENDED_WALLET_IDS.has(wallet.id);

              return (
                <button
                  key={wallet.id}
                  type="button"
                  onClick={() => void handleWalletSelect(wallet.id)}
                  disabled={connecting}
                  className={`w-full rounded-xl border px-3 py-2.5 text-left transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${
                    isSelected
                      ? "border-primary/45 bg-primary/8"
                      : "border-border bg-background/60 hover:border-primary/30 hover:bg-primary/5"
                  }`}
                >
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <div className="h-7 w-7 rounded-lg bg-primary/12 text-primary text-[0.6875rem] font-semibold flex items-center justify-center">
                        {wallet.label.slice(0, 2).toUpperCase()}
                      </div>
                      <div>
                        <p className="text-[0.8125rem] text-foreground">{wallet.label}</p>
                        <p className="text-[0.6875rem] text-muted-foreground">
                          {isSelected ? t("최근 선택됨", "Last used") : t("클릭하여 연결", "Click to connect")}
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center gap-1.5">
                      {isRecommended && (
                        <span className="inline-flex items-center gap-1 rounded-full border border-primary/30 bg-primary/10 px-2 py-0.5 text-[0.625rem] text-primary">
                          <Sparkles className="h-3 w-3" />
                          {t("권장", "Recommended")}
                        </span>
                      )}
                      {isSelected && <CheckCircle2 className="h-4 w-4 text-primary" />}
                    </div>
                  </div>
                </button>
              );
            })}
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
