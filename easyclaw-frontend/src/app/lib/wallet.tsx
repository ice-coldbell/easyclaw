import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

const AUTO_CONNECT_KEY = "easyclaw_wallet_auto_connect";
const PREFERRED_WALLET_KEY = "easyclaw_wallet_provider";
const WALLET_DISCOVERY_INTERVAL_MS = 1000;

interface SolanaPublicKeyLike {
  toBase58?: () => string;
  toString: () => string;
}

interface SolanaConnectResponse {
  publicKey?: unknown;
}

interface SolanaWalletProvider {
  publicKey?: SolanaPublicKeyLike | null;
  providers?: unknown[];
  isPhantom?: boolean;
  isSolflare?: boolean;
  isBackpack?: boolean;
  connect: (options?: { onlyIfTrusted?: boolean }) => Promise<SolanaConnectResponse>;
  disconnect?: () => Promise<void>;
  on?: (
    event: "connect" | "disconnect" | "accountChanged",
    listener: (publicKey?: unknown) => void,
  ) => void;
  off?: (
    event: "connect" | "disconnect" | "accountChanged",
    listener: (publicKey?: unknown) => void,
  ) => void;
}

interface WalletOptionInternal {
  id: string;
  label: string;
  provider: SolanaWalletProvider;
}

interface WalletOption {
  id: string;
  label: string;
}

declare global {
  interface Window {
    solana?: SolanaWalletProvider;
    phantom?: {
      solana?: SolanaWalletProvider;
    };
    solflare?: SolanaWalletProvider | { solana?: SolanaWalletProvider };
    backpack?: SolanaWalletProvider | { solana?: SolanaWalletProvider };
  }
}

interface WalletContextValue {
  walletPubkey: string | null;
  walletInstalled: boolean;
  walletConnected: boolean;
  connecting: boolean;
  error: string | null;
  walletOptions: WalletOption[];
  selectedWalletID: string | null;
  setSelectedWalletID: (walletID: string) => void;
  connectWallet: () => Promise<boolean>;
  connectWalletByID: (walletID: string) => Promise<boolean>;
  disconnectWallet: () => Promise<void>;
  walletPickerOpen: boolean;
  walletPickerTargetPath: string | null;
  openWalletPicker: (targetPath?: string) => void;
  closeWalletPicker: () => void;
  clearError: () => void;
}

const WalletContext = createContext<WalletContextValue | undefined>(undefined);

const BASE58_ADDRESS_REGEX = /^[1-9A-HJ-NP-Za-km-z]{32,44}$/;

function normalizeBase58Address(value: string): string | null {
  const trimmed = value.trim();
  if (trimmed.length === 0) {
    return null;
  }
  if (trimmed === "[object Object]") {
    return null;
  }
  if (!BASE58_ADDRESS_REGEX.test(trimmed)) {
    return null;
  }
  return trimmed;
}

function toBase58(publicKey: unknown, depth = 0): string | null {
  if (!publicKey || depth > 5) {
    return null;
  }

  if (typeof publicKey === "string") {
    return normalizeBase58Address(publicKey);
  }

  if (typeof publicKey !== "object") {
    return null;
  }

  const record = publicKey as Record<string, unknown>;

  if (typeof record.address === "string") {
    const address = normalizeBase58Address(record.address);
    if (address) {
      return address;
    }
  }

  if ("publicKey" in record) {
    const nested = toBase58(record.publicKey, depth + 1);
    if (nested) {
      return nested;
    }
  }

  if (typeof (publicKey as SolanaPublicKeyLike).toBase58 === "function") {
    try {
      const value = (publicKey as SolanaPublicKeyLike).toBase58?.();
      const address = typeof value === "string" ? normalizeBase58Address(value) : null;
      if (address) {
        return address;
      }
    } catch {
      // Ignore provider-specific serialization failures.
    }
  }

  if (typeof (publicKey as SolanaPublicKeyLike).toString === "function") {
    try {
      const value = (publicKey as SolanaPublicKeyLike).toString();
      const address = normalizeBase58Address(value);
      if (address) {
        return address;
      }
    } catch {
      // Ignore provider-specific serialization failures.
    }
  }

  return null;
}

function isWalletProvider(candidate: unknown): candidate is SolanaWalletProvider {
  return typeof candidate === "object" && candidate !== null && typeof (candidate as SolanaWalletProvider).connect === "function";
}

function unwrapWalletProvider(candidate: unknown): SolanaWalletProvider | null {
  if (isWalletProvider(candidate)) {
    return candidate;
  }

  if (typeof candidate === "object" && candidate !== null && "solana" in candidate) {
    const nested = (candidate as { solana?: unknown }).solana;
    if (isWalletProvider(nested)) {
      return nested;
    }
  }

  return null;
}

function pushWalletOption(
  options: WalletOptionInternal[],
  seenProviders: Set<SolanaWalletProvider>,
  id: string,
  label: string,
  candidate: unknown,
): void {
  const provider = unwrapWalletProvider(candidate);
  if (!provider || seenProviders.has(provider)) {
    return;
  }

  options.push({ id, label, provider });
  seenProviders.add(provider);
}

