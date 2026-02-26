import "dotenv/config";
import fs from "node:fs";
import path from "node:path";
import * as anchor from "@coral-xyz/anchor";
import { BALANCED_COMMON, MARKETS } from "./defaults";

const TOKEN_PROGRAM_ID = new anchor.web3.PublicKey(
  "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"
);

function toPublicKey(value: string): anchor.web3.PublicKey {
  return new anchor.web3.PublicKey(value.trim());
}

type EnvPubkeyOptions = {
  allowHex32?: boolean;
  invalidAsNull?: boolean;
};

function parsePubkeyValue(
  name: string,
  raw: string,
  options?: EnvPubkeyOptions
): anchor.web3.PublicKey {
  const normalized = raw.trim();
  if (normalized.length === 0) {
    throw new Error(`Empty pubkey env: ${name}`);
  }

  try {
    return toPublicKey(normalized);
  } catch (_base58Error) {
    const allowHex32 = options?.allowHex32 ?? false;
    if (allowHex32 && /^0x[0-9a-fA-F]{64}$/.test(normalized)) {
      const bytes = Buffer.from(normalized.slice(2), "hex");
      const converted = new anchor.web3.PublicKey(bytes);
      console.warn(
        `[warn] ${name}: hex(0x...) value detected, converted to base58 ${converted.toBase58()}`
      );
      return converted;
    }

    const formatHint = allowHex32 ? "base58 or 0x + 64 hex chars" : "base58";
    throw new Error(
      `Invalid pubkey env ${name}: ${raw} (expected ${formatHint})`
    );
  }
}

function getEnvPubkey(
  name: string,
  options?: EnvPubkeyOptions
): anchor.web3.PublicKey | null {
  const value = process.env[name];
  if (!value || value.trim().length === 0) {
    return null;
  }

  try {
    return parsePubkeyValue(name, value, options);
  } catch (error) {
    if (options?.invalidAsNull) {
      const message =
        error instanceof Error ? error.message : JSON.stringify(error);
      console.warn(`[warn] ${message}. fallback will be used.`);
      return null;
    }
    throw error;
  }
}

function getRequiredEnvPubkey(name: string): anchor.web3.PublicKey {
  const value = getEnvPubkey(name);
  if (!value) {
    throw new Error(`Missing required env: ${name}`);
  }
  return value;
}

function getEnvNumber(name: string, fallback: number): number {
  const raw = process.env[name];
  if (!raw || raw.trim().length === 0) {
    return fallback;
  }
  const parsed = Number(raw);
  if (!Number.isFinite(parsed)) {
    throw new Error(`Invalid numeric env ${name}: ${raw}`);
  }
  return parsed;
}

function getEnvBoolean(name: string, fallback: boolean): boolean {
  const raw = process.env[name];
  if (!raw || raw.trim().length === 0) {
    return fallback;
  }
  const normalized = raw.trim().toLowerCase();
  if (["1", "true", "yes", "on"].includes(normalized)) {
    return true;
  }
  if (["0", "false", "no", "off"].includes(normalized)) {
    return false;
  }
  throw new Error(`Invalid boolean env ${name}: ${raw}`);
}

function normalizeIdl(rawIdl: any): any {
  const idl = { ...rawIdl };
  const typeByName = new Map<string, any>(
    (idl.types ?? []).map((item: any) => [item.name, item.type])
  );

  if (Array.isArray(idl.accounts)) {
    idl.accounts = idl.accounts.map((account: any) =>
      account.type ? account : { ...account, type: typeByName.get(account.name) }
    );
  }

  return idl;
}

function readIdlFromCandidates(programName: string): any {
  const upper = programName.toUpperCase();
  const fromEnv = process.env[`${upper}_IDL_PATH`]?.trim();
  const candidates = [
    fromEnv,
    path.resolve(process.cwd(), `idl/${programName}.json`),
    path.resolve(process.cwd(), `target/idl/${programName}.json`)
  ].filter((value): value is string => Boolean(value && value.length > 0));

  for (const candidate of candidates) {
    if (fs.existsSync(candidate)) {
      return JSON.parse(fs.readFileSync(candidate, "utf8"));
    }
  }

  throw new Error(
    `${programName} IDL not found. Run \`anchor build\` first or set ${upper}_IDL_PATH.`
  );
}

