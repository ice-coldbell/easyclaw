import "dotenv/config";
import fs from "node:fs";
import path from "node:path";

import * as anchor from "@coral-xyz/anchor";
import { Connection, Keypair, PublicKey, SystemProgram, Transaction, TransactionInstruction } from "@solana/web3.js";
import {
  ASSOCIATED_TOKEN_PROGRAM_ID,
  TOKEN_PROGRAM_ID,
  createAssociatedTokenAccountIdempotentInstruction,
  getAssociatedTokenAddressSync
} from "@solana/spl-token";
import lpVaultIdl from "../target/idl/lp_vault.json";

const DISCRIMINATORS = {
  createLpPosition: [157, 34, 123, 115, 132, 209, 158, 124],
  depositLp: [83, 107, 16, 26, 26, 20, 130, 56]
} as const;

function readEnvPubkey(name: string): PublicKey {
  const raw = process.env[name];
  if (!raw || raw.trim().length === 0) {
    throw new Error(`Missing required env: ${name}`);
  }
  return new PublicKey(raw.trim());
}

function readEnvNumber(name: string, fallback: number): number {
  const raw = process.env[name];
  if (!raw || raw.trim().length === 0) {
    return fallback;
  }
  const value = Number(raw);
  if (!Number.isFinite(value) || value <= 0) {
    throw new Error(`Invalid numeric env ${name}: ${raw}`);
  }
  return value;
}

function readKeypair(filePath: string): Keypair {
  const fullPath = path.resolve(filePath);
  const secretKey = JSON.parse(fs.readFileSync(fullPath, "utf8"));
  return Keypair.fromSecretKey(Uint8Array.from(secretKey));
}

function u64Le(value: number | bigint | anchor.BN): Buffer {
  return new anchor.BN(value).toArrayLike(Buffer, "le", 8);
}

function buildInstruction(
  programId: PublicKey,
  discriminator: ReadonlyArray<number>,
  keys: Array<{ pubkey: PublicKey; isSigner: boolean; isWritable: boolean }>,
  data: Buffer
): TransactionInstruction {
  return new TransactionInstruction({
    programId,
    keys,
    data: Buffer.concat([Buffer.from(discriminator), data])
  });
}

async function sendTx(connection: Connection, tx: Transaction, payer: Keypair): Promise<string> {
  const latestBlockhash = await connection.getLatestBlockhash("confirmed");
  tx.recentBlockhash = latestBlockhash.blockhash;
  tx.feePayer = payer.publicKey;
  tx.sign(payer);
  const signature = await connection.sendRawTransaction(tx.serialize());
  await connection.confirmTransaction({ signature, ...latestBlockhash }, "confirmed");
  return signature;
}

async function ensureAta(
  connection: Connection,
  owner: PublicKey,
  mint: PublicKey,
  payer: Keypair
): Promise<PublicKey> {
  const ata = getAssociatedTokenAddressSync(
    mint,
    owner,
    false,
    TOKEN_PROGRAM_ID,
    ASSOCIATED_TOKEN_PROGRAM_ID
  );
  const exists = await connection.getAccountInfo(ata);
  if (exists) {
    return ata;
  }

  const createAtaInstruction = createAssociatedTokenAccountIdempotentInstruction(
    payer.publicKey,
    ata,
    owner,
    mint,
    TOKEN_PROGRAM_ID,
    ASSOCIATED_TOKEN_PROGRAM_ID
  );
  const signature = await sendTx(connection, new Transaction().add(createAtaInstruction), payer);
  console.log("[ok] create_ata:", signature);
  return ata;
}

