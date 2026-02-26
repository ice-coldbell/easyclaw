use anchor_lang::prelude::*;
use anchor_spl::token::{self, Token, TokenAccount, Transfer};

use crate::{
    error::ErrorCode,
    helpers::mul_bps_u64,
    state::{EngineConfig, UserMargin},
};

pub fn handler(ctx: Context<WithdrawCollateral>, amount: u64) -> Result<()> {
    require!(amount > 0, ErrorCode::InvalidAmount);
    require_keys_eq!(
        ctx.accounts.user_margin.owner,
        ctx.accounts.user.key(),
        ErrorCode::Unauthorized
    );

    let collateral_balance = ctx.accounts.user_margin.collateral_balance;
    require!(
        collateral_balance >= amount,
        ErrorCode::InsufficientCollateral
    );

    let post_collateral = collateral_balance
        .checked_sub(amount)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    let required_margin = mul_bps_u64(
        ctx.accounts.user_margin.total_notional,
        ctx.accounts.engine_config.max_imr_bps as u64,
    )?;
    require!(
        post_collateral >= required_margin,
        ErrorCode::MarginRequirementViolation
    );

    let engine_authority_bump = ctx.bumps.engine_authority;
    let signer_seed_group: &[&[u8]] = &[b"engine-authority", &[engine_authority_bump]];
    let signer_seeds = &[signer_seed_group];
    let cpi_accounts = Transfer {
        from: ctx.accounts.collateral_vault.to_account_info(),
        to: ctx.accounts.user_token_account.to_account_info(),
        authority: ctx.accounts.engine_authority.to_account_info(),
    };
    token::transfer(
        CpiContext::new_with_signer(
            ctx.accounts.token_program.to_account_info(),
            cpi_accounts,
            signer_seeds,
        ),
        amount,
    )?;
    ctx.accounts.user_margin.collateral_balance = post_collateral;

    Ok(())
}

#[derive(Accounts)]
pub struct WithdrawCollateral<'info> {
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
    /// CHECK: engine authority PDA.
    #[account(seeds = [b"engine-authority"], bump)]
    pub engine_authority: UncheckedAccount<'info>,
    #[account(mut, address = engine_config.collateral_vault)]
    pub collateral_vault: Account<'info, TokenAccount>,
    #[account(
        mut,
        constraint = user_token_account.mint == engine_config.usdc_mint @ ErrorCode::InvalidCollateralMint,
        constraint = user_token_account.owner == user.key() @ ErrorCode::Unauthorized,
    )]
    pub user_token_account: Account<'info, TokenAccount>,
    pub token_program: Program<'info, Token>,
}
