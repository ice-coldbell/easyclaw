use anchor_lang::prelude::*;
use anchor_spl::token::{self, Token, TokenAccount, Transfer};

use crate::{
    error::ErrorCode,
    state::{EngineConfig, UserMargin},
};

pub fn handler(ctx: Context<DepositCollateral>, amount: u64) -> Result<()> {
    require!(amount > 0, ErrorCode::InvalidAmount);
    require_keys_eq!(
        ctx.accounts.user_margin.owner,
        ctx.accounts.user.key(),
        ErrorCode::Unauthorized
    );

    token::transfer(ctx.accounts.deposit_ctx(), amount)?;

    ctx.accounts.user_margin.collateral_balance = ctx
        .accounts
        .user_margin
        .collateral_balance
        .checked_add(amount)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    Ok(())
}

#[derive(Accounts)]
pub struct DepositCollateral<'info> {
    #[account(mut)]
    pub user: Signer<'info>,
    #[account(
        seeds = [b"engine-config"],
        bump = engine_config.bump,
    )]
    pub engine_config: Account<'info, EngineConfig>,
    #[account(
        mut,
        seeds = [b"user-margin", user.key().as_ref()],
        bump = user_margin.bump,
    )]
    pub user_margin: Account<'info, UserMargin>,
    #[account(
        mut,
        constraint = user_token_account.mint == engine_config.usdc_mint @ ErrorCode::InvalidCollateralMint,
        constraint = user_token_account.owner == user.key() @ ErrorCode::Unauthorized,
    )]
    pub user_token_account: Account<'info, TokenAccount>,
    #[account(mut, address = engine_config.collateral_vault)]
    pub collateral_vault: Account<'info, TokenAccount>,
    pub token_program: Program<'info, Token>,
}

impl<'info> DepositCollateral<'info> {
    fn deposit_ctx(&self) -> CpiContext<'_, '_, '_, 'info, Transfer<'info>> {
        let cpi_accounts = Transfer {
            from: self.user_token_account.to_account_info(),
            to: self.collateral_vault.to_account_info(),
            authority: self.user.to_account_info(),
        };
        CpiContext::new(self.token_program.to_account_info(), cpi_accounts)
    }
}
