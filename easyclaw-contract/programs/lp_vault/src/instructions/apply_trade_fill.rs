use anchor_lang::prelude::*;
use anchor_spl::token::TokenAccount;

use crate::{
    error::ErrorCode,
    helpers::{assert_engine_authority, mul_bps},
    state::{KeeperRebate, Pool},
};

pub fn handler(
    ctx: Context<ApplyTradeFill>,
    _market_id: u64,
    _user: Pubkey,
    _order_id: u64,
    _notional: u64,
    fee: u64,
    pnl_delta: i64,
) -> Result<()> {
    assert_engine_authority(&ctx.accounts.pool, &ctx.accounts.engine_authority)?;

    let pool = &mut ctx.accounts.pool;
    let lp_fee = mul_bps(fee, pool.lp_fee_bps as u64)?;
    let insurance_fee = mul_bps(fee, pool.insurance_fee_bps as u64)?;
    let protocol_fee = fee
        .checked_sub(lp_fee)
        .and_then(|x| x.checked_sub(insurance_fee))
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    pool.total_trading_fees = pool
        .total_trading_fees
        .checked_add(fee)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    pool.cumulative_trader_pnl = pool
        .cumulative_trader_pnl
        .checked_add(pnl_delta as i128)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    // Fee splits are transferred into dedicated vaults by order_engine.
    require!(
        protocol_fee <= ctx.accounts.protocol_fee_vault.amount,
        ErrorCode::InsufficientProtocolFeeVault
    );
    require!(
        insurance_fee <= ctx.accounts.insurance_vault.amount,
        ErrorCode::InsufficientInsuranceVault
    );

    if pool.execution_rebate_usdc > 0 {
        let rebate = &mut ctx.accounts.keeper_rebate;
        require_keys_eq!(rebate.pool, pool.key(), ErrorCode::InvalidKeeperRebate);
        require_keys_eq!(
            rebate.keeper,
            ctx.accounts.keeper.key(),
            ErrorCode::InvalidKeeperRebate
        );

        rebate.amount = rebate
            .amount
            .checked_add(pool.execution_rebate_usdc)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
        pool.pending_keeper_rebates = pool
            .pending_keeper_rebates
            .checked_add(pool.execution_rebate_usdc)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    }

    Ok(())
}

#[derive(Accounts)]
pub struct ApplyTradeFill<'info> {
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
    #[account(address = pool.liquidity_vault)]
    pub liquidity_vault: Account<'info, TokenAccount>,
    #[account(address = pool.insurance_vault)]
    pub insurance_vault: Account<'info, TokenAccount>,
    #[account(address = pool.protocol_fee_vault)]
    pub protocol_fee_vault: Account<'info, TokenAccount>,
}
