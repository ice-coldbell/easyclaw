use anchor_lang::prelude::*;
use anchor_spl::token::{Mint, Token, TokenAccount};

use crate::{
    constants::{
        BPS_DENOM, DEFAULT_EXECUTION_REBATE_USDC, DEFAULT_INSURANCE_BPS, DEFAULT_LP_BPS,
        DEFAULT_PROTOCOL_BPS,
    },
    error::ErrorCode,
    state::Pool,
};

pub fn handler(
    ctx: Context<InitializePool>,
    cooldown_secs: i64,
    min_liquidity_buffer_bps: u16,
) -> Result<()> {
    require!(cooldown_secs >= 0, ErrorCode::InvalidCooldown);
    require!(
        min_liquidity_buffer_bps <= BPS_DENOM as u16,
        ErrorCode::InvalidBps
    );
    require!(
        ctx.accounts.order_engine_program.executable,
        ErrorCode::InvalidProgramAccount
    );

    let (expected_engine_authority, _) = Pubkey::find_program_address(
        &[b"engine-authority"],
        &ctx.accounts.order_engine_program.key(),
    );
    require_keys_eq!(
        expected_engine_authority,
        ctx.accounts.engine_authority.key(),
        ErrorCode::InvalidEngineAuthority
    );

    let pool = &mut ctx.accounts.pool;
    pool.admin = ctx.accounts.admin.key();
    pool.usdc_mint = ctx.accounts.usdc_mint.key();
    pool.order_engine_program = ctx.accounts.order_engine_program.key();
    pool.engine_authority = ctx.accounts.engine_authority.key();
    pool.liquidity_vault = ctx.accounts.liquidity_vault.key();
    pool.insurance_vault = ctx.accounts.insurance_vault.key();
    pool.protocol_fee_vault = ctx.accounts.protocol_fee_vault.key();
    pool.cooldown_secs = cooldown_secs;
    pool.min_liquidity_buffer_bps = min_liquidity_buffer_bps;
    pool.lp_fee_bps = DEFAULT_LP_BPS;
    pool.insurance_fee_bps = DEFAULT_INSURANCE_BPS;
    pool.protocol_fee_bps = DEFAULT_PROTOCOL_BPS;
    pool.execution_rebate_usdc = DEFAULT_EXECUTION_REBATE_USDC;
    pool.total_shares = 0;
    pool.pending_keeper_rebates = 0;
    pool.total_trading_fees = 0;
    pool.cumulative_trader_pnl = 0;
    pool.bump = ctx.bumps.pool;

    Ok(())
}

#[derive(Accounts)]
pub struct InitializePool<'info> {
    #[account(mut)]
    pub admin: Signer<'info>,
    pub usdc_mint: Account<'info, Mint>,
    /// CHECK: external program id pinned into pool config.
    pub order_engine_program: UncheckedAccount<'info>,
    /// CHECK: PDA owned by order_engine used as CPI signer.
    pub engine_authority: UncheckedAccount<'info>,
    #[account(
        init,
        payer = admin,
        seeds = [b"pool", usdc_mint.key().as_ref()],
        bump,
        space = 8 + Pool::INIT_SPACE,
    )]
    pub pool: Account<'info, Pool>,
    /// CHECK: PDA authority for liquidity vault transfer signing.
    #[account(seeds = [b"liquidity-auth", pool.key().as_ref()], bump)]
    pub liquidity_auth: UncheckedAccount<'info>,
    /// CHECK: PDA authority for insurance vault transfer signing.
    #[account(seeds = [b"insurance-auth", pool.key().as_ref()], bump)]
    pub insurance_auth: UncheckedAccount<'info>,
    /// CHECK: PDA authority for protocol fee vault transfer signing.
    #[account(seeds = [b"protocol-fee-auth", pool.key().as_ref()], bump)]
    pub protocol_fee_auth: UncheckedAccount<'info>,
    #[account(
        init,
        payer = admin,
        seeds = [b"liquidity-vault", pool.key().as_ref()],
        bump,
        token::mint = usdc_mint,
        token::authority = liquidity_auth,
    )]
    pub liquidity_vault: Account<'info, TokenAccount>,
    #[account(
        init,
        payer = admin,
        seeds = [b"insurance-vault", pool.key().as_ref()],
        bump,
        token::mint = usdc_mint,
        token::authority = insurance_auth,
    )]
    pub insurance_vault: Account<'info, TokenAccount>,
    #[account(
        init,
        payer = admin,
        seeds = [b"protocol-fee-vault", pool.key().as_ref()],
        bump,
        token::mint = usdc_mint,
        token::authority = protocol_fee_auth,
    )]
    pub protocol_fee_vault: Account<'info, TokenAccount>,
    pub token_program: Program<'info, Token>,
    pub system_program: Program<'info, System>,
}