function walletIdentityFromProvider(provider: SolanaWalletProvider, index: number): { id: string; label: string } {
  if (provider.isPhantom) {
    return { id: "phantom", label: "Phantom" };
  }
  if (provider.isSolflare) {
    return { id: "solflare", label: "Solflare" };
  }
  if (provider.isBackpack) {
    return { id: "backpack", label: "Backpack" };
  }
  return { id: `injected-${index + 1}`, label: `Injected Wallet ${index + 1}` };
}

function detectWalletOptions(): WalletOptionInternal[] {
  if (typeof window === "undefined") {
    return [];
  }

  const options: WalletOptionInternal[] = [];
  const seenProviders = new Set<SolanaWalletProvider>();

  pushWalletOption(options, seenProviders, "phantom", "Phantom", window.phantom?.solana);
  pushWalletOption(options, seenProviders, "solflare", "Solflare", window.solflare);
  pushWalletOption(options, seenProviders, "backpack", "Backpack", window.backpack);

  const globalProvider = unwrapWalletProvider(window.solana);
  const providerPool = Array.isArray(globalProvider?.providers) ? globalProvider.providers : [];

  providerPool.forEach((candidate, index) => {
    const provider = unwrapWalletProvider(candidate);
    if (!provider) {
      return;
    }
    const identity = walletIdentityFromProvider(provider, index);
    pushWalletOption(options, seenProviders, identity.id, identity.label, provider);
  });

  if (options.length === 0 && globalProvider) {
    const identity = walletIdentityFromProvider(globalProvider, 0);
    pushWalletOption(options, seenProviders, identity.id, identity.label, globalProvider);
  }

  return options;
}

function getSavedWalletID(): string | null {
  if (typeof window === "undefined") {
    return null;
  }

  const saved = window.localStorage.getItem(PREFERRED_WALLET_KEY);
  return saved && saved.trim().length > 0 ? saved : null;
}

function pickPreferredWalletID(options: WalletOptionInternal[], candidateWalletID: string | null): string | null {
  if (candidateWalletID && options.some((option) => option.id === candidateWalletID)) {
    return candidateWalletID;
  }

  const priorities = ["phantom", "solflare", "backpack"];
  for (const walletID of priorities) {
    if (options.some((option) => option.id === walletID)) {
      return walletID;
    }
  }

  return options[0]?.id ?? null;
}

function walletOptionsEqual(a: WalletOptionInternal[], b: WalletOptionInternal[]): boolean {
  if (a.length !== b.length) {
    return false;
  }

  for (let index = 0; index < a.length; index += 1) {
    if (a[index].id !== b[index].id || a[index].provider !== b[index].provider) {
      return false;
    }
  }

  return true;
}

