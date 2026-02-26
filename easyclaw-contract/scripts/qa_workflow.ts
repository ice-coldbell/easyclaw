import "dotenv/config"
import fs from "node:fs"
import path from "node:path"

import * as anchor from "@coral-xyz/anchor"
import {
  Connection,
  Keypair,
  PublicKey,
  SystemProgram,
  Transaction,
  TransactionInstruction,
} from "@solana/web3.js"
import {
  ASSOCIATED_TOKEN_PROGRAM_ID,
  TOKEN_PROGRAM_ID,
  createAssociatedTokenAccountIdempotentInstruction,
  getAssociatedTokenAddressSync,
} from "@solana/spl-token"
import orderEngineIdl from "../target/idl/order_engine.json"
import lpVaultIdl from "../target/idl/lp_vault.json"

const MARKET_IDS = [1, 2, 3] as const

const DISCRIMINATORS = {
  createMarginAccount: [98, 114, 213, 184, 129, 89, 90, 185],
  createUserMarketPosition: [184, 183, 182, 125, 240, 234, 69, 166],
  depositCollateral: [156, 131, 142, 116, 146, 247, 162, 120],
  placeOrder: [51, 194, 155, 175, 109, 130, 96, 106],
  createLpPosition: [157, 34, 123, 115, 132, 209, 158, 124],
  depositLp: [83, 107, 16, 26, 26, 20, 130, 56],
} as const

function readEnvPubkey(name: string): PublicKey {
  const raw = process.env[name]
  if (!raw || raw.trim().length === 0) {
    throw new Error(`missing env: ${name}`)
  }
  return new PublicKey(raw.trim())
}

function readEnvNumber(name: string, fallback: number): number {
  const raw = process.env[name]
  if (!raw || raw.trim().length === 0) {
    return fallback
  }
  const value = Number(raw)
  if (!Number.isFinite(value)) {
    throw new Error(`invalid env value ${name}: ${raw}`)
  }
  return value
}

function readKeypair(filePath: string): Keypair {
  const fullPath = path.resolve(filePath)
  const secretKey = JSON.parse(fs.readFileSync(fullPath, "utf8"))
  return Keypair.fromSecretKey(Uint8Array.from(secretKey))
}

function u64Le(value: number | bigint | anchor.BN): Buffer {
  return new anchor.BN(value).toArrayLike(Buffer, "le", 8)
}

function i64Le(value: number | bigint | anchor.BN): Buffer {
  const bn = new anchor.BN(value)
  if (bn.isNeg()) {
    throw new Error("TTL must be positive")
  }
  return bn.toArrayLike(Buffer, "le", 8)
}

function boolByte(value: boolean): number {
  return value ? 1 : 0
}

function boolToByte(value: boolean): Buffer {
  return Buffer.from([boolByte(value)])
}

function u64FromAccountInfo(value: any): anchor.BN {
  if (typeof value === "bigint") {
    return new anchor.BN(value.toString())
  }
  if (typeof value === "number") {
    return new anchor.BN(value)
  }
  if (value?.toString) {
    return new anchor.BN(value.toString())
  }
  return new anchor.BN(0)
}

function buildInstruction(
  programId: PublicKey,
  discriminator: number[],
  keys: { pubkey: PublicKey; isSigner: boolean; isWritable: boolean }[],
  data: Buffer,
): TransactionInstruction {
  return new TransactionInstruction({
    programId,
    keys,
    data: Buffer.concat([Buffer.from(discriminator), data]),
  })
}

async function ensureAta(connection: Connection, owner: PublicKey, mint: PublicKey): Promise<PublicKey> {
  const ata = getAssociatedTokenAddressSync(
    mint,
    owner,
    false,
    TOKEN_PROGRAM_ID,
    ASSOCIATED_TOKEN_PROGRAM_ID,
  )
  const exists = await connection.getAccountInfo(ata)
  if (!exists) {
    const ixs = [
      createAssociatedTokenAccountIdempotentInstruction(
        owner,
        ata,
        owner,
        mint,
        TOKEN_PROGRAM_ID,
        ASSOCIATED_TOKEN_PROGRAM_ID,
      ),
    ]
    const latestBlockhash = await connection.getLatestBlockhash("confirmed")
    const tx = new Transaction().add(...ixs)
    tx.recentBlockhash = latestBlockhash.blockhash
    tx.feePayer = owner
    const kp = readKeypair(process.env.QA_SIGNER || process.env.ANCHOR_WALLET || path.join(process.env.HOME ?? "", ".config/solana/id.json"))
    tx.sign(kp)
    await connection.sendRawTransaction(await tx.serialize(), { skipPreflight: false })
    await connection.confirmTransaction(latestBlockhash, "confirmed")
  }
  return ata
}