function toBn(value: number | bigint): anchor.BN {
  if (typeof value === "bigint") {
    return new anchor.BN(value.toString());
  }
  return new anchor.BN(value);
}

function u64Seed(value: number | bigint): Buffer {
  return toBn(value).toArrayLike(Buffer, "le", 8);
}

function parseKeepers(
  defaultKeeper: anchor.web3.PublicKey
): anchor.web3.PublicKey[] {
  const raw = process.env.KEEPERS?.trim();
  if (!raw) {
    return [defaultKeeper];
  }
  const keepers = raw
    .split(",")
    .map((item) => item.trim())
    .filter((item) => item.length > 0)
    .map((item) => toPublicKey(item));

  return keepers.length > 0 ? keepers : [defaultKeeper];
}

function parseBootstrapMarkets(): Array<(typeof MARKETS)[number]> {
  const raw = process.env.BOOTSTRAP_MARKET_IDS?.trim();
  if (!raw) {
    return [...MARKETS];
  }

  const requestedMarketIds = new Set<number>(
    raw
      .split(",")
      .map((item) => item.trim())
      .filter((item) => item.length > 0)
      .map((item) => {
        const parsed = Number(item);
        if (!Number.isInteger(parsed) || parsed <= 0) {
          throw new Error(`Invalid BOOTSTRAP_MARKET_IDS value: ${item}`);
        }
        return parsed;
      })
  );

  if (requestedMarketIds.size === 0) {
    throw new Error("BOOTSTRAP_MARKET_IDS is empty");
  }

  const selectedMarkets = MARKETS.filter((market) =>
    requestedMarketIds.has(market.marketId)
  );
  const missingMarketIds = Array.from(requestedMarketIds).filter(
    (requestedMarketId) =>
      !selectedMarkets.some((selected) => selected.marketId === requestedMarketId)
  );

  if (missingMarketIds.length > 0) {
    throw new Error(
      `Unknown market ids in BOOTSTRAP_MARKET_IDS: ${missingMarketIds.join(",")}`
    );
  }

  return selectedMarkets;
}

async function accountExists(
  connection: anchor.web3.Connection,
  pubkey: anchor.web3.PublicKey
): Promise<boolean> {
  return (await connection.getAccountInfo(pubkey)) !== null;
}

async function callRpc(
  program: any,
  methodName: string,
  args: any[],
  accounts: Record<string, anchor.web3.PublicKey>,
  label: string
): Promise<void> {
  const toSnake = (value: string) =>
    value.replace(/[A-Z]/g, (match) => `_${match.toLowerCase()}`);
  const toCamel = (value: string) =>
    value.replace(/_([a-z])/g, (_, letter: string) => letter.toUpperCase());

  const accountAliases: Record<string, anchor.web3.PublicKey> = {};
  for (const [key, value] of Object.entries(accounts)) {
    accountAliases[key] = value;
    accountAliases[toSnake(key)] = value;
    accountAliases[toCamel(key)] = value;
  }

  const methodCandidates = Array.from(
    new Set([methodName, toCamel(methodName), toSnake(methodName)])
  );
  const method = methodCandidates
    .map((name) => program.methods?.[name])
    .find((candidate) => typeof candidate === "function");

  if (typeof method !== "function") {
    throw new Error(`Method not found: ${methodName}`);
  }

  const sig = await method(...args).accounts(accountAliases).rpc();
  console.log(`[ok] ${label}: ${sig}`);
}

function deriveMarketPda(
  marketRegistryProgramId: anchor.web3.PublicKey,
  marketId: number | bigint
): anchor.web3.PublicKey {
  return anchor.web3.PublicKey.findProgramAddressSync(
    [Buffer.from("market"), u64Seed(marketId)],
    marketRegistryProgramId
  )[0];
}

function deriveFundingPda(
  orderEngineProgramId: anchor.web3.PublicKey,
  marketId: number | bigint
): anchor.web3.PublicKey {
  return anchor.web3.PublicKey.findProgramAddressSync(
    [Buffer.from("funding"), u64Seed(marketId)],
    orderEngineProgramId
  )[0];
}