export function WalletProvider({ children }: { children: ReactNode }) {
  const [walletPubkey, setWalletPubkey] = useState<string | null>(null);
  const [walletOptions, setWalletOptions] = useState<WalletOptionInternal[]>([]);
  const [selectedWalletID, setSelectedWalletIDState] = useState<string | null>(getSavedWalletID);
  const [connecting, setConnecting] = useState(false);
  const [walletPickerOpen, setWalletPickerOpen] = useState(false);
  const [walletPickerTargetPath, setWalletPickerTargetPath] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (typeof window === "undefined") {
      return undefined;
    }

    const refreshWalletOptions = () => {
      const discovered = detectWalletOptions();
      setWalletOptions((current) => (walletOptionsEqual(current, discovered) ? current : discovered));

      setSelectedWalletIDState((currentID) => {
        const nextID = pickPreferredWalletID(discovered, currentID ?? getSavedWalletID());

        if (nextID !== currentID) {
          if (nextID) {
            window.localStorage.setItem(PREFERRED_WALLET_KEY, nextID);
          } else {
            window.localStorage.removeItem(PREFERRED_WALLET_KEY);
          }
        }

        return nextID;
      });
    };

    refreshWalletOptions();
    const timerID = window.setInterval(refreshWalletOptions, WALLET_DISCOVERY_INTERVAL_MS);

    return () => {
      window.clearInterval(timerID);
    };
  }, []);

  const selectedWalletOption = useMemo(
    () => walletOptions.find((option) => option.id === selectedWalletID) ?? null,
    [selectedWalletID, walletOptions],
  );

  useEffect(() => {
    const provider = selectedWalletOption?.provider ?? null;
    if (!provider) {
      setWalletPubkey(null);
      return undefined;
    }

    setWalletPubkey(toBase58(provider.publicKey));

    const handleConnect = (publicKey?: unknown) => {
      setWalletPubkey(toBase58(publicKey ?? provider.publicKey));
      setError(null);
    };

    const handleDisconnect = () => {
      setWalletPubkey(null);
      if (typeof window !== "undefined") {
        window.localStorage.removeItem(AUTO_CONNECT_KEY);
      }
    };

    const handleAccountChanged = (publicKey?: unknown) => {
      setWalletPubkey(toBase58(publicKey));
    };

    provider.on?.("connect", handleConnect);
    provider.on?.("disconnect", handleDisconnect);
    provider.on?.("accountChanged", handleAccountChanged);

    if (typeof window !== "undefined" && window.localStorage.getItem(AUTO_CONNECT_KEY) === "1") {
      void provider.connect({ onlyIfTrusted: true }).catch(() => {
        // Ignore trusted auto-connect failures. The user can reconnect manually.
      });
    }

    return () => {
      provider.off?.("connect", handleConnect);
      provider.off?.("disconnect", handleDisconnect);
      provider.off?.("accountChanged", handleAccountChanged);
    };
  }, [selectedWalletOption]);

  const setSelectedWalletID = useCallback((walletID: string) => {
    if (!walletOptions.some((option) => option.id === walletID)) {
      return;
    }

    setSelectedWalletIDState(walletID);
    if (typeof window !== "undefined") {
      window.localStorage.setItem(PREFERRED_WALLET_KEY, walletID);
    }
    setError(null);
  }, [walletOptions]);

  const connectWalletWithOption = useCallback(async (option: WalletOptionInternal | null): Promise<boolean> => {
    if (connecting) {
      return false;
    }
    if (!option) {
      setError("No compatible Solana wallet found.");
      return false;
    }

    try {
      setConnecting(true);
      setError(null);
      const response = await option.provider.connect();
      const address = toBase58(response.publicKey ?? option.provider.publicKey);
      if (!address) {
        throw new Error("Wallet returned no public key.");
      }

      setWalletPubkey(address);
      setSelectedWalletIDState(option.id);
      if (typeof window !== "undefined") {
        window.localStorage.setItem(AUTO_CONNECT_KEY, "1");
        window.localStorage.setItem(PREFERRED_WALLET_KEY, option.id);
      }
      return true;
    } catch (connectError) {
      const message = connectError instanceof Error ? connectError.message : String(connectError);
      setError(message);
      return false;
    } finally {
      setConnecting(false);
    }
  }, [connecting]);

  const connectWallet = useCallback(async (): Promise<boolean> => {
    return connectWalletWithOption(selectedWalletOption);
  }, [connectWalletWithOption, selectedWalletOption]);

  const connectWalletByID = useCallback(async (walletID: string): Promise<boolean> => {
    const option = walletOptions.find((candidate) => candidate.id === walletID) ?? null;
    return connectWalletWithOption(option);
  }, [connectWalletWithOption, walletOptions]);

  const disconnectWallet = useCallback(async () => {
    const selected = selectedWalletOption;

    try {
      setConnecting(true);
      setError(null);
      if (selected?.provider.disconnect) {
        await selected.provider.disconnect();
      }
    } catch (disconnectError) {
      const message = disconnectError instanceof Error ? disconnectError.message : String(disconnectError);
      setError(message);
    } finally {
      if (typeof window !== "undefined") {
        window.localStorage.removeItem(AUTO_CONNECT_KEY);
      }
      setWalletPubkey(null);
      setConnecting(false);
    }
  }, [selectedWalletOption]);

  const clearError = useCallback(() => setError(null), []);

  const openWalletPicker = useCallback((targetPath?: string) => {
    setWalletPickerTargetPath(targetPath ?? null);
    setWalletPickerOpen(true);
    setError(null);
  }, []);

  const closeWalletPicker = useCallback(() => {
    setWalletPickerOpen(false);
    setWalletPickerTargetPath(null);
  }, []);

  const value = useMemo<WalletContextValue>(
    () => ({
      walletPubkey,
      walletInstalled: walletOptions.length > 0,
      walletConnected: Boolean(walletPubkey),
      connecting,
      error,
      walletOptions: walletOptions.map((option) => ({ id: option.id, label: option.label })),
      selectedWalletID,
      setSelectedWalletID,
      connectWallet,
      connectWalletByID,
      disconnectWallet,
      walletPickerOpen,
      walletPickerTargetPath,
      openWalletPicker,
      closeWalletPicker,
      clearError,
    }),
    [
      closeWalletPicker,
      clearError,
      connectWallet,
      connectWalletByID,
      connecting,
      disconnectWallet,
      error,
      openWalletPicker,
      selectedWalletID,
      setSelectedWalletID,
      walletOptions,
      walletPickerOpen,
      walletPickerTargetPath,
      walletPubkey,
    ],
  );

  return <WalletContext.Provider value={value}>{children}</WalletContext.Provider>;
}

export function useWallet(): WalletContextValue {
  const context = useContext(WalletContext);
  if (!context) {
    throw new Error("useWallet must be used within WalletProvider");
  }
  return context;
}

export function formatWalletAddress(address: string): string {
  if (address.length <= 12) {
    return address;
  }
  return `${address.slice(0, 6)}...${address.slice(-4)}`;
}