async function sendTx(connection: Connection, tx: Transaction, payer: Keypair): Promise<string> {
  const latestBlockhash = await connection.getLatestBlockhash("confirmed")
  tx.recentBlockhash = latestBlockhash.blockhash
  tx.feePayer = payer.publicKey
  tx.sign(payer)
  const signature = await connection.sendRawTransaction(tx.serialize())
  await connection.confirmTransaction({ signature, ...latestBlockhash }, "confirmed")
  return signature
}

async function main() {
  const keypairPath =
    process.env.QA_SIGNER ||
    process.env.ANCHOR_WALLET ||
    path.join(process.env.HOME ?? "", ".config/solana/id.json")
  const signer = readKeypair(keypairPath)

  const rpc = process.env.ANCHOR_PROVIDER_URL || "http://127.0.0.1:8899"
  const connection = new Connection(rpc, {
    commitment: "confirmed",
  })
  const wallet = new anchor.Wallet(signer)
  const provider = new anchor.AnchorProvider(connection, wallet, { commitment: "confirmed" })

  const orderEngineProgramId = readEnvPubkey("ORDER_ENGINE_PROGRAM_ID")
  const marketRegistryProgramId = readEnvPubkey("MARKET_REGISTRY_PROGRAM_ID")
  const lpVaultProgramId = readEnvPubkey("LP_VAULT_PROGRAM_ID")
  const usdcMint = readEnvPubkey("USDC_MINT")

  const orderEngineProgram = new (anchor.Program as any)({
    ...(orderEngineIdl as any),
    address: orderEngineProgramId.toBase58(),
  }, provider)
  const lpVaultProgram = new (anchor.Program as any)({
    ...(lpVaultIdl as any),
    address: lpVaultProgramId.toBase58(),
  }, provider)

  const engineConfig = PublicKey.findProgramAddressSync(
    [Buffer.from("engine-config")],
    orderEngineProgramId,
  )[0]
  const collateralVault = PublicKey.findProgramAddressSync(
    [Buffer.from("collateral-vault")],
    orderEngineProgramId,
  )[0]
  const globalConfig = PublicKey.findProgramAddressSync(
    [Buffer.from("global-config")],
    marketRegistryProgramId,
  )[0]
  const userMarginPda = PublicKey.findProgramAddressSync(
    [Buffer.from("user-margin"), signer.publicKey.toBuffer()],
    orderEngineProgramId,
  )[0]

  const pool = PublicKey.findProgramAddressSync(
    [Buffer.from("pool"), usdcMint.toBuffer()],
    lpVaultProgramId,
  )[0]

  const userAta = getAssociatedTokenAddressSync(
    usdcMint,
    signer.publicKey,
    false,
    TOKEN_PROGRAM_ID,
    ASSOCIATED_TOKEN_PROGRAM_ID,
  )

  console.log("[step] signer:", signer.publicKey.toBase58())
  console.log("[step] rpc:", rpc)

  const existingAta = await connection.getAccountInfo(userAta)
  if (!existingAta) {
    const tx = new Transaction().add(
      createAssociatedTokenAddressCreateInstruction(
        signer.publicKey,
        signer.publicKey,
        userAta,
        usdcMint,
      ),
    )
    const sig = await sendTx(connection, tx, signer)
    console.log("[ok] create associated token account:", sig)
  }

  const userMarginAccount = await connection.getAccountInfo(userMarginPda)
  if (!userMarginAccount) {
    const tx = new Transaction().add(
      buildInstruction(
        orderEngineProgramId,
        DISCRIMINATORS.createMarginAccount,
        [
          { pubkey: signer.publicKey, isSigner: true, isWritable: true },
          { pubkey: engineConfig, isSigner: false, isWritable: false },
          { pubkey: userMarginPda, isSigner: false, isWritable: true },
          { pubkey: SystemProgram.programId, isSigner: false, isWritable: false },
        ],
        Buffer.alloc(0),
      ),
    )
    const sig = await sendTx(connection, tx, signer)
    console.log("[ok] create user margin account:", sig)
  }

  const marginTopUp = readEnvNumber("MARGIN_DEPOSIT_AMOUNT", 10_000_000_000)
  const collateralBalanceBeforeTx = await orderEngineProgram.account.userMargin.fetchNullable(userMarginPda)
  const shouldDeposit = collateralBalanceBeforeTx?.collateralBalance.eqn(0)
    ? true
    : collateralBalanceBeforeTx?.collateralBalance.lt(new anchor.BN(marginTopUp))
      ? true
      : false

  if (shouldDeposit) {
    const tx = new Transaction().add(
      buildInstruction(
        orderEngineProgramId,
        DISCRIMINATORS.depositCollateral,
        [
          { pubkey: signer.publicKey, isSigner: true, isWritable: true },
          { pubkey: engineConfig, isSigner: false, isWritable: false },
          { pubkey: userMarginPda, isSigner: false, isWritable: true },
          { pubkey: userAta, isSigner: false, isWritable: true },
          { pubkey: collateralVault, isSigner: false, isWritable: true },
          { pubkey: TOKEN_PROGRAM_ID, isSigner: false, isWritable: false },
        ],
        Buffer.concat([u64Le(marginTopUp)]),
      ),
    )
    const sig = await sendTx(connection, tx, signer)
    console.log("[ok] deposit collateral:", sig)
  }

  const lpPoolState = await lpVaultProgram.account.pool.fetch(pool)
  const liquidityVault = lpPoolState.liquidityVault

  const lpPositionPda = PublicKey.findProgramAddressSync(
    [Buffer.from("lp-pos"), pool.toBuffer(), signer.publicKey.toBuffer()],
    lpVaultProgramId,
  )[0]
  const lpPositionAccount = await connection.getAccountInfo(lpPositionPda)
  if (!lpPositionAccount) {
    const tx = new Transaction().add(
      buildInstruction(
        lpVaultProgramId,
        DISCRIMINATORS.createLpPosition,
        [
          { pubkey: signer.publicKey, isSigner: true, isWritable: true },
          { pubkey: pool, isSigner: false, isWritable: false },
          { pubkey: lpPositionPda, isSigner: false, isWritable: true },
          { pubkey: SystemProgram.programId, isSigner: false, isWritable: false },
        ],
        Buffer.alloc(0),
      ),
    )
    const sig = await sendTx(connection, tx, signer)
    console.log("[ok] create lp position:", sig)
  }

  const lpTopUp = readEnvNumber("LP_DEPOSIT_AMOUNT", 10_000_000_000)
  const lpPositionState = await lpVaultProgram.account.lpPosition.fetchNullable(lpPositionPda)
  const shouldDepositLp = !lpPositionState || lpPositionState.pendingShares.eqn(0)
  if (shouldDepositLp) {
    const tx = new Transaction().add(
      buildInstruction(
        lpVaultProgramId,
        DISCRIMINATORS.depositLp,
        [
          { pubkey: signer.publicKey, isSigner: true, isWritable: true },
          { pubkey: pool, isSigner: false, isWritable: true },
          { pubkey: lpPositionPda, isSigner: false, isWritable: true },
          { pubkey: userAta, isSigner: false, isWritable: true },
          { pubkey: liquidityVault, isSigner: false, isWritable: true },
          { pubkey: TOKEN_PROGRAM_ID, isSigner: false, isWritable: false },
        ],
        u64Le(lpTopUp),
      ),
    )
    const sig = await sendTx(connection, tx, signer)
    console.log("[ok] deposit lp:", sig)
  }

  for (const marketId of MARKET_IDS) {
    const marketPda = PublicKey.findProgramAddressSync(
      [Buffer.from("market"), u64Le(marketId)],
      marketRegistryProgramId,
    )[0]
    const userMarketPositionPda = PublicKey.findProgramAddressSync(
      [Buffer.from("user-market-pos"), userMarginPda.toBuffer(), u64Le(marketId)],
      orderEngineProgramId,
    )[0]
    const exists = await connection.getAccountInfo(userMarketPositionPda)
    if (!exists) {
      const tx = new Transaction().add(
        buildInstruction(
          orderEngineProgramId,
          DISCRIMINATORS.createUserMarketPosition,
          [
            { pubkey: signer.publicKey, isSigner: true, isWritable: true },
            { pubkey: engineConfig, isSigner: false, isWritable: false },
            { pubkey: userMarginPda, isSigner: false, isWritable: true },
            { pubkey: userMarketPositionPda, isSigner: false, isWritable: true },
            { pubkey: SystemProgram.programId, isSigner: false, isWritable: false },
          ],
          Buffer.concat([u64Le(marketId)]),
        ),
      )
      const sig = await sendTx(connection, tx, signer)
      console.log(`[ok] create user market position(${marketId}):`, sig)
    }
  }

  const userMarginState = await orderEngineProgram.account.userMargin.fetch(userMarginPda)
  let orderNonce = u64FromAccountInfo(userMarginState.nextOrderNonce).toNumber()

  const orderConfig = [
    { side: 0, orderType: 0, margin: 1_000_000_000, long: true, limitPrice: 120_000_000_000, marketId: 1 },
    { side: 1, orderType: 0, margin: 1_000_000_000, long: false, limitPrice: 1, marketId: 1 },
    { side: 0, orderType: 1, margin: 1_000_000_000, long: true, limitPrice: 110_000_000_000, marketId: 1 },
    { side: 1, orderType: 1, margin: 1_000_000_000, long: false, limitPrice: 80_000_000_000, marketId: 1 },

    { side: 0, orderType: 0, margin: 1_000_000_000, long: true, limitPrice: 3_200_000_000, marketId: 2 },
    { side: 1, orderType: 0, margin: 1_000_000_000, long: false, limitPrice: 1, marketId: 2 },
    { side: 0, orderType: 1, margin: 1_000_000_000, long: true, limitPrice: 3_500_000_000, marketId: 2 },
    { side: 1, orderType: 1, margin: 1_000_000_000, long: false, limitPrice: 2_000_000_000, marketId: 2 },

    { side: 0, orderType: 0, margin: 1_000_000_000, long: true, limitPrice: 220_000_000, marketId: 3 },
    { side: 1, orderType: 0, margin: 1_000_000_000, long: false, limitPrice: 1, marketId: 3 },
    { side: 0, orderType: 1, margin: 1_000_000_000, long: true, limitPrice: 260_000_000, marketId: 3 },
    { side: 1, orderType: 1, margin: 1_000_000_000, long: false, limitPrice: 130_000_000, marketId: 3 },
  ]

  for (const [idx, item] of orderConfig.entries()) {
    const { side, orderType, margin, limitPrice, marketId } = item
    const marketPda = PublicKey.findProgramAddressSync(
      [Buffer.from("market"), u64Le(marketId)],
      marketRegistryProgramId,
    )[0]
    const orderPda = PublicKey.findProgramAddressSync(
      [Buffer.from("order"), userMarginPda.toBuffer(), u64Le(orderNonce)],
      orderEngineProgramId,
    )[0]
    orderNonce += 1

    const ix = buildInstruction(
      orderEngineProgramId,
      DISCRIMINATORS.placeOrder,
      [
        { pubkey: signer.publicKey, isSigner: true, isWritable: true },
        { pubkey: engineConfig, isSigner: false, isWritable: false },
        { pubkey: marketRegistryProgramId, isSigner: false, isWritable: false },
        { pubkey: globalConfig, isSigner: false, isWritable: false },
        { pubkey: marketPda, isSigner: false, isWritable: false },
        { pubkey: userMarginPda, isSigner: false, isWritable: true },
        { pubkey: orderPda, isSigner: false, isWritable: true },
        { pubkey: SystemProgram.programId, isSigner: false, isWritable: false },
      ],
      Buffer.concat([
        u64Le(marketId),
        Buffer.from([side]),
        Buffer.from([orderType]),
        boolToByte(false),
        u64Le(margin),
        u64Le(limitPrice),
        i64Le(300),
        u64Le(1_000 + idx),
      ]),
    )

    const tx = new Transaction().add(ix)
    const sig = await sendTx(connection, tx, signer)
    console.log(`[ok] place order (market=${marketId}, side=${side}, type=${orderType}, idx=${idx}):`, sig)
  }

  console.log("[done] qa workflow finished")
}

main().catch((error) => {
  console.error(error)
  process.exitCode = 1
})

function createAssociatedTokenAddressCreateInstruction(
  fundingAccount: PublicKey,
  owner: PublicKey,
  ata: PublicKey,
  mint: PublicKey,
): TransactionInstruction {
  return createAssociatedTokenAccountIdempotentInstruction(
    fundingAccount,
    ata,
    owner,
    mint,
    TOKEN_PROGRAM_ID,
    ASSOCIATED_TOKEN_PROGRAM_ID,
  )
}
