use anchor_lang::prelude::*;
use anchor_spl::token::{self, Token, TokenAccount, Transfer};

use crate::{
    constants::BPS_DENOM,
    error::ErrorCode,
    state::{LpPosition, Pool, WithdrawRequest},
};

pub fn handler(ctx: Context<ClaimWithdrawLp>) -> Result<()> {
    let now = Clock::get()?.unix_timestamp;

    require_keys_eq!(
        ctx.accounts.withdraw_request.owner,
        ctx.accounts.user.key(),
        ErrorCode::Unauthorized
    );
    require_keys_eq!(
        ctx.accounts.withdraw_request.pool,
        ctx.accounts.pool.key(),
        ErrorCode::InvalidWithdrawRequest
    );
    require!(
        !ctx.accounts.withdraw_request.claimed,
        ErrorCode::AlreadyClaimed
    );
    require!(
        now >= ctx.accounts.withdraw_request.requested_at + ctx.accounts.pool.cooldown_secs,
        ErrorCode::CooldownNotFinished
    );
    require!(
        ctx.accounts.lp_position.pending_shares >= ctx.accounts.withdraw_request.share_amount,
        ErrorCode::InsufficientShares
    );
    require!(
        ctx.accounts.pool.total_shares > 0,
        ErrorCode::InvalidPoolState
    );

    let liquidity_before = ctx.accounts.liquidity_vault.amount;
    let share_amount = ctx.accounts.withdraw_request.share_amount;
    let total_shares = ctx.accounts.pool.total_shares;
    let min_liquidity_buffer_bps = ctx.accounts.pool.min_liquidity_buffer_bps;
    let withdraw_amount = (share_amount
        .checked_mul(liquidity_before as u128)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?)
    .checked_div(total_shares)
    .ok_or_else(|| error!(ErrorCode::MathOverflow))? as u64;

    require!(withdraw_amount > 0, ErrorCode::InvalidAmount);

    let post_liquidity = liquidity_before
        .checked_sub(withdraw_amount)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    let min_buffer_amount = ((liquidity_before as u128)
        .checked_mul(min_liquidity_buffer_bps as u128)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?)
    .checked_div(BPS_DENOM as u128)
    .ok_or_else(|| error!(ErrorCode::MathOverflow))? as u64;

    require!(
        post_liquidity >= min_buffer_amount,
        ErrorCode::LiquidityBufferViolation
    );

    let liquidity_auth_bump = ctx.bumps.liquidity_auth;
    let liquidity_auth_key = ctx.accounts.pool.key();
    let signer_seed_group: &[&[u8]] = &[
        b"liquidity-auth",
        liquidity_auth_key.as_ref(),
        &[liquidity_auth_bump],
    ];
    let signer_seeds = &[signer_seed_group];
    let cpi_accounts = Transfer {
        from: ctx.accounts.liquidity_vault.to_account_info(),
        to: ctx.accounts.user_token_account.to_account_info(),
        authority: ctx.accounts.liquidity_auth.to_account_info(),
    };
    token::transfer(
        CpiContext::new_with_signer(
            ctx.accounts.token_program.to_account_info(),
            cpi_accounts,
            signer_seeds,
        ),
        withdraw_amount,
    )?;

    let lp = &mut ctx.accounts.lp_position;
    lp.pending_shares = lp
        .pending_shares
        .checked_sub(share_amount)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    let pool = &mut ctx.accounts.pool;
    pool.total_shares = pool
        .total_shares
        .checked_sub(share_amount)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    ctx.accounts.withdraw_request.claimed = true;

    Ok(())
}

#[derive(Accounts)]
pub struct ClaimWithdrawLp<'info> {
    #[account(mut)]
    pub user: Signer<'info>,
    #[account(
        mut,
        seeds = [b"pool", pool.usdc_mint.as_ref()],
        bump = pool.bump,
    )]
    pub pool: Account<'info, Pool>,
    #[account(
        mut,
        seeds = [b"lp-pos", pool.key().as_ref(), user.key().as_ref()],
        bump = lp_position.bump,
    )]
    pub lp_position: Account<'info, LpPosition>,
    #[account(
        mut,
        close = user,
        seeds = [b"withdraw-req", pool.key().as_ref(), user.key().as_ref(), &withdraw_request.nonce.to_le_bytes()],
        bump = withdraw_request.bump,
    )]
    pub withdraw_request: Account<'info, WithdrawRequest>,
    /// CHECK: liquidity auth PDA.
    #[account(seeds = [b"liquidity-auth", pool.key().as_ref()], bump)]
    pub liquidity_auth: UncheckedAccount<'info>,
    #[account(mut, address = pool.liquidity_vault)]
    pub liquidity_vault: Account<'info, TokenAccount>,
    #[account(
        mut,
        constraint = user_token_account.mint == pool.usdc_mint @ ErrorCode::InvalidTokenAccount,
        constraint = user_token_account.owner == user.key() @ ErrorCode::Unauthorized,
    )]
    pub user_token_account: Account<'info, TokenAccount>,
    pub token_program: Program<'info, Token>,
}