async function main() {
  const provider = anchor.AnchorProvider.env();
  anchor.setProvider(provider);

  const wallet = provider.wallet.publicKey;
  const connection = provider.connection;

  const marketRegistryIdl = normalizeIdl(readIdlFromCandidates("market_registry"));
  const lpVaultIdl = normalizeIdl(readIdlFromCandidates("lp_vault"));
  const orderEngineIdl = normalizeIdl(readIdlFromCandidates("order_engine"));

  const marketRegistryProgramId =
    getEnvPubkey("MARKET_REGISTRY_PROGRAM_ID") ??
    toPublicKey((marketRegistryIdl as any).address);
  const lpVaultProgramId =
    getEnvPubkey("LP_VAULT_PROGRAM_ID") ??
    toPublicKey((lpVaultIdl as any).address);
  const orderEngineProgramId =
    getEnvPubkey("ORDER_ENGINE_PROGRAM_ID") ??
    toPublicKey((orderEngineIdl as any).address);

  const multisig = getEnvPubkey("MULTISIG") ?? wallet;
  if (!multisig.equals(wallet)) {
    throw new Error(
      "This bootstrap signer flow requires MULTISIG to equal provider wallet pubkey"
    );
  }

  const usdcMint = getRequiredEnvPubkey("USDC_MINT");
  const keepers = parseKeepers(wallet);
  const bootstrapMarkets = parseBootstrapMarkets();

  const feeSplit = {
    lpBps: getEnvNumber("GLOBAL_LP_BPS", 7000),
    insuranceBps: getEnvNumber("GLOBAL_INSURANCE_BPS", 2000),
    protocolBps: getEnvNumber("GLOBAL_PROTOCOL_BPS", 1000)
  };
  if (
    feeSplit.lpBps + feeSplit.insuranceBps + feeSplit.protocolBps !==
    10_000
  ) {
    throw new Error("GLOBAL_*_BPS sum must be 10000");
  }
  const pauseFlags = getEnvBoolean("GLOBAL_PAUSE", false);

  const cooldownSecs = getEnvNumber("LP_COOLDOWN_SECS", 86_400);
  const minLiquidityBufferBps = getEnvNumber("LP_MIN_BUFFER_BPS", 100);

  const maxTtlSecs = getEnvNumber("ENGINE_MAX_TTL_SECS", 300);
  const liquidationPenaltyBps = getEnvNumber(
    "ENGINE_LIQUIDATION_PENALTY_BPS",
    1_000
  );
  const maxImrBps = getEnvNumber("ENGINE_MAX_IMR_BPS", 5_000);

  const marketRegistryProgram = new (anchor.Program as any)(
    {
      ...(marketRegistryIdl as any),
      address: marketRegistryProgramId.toBase58()
    } as anchor.Idl,
    provider
  );
  const lpVaultProgram = new (anchor.Program as any)(
    {
      ...(lpVaultIdl as any),
      address: lpVaultProgramId.toBase58()
    } as anchor.Idl,
    provider
  );
  const orderEngineProgram = new (anchor.Program as any)(
    {
      ...(orderEngineIdl as any),
      address: orderEngineProgramId.toBase58()
    } as anchor.Idl,
    provider
  );

  const globalConfig = anchor.web3.PublicKey.findProgramAddressSync(
    [Buffer.from("global-config")],
    marketRegistryProgramId
  )[0];
  const keeperSet = anchor.web3.PublicKey.findProgramAddressSync(
    [Buffer.from("keeper-set")],
    marketRegistryProgramId
  )[0];

  const engineAuthority = anchor.web3.PublicKey.findProgramAddressSync(
    [Buffer.from("engine-authority")],
    orderEngineProgramId
  )[0];
  const engineConfig = anchor.web3.PublicKey.findProgramAddressSync(
    [Buffer.from("engine-config")],
    orderEngineProgramId
  )[0];
  const collateralVault = anchor.web3.PublicKey.findProgramAddressSync(
    [Buffer.from("collateral-vault")],
    orderEngineProgramId
  )[0];

  const pool = anchor.web3.PublicKey.findProgramAddressSync(
    [Buffer.from("pool"), usdcMint.toBuffer()],
    lpVaultProgramId
  )[0];
  const liquidityAuth = anchor.web3.PublicKey.findProgramAddressSync(
    [Buffer.from("liquidity-auth"), pool.toBuffer()],
    lpVaultProgramId
  )[0];
  const insuranceAuth = anchor.web3.PublicKey.findProgramAddressSync(
    [Buffer.from("insurance-auth"), pool.toBuffer()],
    lpVaultProgramId
  )[0];
  const protocolFeeAuth = anchor.web3.PublicKey.findProgramAddressSync(
    [Buffer.from("protocol-fee-auth"), pool.toBuffer()],
    lpVaultProgramId
  )[0];
  const liquidityVault = anchor.web3.PublicKey.findProgramAddressSync(
    [Buffer.from("liquidity-vault"), pool.toBuffer()],
    lpVaultProgramId
  )[0];
  const insuranceVault = anchor.web3.PublicKey.findProgramAddressSync(
    [Buffer.from("insurance-vault"), pool.toBuffer()],
    lpVaultProgramId
  )[0];
  const protocolFeeVault = anchor.web3.PublicKey.findProgramAddressSync(
    [Buffer.from("protocol-fee-vault"), pool.toBuffer()],
    lpVaultProgramId
  )[0];

  console.log("bootstrap signer:", wallet.toBase58());
  console.log("market_registry:", marketRegistryProgramId.toBase58());
  console.log("lp_vault:", lpVaultProgramId.toBase58());
  console.log("order_engine:", orderEngineProgramId.toBase58());
  console.log("usdc_mint:", usdcMint.toBase58());
  console.log(
    "bootstrap_markets:",
    bootstrapMarkets.map((market) => `${market.marketId}:${market.symbol}`).join(", ")
  );

  if (!(await accountExists(connection, globalConfig))) {
    await callRpc(
      marketRegistryProgram,
      "initializeGlobal",
      [multisig, feeSplit, pauseFlags],
      {
        payer: wallet,
        globalConfig,
        keeperSet,
        systemProgram: anchor.web3.SystemProgram.programId
      },
      "market_registry.initialize_global"
    );
  } else {
    console.log("[skip] market_registry.initialize_global (already initialized)");
  }

  for (const keeper of keepers) {
    try {
      await callRpc(
        marketRegistryProgram,
        "addKeeper",
        [keeper],
        {
          authority: wallet,
          globalConfig,
          keeperSet
        },
        `market_registry.add_keeper(${keeper.toBase58()})`
      );
    } catch (error) {
      const message =
        error instanceof Error ? error.message : JSON.stringify(error);
      if (
        message.includes("KeeperAlreadyExists") ||
        message.includes("already in use")
      ) {
        console.log(`[skip] keeper exists: ${keeper.toBase58()}`);
      } else {
        throw error;
      }
    }
  }

  const pythFeedFallback =
    getEnvPubkey("PYTH_FEED_FALLBACK", {
      allowHex32: true,
      invalidAsNull: true
    }) ?? wallet;
  for (const market of bootstrapMarkets) {
    const marketId = toBn(market.marketId);
    const marketPda = deriveMarketPda(marketRegistryProgramId, market.marketId);
    const envKey = `PYTH_FEED_${market.symbol.replace(/-/g, "_").toUpperCase()}`;
    const pythFeed =
      getEnvPubkey(envKey, { allowHex32: true, invalidAsNull: true }) ??
      pythFeedFallback;

    if (!(await accountExists(connection, marketPda))) {
      await callRpc(
        marketRegistryProgram,
        "createMarket",
        [
          marketId,
          market.symbol,
          pythFeed,
          {
            maxLeverage: market.risk.maxLeverage,
            imrBps: market.risk.imrBps,
            mmrBps: market.risk.mmrBps,
            oiCap: toBn(market.risk.oiCap),
            skewCap: toBn(market.risk.skewCap),
            maxTradeNotional: toBn(market.risk.maxTradeNotional)
          },
          {
            baseSpreadBps: market.pricing.baseSpreadBps,
            skewCoeffBps: market.pricing.skewCoeffBps,
            maxFillDeviationBps: market.pricing.maxFillDeviationBps,
            maxOracleStalenessSec: toBn(market.pricing.maxOracleStalenessSec),
            maxConfBps: market.pricing.maxConfBps
          },
          {
            intervalSec: toBn(BALANCED_COMMON.funding.intervalSec),
            fundingVelocityCapBpsPerDay: toBn(
              BALANCED_COMMON.funding.fundingVelocityCapBpsPerDay
            ),
            premiumClampBps: toBn(BALANCED_COMMON.funding.premiumClampBps)
          },
          {
            takerFeeBps: BALANCED_COMMON.fee.takerFeeBps,
            makerFeeBps: BALANCED_COMMON.fee.makerFeeBps
          }
        ],
        {
          authority: wallet,
          globalConfig,
          keeperSet,
          market: marketPda,
          systemProgram: anchor.web3.SystemProgram.programId
        },
        `market_registry.create_market(${market.symbol})`
      );
    } else {
      console.log(`[skip] market exists: ${market.symbol}`);
    }
  }

  if (!(await accountExists(connection, pool))) {
    await callRpc(
      lpVaultProgram,
      "initializePool",
      [toBn(cooldownSecs), minLiquidityBufferBps],
      {
        admin: wallet,
        usdcMint,
        orderEngineProgram: orderEngineProgramId,
        engineAuthority,
        pool,
        liquidityAuth,
        insuranceAuth,
        protocolFeeAuth,
        liquidityVault,
        insuranceVault,
        protocolFeeVault,
        tokenProgram: TOKEN_PROGRAM_ID,
        systemProgram: anchor.web3.SystemProgram.programId
      },
      "lp_vault.initialize_pool"
    );
  } else {
    console.log("[skip] lp_vault.initialize_pool (already initialized)");
  }

  if (!(await accountExists(connection, engineConfig))) {
    await callRpc(
      orderEngineProgram,
      "initializeEngine",
      [toBn(maxTtlSecs), liquidationPenaltyBps, maxImrBps],
      {
        admin: wallet,
        usdcMint,
        marketRegistryProgram: marketRegistryProgramId,
        globalConfig,
        keeperSet,
        lpVaultProgram: lpVaultProgramId,
        lpPool: pool,
        lpLiquidityVault: liquidityVault,
        lpInsuranceVault: insuranceVault,
        lpProtocolFeeVault: protocolFeeVault,
        engineAuthority,
        engineConfig,
        collateralVault,
        tokenProgram: TOKEN_PROGRAM_ID,
        systemProgram: anchor.web3.SystemProgram.programId
      },
      "order_engine.initialize_engine"
    );
  } else {
    console.log("[skip] order_engine.initialize_engine (already initialized)");
  }

  for (const market of bootstrapMarkets) {
    const marketPda = deriveMarketPda(marketRegistryProgramId, market.marketId);
    const marketFundingState = deriveFundingPda(
      orderEngineProgramId,
      market.marketId
    );
    if (!(await accountExists(connection, marketFundingState))) {
      await callRpc(
        orderEngineProgram,
        "initializeMarketFundingState",
        [toBn(market.marketId)],
        {
          admin: wallet,
          engineConfig,
          marketRegistryProgram: marketRegistryProgramId,
          market: marketPda,
          marketFundingState,
          systemProgram: anchor.web3.SystemProgram.programId
        },
        `order_engine.initialize_market_funding_state(${market.symbol})`
      );
    } else {
      console.log(`[skip] funding exists: ${market.symbol}`);
    }
  }

  for (const keeper of keepers) {
    if (!keeper.equals(wallet)) {
      console.log(
        `[skip] keeper rebate init requires signer: ${keeper.toBase58()}`
      );
      continue;
    }
    const keeperRebate = anchor.web3.PublicKey.findProgramAddressSync(
      [Buffer.from("keeper-rebate"), pool.toBuffer(), keeper.toBuffer()],
      lpVaultProgramId
    )[0];

    if (!(await accountExists(connection, keeperRebate))) {
      await callRpc(
        lpVaultProgram,
        "initKeeperRebate",
        [],
        {
          keeper,
          pool,
          keeperRebate,
          systemProgram: anchor.web3.SystemProgram.programId
        },
        `lp_vault.init_keeper_rebate(${keeper.toBase58()})`
      );
    } else {
      console.log(`[skip] keeper rebate exists: ${keeper.toBase58()}`);
    }
  }

  console.log("bootstrap complete");
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
