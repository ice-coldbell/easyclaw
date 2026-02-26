use anchor_lang::prelude::*;
use anchor_spl::token::{self, Token, TokenAccount, Transfer};

use crate::{
    error::ErrorCode,
    state::{LpPosition, Pool},
};

pub fn handler(ctx: Context<DepositLp>, amount: u64) -> Result<()> {
    require!(amount > 0, ErrorCode::InvalidAmount);

    require_keys_eq!(
        ctx.accounts.lp_position.owner,
        ctx.accounts.user.key(),
        ErrorCode::Unauthorized
    );
    require_keys_eq!(
        ctx.accounts.lp_position.pool,
        ctx.accounts.pool.key(),
        ErrorCode::InvalidLpPosition
    );

    let pre_liquidity = ctx.accounts.liquidity_vault.amount;
    let total_shares_before = ctx.accounts.pool.total_shares;
    token::transfer(ctx.accounts.deposit_ctx(), amount)?;

    let minted_shares = if total_shares_before == 0 {
        amount as u128
    } else {
        ((amount as u128)
            .checked_mul(total_shares_before)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?)
        .checked_div(pre_liquidity as u128)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?
    };

    require!(minted_shares > 0, ErrorCode::InvalidAmount);

    let pool = &mut ctx.accounts.pool;
    pool.total_shares = pool
        .total_shares
        .checked_add(minted_shares)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    let lp = &mut ctx.accounts.lp_position;
    lp.shares = lp
        .shares
        .checked_add(minted_shares)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    Ok(())
}

#[derive(Accounts)]
pub struct DepositLp<'info> {
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
        constraint = user_token_account.mint == pool.usdc_mint @ ErrorCode::InvalidTokenAccount,
        constraint = user_token_account.owner == user.key() @ ErrorCode::Unauthorized,
    )]
    pub user_token_account: Account<'info, TokenAccount>,
    #[account(mut, address = pool.liquidity_vault)]
    pub liquidity_vault: Account<'info, TokenAccount>,
    pub token_program: Program<'info, Token>,
}

impl<'info> DepositLp<'info> {
    fn deposit_ctx(&self) -> CpiContext<'_, '_, '_, 'info, Transfer<'info>> {
        let cpi_accounts = Transfer {
            from: self.user_token_account.to_account_info(),
            to: self.liquidity_vault.to_account_info(),
            authority: self.user.to_account_info(),
        };
        CpiContext::new(self.token_program.to_account_info(), cpi_accounts)
    }
}