async function main() {
  const keypairPath =
    process.env.LP_SIGNER ||
    process.env.QA_SIGNER ||
    process.env.ANCHOR_WALLET ||
    path.join(process.env.HOME ?? "", ".config/solana/id.json");
  const signer = readKeypair(keypairPath);
  const rpcUrl = process.env.ANCHOR_PROVIDER_URL || "http://127.0.0.1:8899";
  const connection = new Connection(rpcUrl, { commitment: "confirmed" });

  const lpVaultProgramId = readEnvPubkey("LP_VAULT_PROGRAM_ID");
  const usdcMint = readEnvPubkey("USDC_MINT");
  const lpDepositAmount = readEnvNumber("LP_DEPOSIT_AMOUNT", 10_000_000_000);

  const wallet = new anchor.Wallet(signer);
  const provider = new anchor.AnchorProvider(connection, wallet, { commitment: "confirmed" });
  const lpVaultProgram = new (anchor.Program as any)(
    {
      ...(lpVaultIdl as any),
      address: lpVaultProgramId.toBase58()
    },
    provider
  );

  const pool = PublicKey.findProgramAddressSync(
    [Buffer.from("pool"), usdcMint.toBuffer()],
    lpVaultProgramId
  )[0];

  const poolAccountInfo = await connection.getAccountInfo(pool);
  if (!poolAccountInfo) {
    throw new Error("Pool is not initialized. Run bootstrap first.");
  }

  const poolState = await lpVaultProgram.account.pool.fetch(pool);
  const liquidityVault = poolState.liquidityVault as PublicKey;

  const userAta = await ensureAta(connection, signer.publicKey, usdcMint, signer);
  const userBalanceBefore = await connection.getTokenAccountBalance(userAta);
  console.log("[info] signer:", signer.publicKey.toBase58());
  console.log("[info] rpc:", rpcUrl);
  console.log("[info] pool:", pool.toBase58());
  console.log("[info] user_ata:", userAta.toBase58());
  console.log("[info] user_balance_before:", userBalanceBefore.value.uiAmountString);

  const lpPositionPda = PublicKey.findProgramAddressSync(
    [Buffer.from("lp-pos"), pool.toBuffer(), signer.publicKey.toBuffer()],
    lpVaultProgramId
  )[0];

  const lpPositionAccountInfo = await connection.getAccountInfo(lpPositionPda);
  if (!lpPositionAccountInfo) {
    const createLpPositionInstruction = buildInstruction(
      lpVaultProgramId,
      DISCRIMINATORS.createLpPosition,
      [
        { pubkey: signer.publicKey, isSigner: true, isWritable: true },
        { pubkey: pool, isSigner: false, isWritable: false },
        { pubkey: lpPositionPda, isSigner: false, isWritable: true },
        { pubkey: SystemProgram.programId, isSigner: false, isWritable: false }
      ],
      Buffer.alloc(0)
    );
    const createLpPositionSignature = await sendTx(
      connection,
      new Transaction().add(createLpPositionInstruction),
      signer
    );
    console.log("[ok] create_lp_position:", createLpPositionSignature);
  } else {
    console.log("[skip] lp position already exists:", lpPositionPda.toBase58());
  }

  const depositLpInstruction = buildInstruction(
    lpVaultProgramId,
    DISCRIMINATORS.depositLp,
    [
      { pubkey: signer.publicKey, isSigner: true, isWritable: true },
      { pubkey: pool, isSigner: false, isWritable: true },
      { pubkey: lpPositionPda, isSigner: false, isWritable: true },
      { pubkey: userAta, isSigner: false, isWritable: true },
      { pubkey: liquidityVault, isSigner: false, isWritable: true },
      { pubkey: TOKEN_PROGRAM_ID, isSigner: false, isWritable: false }
    ],
    u64Le(lpDepositAmount)
  );
  const depositLpSignature = await sendTx(
    connection,
    new Transaction().add(depositLpInstruction),
    signer
  );
  console.log("[ok] deposit_lp:", depositLpSignature);

  const userBalanceAfter = await connection.getTokenAccountBalance(userAta);
  console.log("[info] user_balance_after:", userBalanceAfter.value.uiAmountString);
  console.log("[done] lp funding finished");
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
