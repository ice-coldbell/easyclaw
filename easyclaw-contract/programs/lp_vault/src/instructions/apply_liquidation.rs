use anchor_lang::prelude::*;
use anchor_spl::token::TokenAccount;

use crate::{
    error::ErrorCode,
    helpers::{assert_engine_authority, mul_bps},
    state::{KeeperRebate, Pool},
};

pub fn handler(
    ctx: Context<ApplyLiquidation>,
    _market_id: u64,
    _user: Pubkey,
    penalty: u64,
    bad_debt: u64,
) -> Result<()> {
    assert_engine_authority(&ctx.accounts.pool, &ctx.accounts.engine_authority)?;

    let pool = &mut ctx.accounts.pool;
    let keeper_portion = mul_bps(penalty, 1_000)?; // 10%
    let insurance_portion = penalty
        .checked_sub(keeper_portion)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    let rebate = &mut ctx.accounts.keeper_rebate;
    require_keys_eq!(rebate.pool, pool.key(), ErrorCode::InvalidKeeperRebate);
    require_keys_eq!(
        rebate.keeper,
        ctx.accounts.keeper.key(),
        ErrorCode::InvalidKeeperRebate
    );

    rebate.amount = rebate
        .amount
        .checked_add(keeper_portion)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    pool.pending_keeper_rebates = pool
        .pending_keeper_rebates
        .checked_add(keeper_portion)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    require!(
        insurance_portion <= ctx.accounts.insurance_vault.amount,
        ErrorCode::InsufficientInsuranceVault
    );

    if bad_debt > 0 {
        require!(
            bad_debt <= ctx.accounts.insurance_vault.amount,
            ErrorCode::InsuranceShortfall
        );
    }

    Ok(())
}

#[derive(Accounts)]
pub struct ApplyLiquidation<'info> {
    pub engine_authority: Signer<'info>,
    #[account(
        mut,
        seeds = [b"pool", pool.usdc_mint.as_ref()],
        bump = pool.bump,
    )]
    pub pool: Account<'info, Pool>,
    /// CHECK: used for keeper rebate identity.
    pub keeper: UncheckedAccount<'info>,
    #[account(
        mut,
        seeds = [b"keeper-rebate", pool.key().as_ref(), keeper.key().as_ref()],
        bump = keeper_rebate.bump,
    )]
    pub keeper_rebate: Account<'info, KeeperRebate>,
    #[account(address = pool.insurance_vault)]
    pub insurance_vault: Account<'info, TokenAccount>,
}
