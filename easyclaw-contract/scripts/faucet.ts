import "dotenv/config";
import fs from "node:fs";
import path from "node:path";
import * as anchor from "@coral-xyz/anchor";
import {
  ASSOCIATED_TOKEN_PROGRAM_ID,
  createAssociatedTokenAccountIdempotentInstruction,
  getAssociatedTokenAddressSync,
  TOKEN_PROGRAM_ID
} from "@solana/spl-token";

type FaucetAction = "show" | "init" | "claim" | "set-limits";

function readIdlFromCandidates(): any {
  const fromEnv = process.env.FAUCET_IDL_PATH?.trim();
  const candidates = [
    fromEnv,
    path.resolve(process.cwd(), "idl/usdc_faucet.json"),
    path.resolve(process.cwd(), "target/idl/usdc_faucet.json")
  ].filter((value): value is string => Boolean(value && value.length > 0));

  for (const candidate of candidates) {
    if (fs.existsSync(candidate)) {
      return JSON.parse(fs.readFileSync(candidate, "utf8"));
    }
  }

  throw new Error(
    "usdc_faucet IDL not found. Run `anchor build` first or set FAUCET_IDL_PATH."
  );
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

function getRequiredPubkey(name: string): anchor.web3.PublicKey {
  const value = process.env[name]?.trim();
  if (!value) {
    throw new Error(`Missing required env: ${name}`);
  }
  return new anchor.web3.PublicKey(value);
}

function getAction(): FaucetAction {
  const action = (process.env.FAUCET_ACTION ?? "show").trim() as FaucetAction;
  if (!["show", "init", "claim", "set-limits"].includes(action)) {
    throw new Error(`Invalid FAUCET_ACTION: ${action}`);
  }
  return action;
}

function addAccountAliases(
  accounts: Record<string, anchor.web3.PublicKey>
): Record<string, anchor.web3.PublicKey> {
  const toSnake = (value: string) =>
    value.replace(/[A-Z]/g, (match) => `_${match.toLowerCase()}`);
  const toCamel = (value: string) =>
    value.replace(/_([a-z])/g, (_, letter: string) => letter.toUpperCase());

  const aliases: Record<string, anchor.web3.PublicKey> = {};
  for (const [key, value] of Object.entries(accounts)) {
    aliases[key] = value;
    aliases[toSnake(key)] = value;
    aliases[toCamel(key)] = value;
  }
  return aliases;
}

async function callProgram(
  program: any,
  methodName: string,
  args: any[],
  accounts: Record<string, anchor.web3.PublicKey>
): Promise<string> {
  const toSnake = (value: string) =>
    value.replace(/[A-Z]/g, (match) => `_${match.toLowerCase()}`);
  const toCamel = (value: string) =>
    value.replace(/_([a-z])/g, (_, letter: string) => letter.toUpperCase());

  const candidates = Array.from(
    new Set([methodName, toCamel(methodName), toSnake(methodName)])
  );
  const method = candidates
    .map((name) => program.methods?.[name])
    .find((fn) => typeof fn === "function");

  if (typeof method !== "function") {
    throw new Error(`Method not found: ${methodName}`);
  }

  return method(...args).accounts(addAccountAliases(accounts)).rpc();
}

async function ensureAta(
  provider: anchor.AnchorProvider,
  owner: anchor.web3.PublicKey,
  mint: anchor.web3.PublicKey
): Promise<anchor.web3.PublicKey> {
  const ata = getAssociatedTokenAddressSync(
    mint,
    owner,
    false,
    TOKEN_PROGRAM_ID,
    ASSOCIATED_TOKEN_PROGRAM_ID
  );
  const ataInfo = await provider.connection.getAccountInfo(ata);
  if (ataInfo) {
    return ata;
  }

  const createAtaIx = createAssociatedTokenAccountIdempotentInstruction(
    provider.wallet.publicKey,
    ata,
    owner,
    mint,
    TOKEN_PROGRAM_ID,
    ASSOCIATED_TOKEN_PROGRAM_ID
  );
  const sig = await provider.sendAndConfirm(
    new anchor.web3.Transaction().add(createAtaIx)
  );
  console.log(`[ok] create_ata: ${sig}`);
  return ata;
}

async function main() {
  const action = getAction();
  const provider = anchor.AnchorProvider.env();
  anchor.setProvider(provider);

  const idl = normalizeIdl(readIdlFromCandidates());
  const programId = new anchor.web3.PublicKey(
    process.env.FAUCET_PROGRAM_ID?.trim() || idl.address
  );
  const program = new (anchor.Program as any)(
    {
      ...(idl as any),
      address: programId.toBase58()
    } as anchor.Idl,
    provider
  );

  const mint = getRequiredPubkey("USDC_MINT");
  const faucetConfig = anchor.web3.PublicKey.findProgramAddressSync(
    [Buffer.from("faucet-config"), mint.toBuffer()],
    programId
  )[0];
  const faucetAuthority = anchor.web3.PublicKey.findProgramAddressSync(
    [Buffer.from("faucet-authority"), mint.toBuffer()],
    programId
  )[0];

  console.log("action:", action);
  console.log("provider:", provider.wallet.publicKey.toBase58());
  console.log("faucet_program:", programId.toBase58());
  console.log("usdc_mint:", mint.toBase58());
  console.log("faucet_config:", faucetConfig.toBase58());
  console.log("faucet_authority:", faucetAuthority.toBase58());

  if (action === "show") {
    return;
  }

  if (action === "init") {
    const defaultAmount = getEnvNumber("FAUCET_DEFAULT_AMOUNT", 1_000_000);
    const maxClaimAmount = getEnvNumber("FAUCET_MAX_CLAIM_AMOUNT", 100_000_000);
    const configInfo = await provider.connection.getAccountInfo(faucetConfig);
    if (configInfo) {
      console.log("[skip] initialize_faucet (already initialized)");
      return;
    }
    const sig = await callProgram(
      program,
      "initializeFaucet",
      [new anchor.BN(defaultAmount), new anchor.BN(maxClaimAmount)],
      {
        admin: provider.wallet.publicKey,
        mint,
        faucetAuthority,
        faucetConfig,
        tokenProgram: TOKEN_PROGRAM_ID,
        systemProgram: anchor.web3.SystemProgram.programId
      }
    );
    console.log(`[ok] initialize_faucet: ${sig}`);
    return;
  }

  if (action === "set-limits") {
    const defaultAmount = getEnvNumber("FAUCET_DEFAULT_AMOUNT", 1_000_000);
    const maxClaimAmount = getEnvNumber("FAUCET_MAX_CLAIM_AMOUNT", 100_000_000);
    const sig = await callProgram(
      program,
      "updateFaucetLimits",
      [new anchor.BN(defaultAmount), new anchor.BN(maxClaimAmount)],
      {
        admin: provider.wallet.publicKey,
        faucetConfig
      }
    );
    console.log(`[ok] update_faucet_limits: ${sig}`);
    return;
  }

  const claimAmount = getEnvNumber("FAUCET_CLAIM_AMOUNT", 0);
  const userAta = await ensureAta(provider, provider.wallet.publicKey, mint);
  const sig = await callProgram(
    program,
    "claimFromFaucet",
    [new anchor.BN(claimAmount)],
    {
      user: provider.wallet.publicKey,
      faucetConfig,
      mint,
      faucetAuthority,
      userTokenAccount: userAta,
      tokenProgram: TOKEN_PROGRAM_ID
    }
  );
  console.log(`[ok] claim_from_faucet: ${sig}`);
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
