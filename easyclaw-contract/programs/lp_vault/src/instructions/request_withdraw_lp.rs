use anchor_lang::prelude::*;

use crate::{
    error::ErrorCode,
    state::{LpPosition, Pool, WithdrawRequest},
};

pub fn handler(ctx: Context<RequestWithdrawLp>, share_amount: u128) -> Result<()> {
    require!(share_amount > 0, ErrorCode::InvalidAmount);

    let lp = &mut ctx.accounts.lp_position;
    require_keys_eq!(lp.owner, ctx.accounts.user.key(), ErrorCode::Unauthorized);
    require_keys_eq!(
        lp.pool,
        ctx.accounts.pool.key(),
        ErrorCode::InvalidLpPosition
    );
    require!(lp.shares >= share_amount, ErrorCode::InsufficientShares);

    lp.shares = lp
        .shares
        .checked_sub(share_amount)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    lp.pending_shares = lp
        .pending_shares
        .checked_add(share_amount)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    let req = &mut ctx.accounts.withdraw_request;
    req.owner = ctx.accounts.user.key();
    req.pool = ctx.accounts.pool.key();
    req.nonce = lp.withdraw_nonce;
    req.share_amount = share_amount;
    req.requested_at = Clock::get()?.unix_timestamp;
    req.claimed = false;
    req.bump = ctx.bumps.withdraw_request;

    lp.withdraw_nonce = lp
        .withdraw_nonce
        .checked_add(1)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    Ok(())
}

#[derive(Accounts)]
pub struct RequestWithdrawLp<'info> {
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
        init,
        payer = user,
        seeds = [b"withdraw-req", pool.key().as_ref(), user.key().as_ref(), &lp_position.withdraw_nonce.to_le_bytes()],
        bump,
        space = 8 + WithdrawRequest::INIT_SPACE,
    )]
    pub withdraw_request: Account<'info, WithdrawRequest>,
    pub system_program: Program<'info, System>,
}
