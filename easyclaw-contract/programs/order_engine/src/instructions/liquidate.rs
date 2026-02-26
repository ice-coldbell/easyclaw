use anchor_lang::prelude::*;
use anchor_spl::token::{Token, TokenAccount};
use lp_vault::program::LpVault;
use market_registry::program::MarketRegistry;

use crate::{
    error::ErrorCode,
    helpers::{
        assert_keeper_only, mul_bps_u64, reduce_position, settle_user_funding,
        transfer_from_collateral, update_funding_index,
    },
    state::{EngineConfig, MarketFundingState, PositionLeg, UserMargin, UserMarketPosition},
};

pub fn handler(
    ctx: Context<Liquidate>,
    market_id: u64,
    leg: PositionLeg,
    close_qty: u64,
) -> Result<()> {
    require!(close_qty > 0, ErrorCode::InvalidAmount);

    let now = Clock::get()?.unix_timestamp;
    let market = &ctx.accounts.market;
    let funding_state = &mut ctx.accounts.market_funding_state;
    let margin = &mut ctx.accounts.user_margin;
    let position = &mut ctx.accounts.user_market_position;
    let keeper_rebate = &ctx.accounts.keeper_rebate;

    require!(market.market_id == market_id, ErrorCode::MarketMismatch);
    require!(position.market_id == market_id, ErrorCode::MarketMismatch);
    require_keys_eq!(
        position.user_margin,
        margin.key(),
        ErrorCode::PositionOwnerMismatch
    );
    require!(
        market.status == market_registry::MarketStatus::Active,
        ErrorCode::MarketNotActive
    );
    require!(!funding_state.halted, ErrorCode::MarketHaltedLocal);
    require_keys_eq!(
        keeper_rebate.pool,
        ctx.accounts.lp_pool.key(),
        ErrorCode::InvalidKeeperRebateAccount
    );
    require_keys_eq!(
        keeper_rebate.keeper,
        ctx.accounts.executor.key(),
        ErrorCode::InvalidKeeperRebateAccount
    );

    assert_keeper_only(&ctx.accounts.executor, &ctx.accounts.keeper_set)?;

    update_funding_index(
        funding_state,
        now,
        &market.funding_params,
        market.risk_params.oi_cap,
    )?;
    settle_user_funding(position, funding_state, margin)?;

    let mmr_required = mul_bps_u64(margin.total_notional, market.risk_params.mmr_bps as u64)?;
    require!(
        margin.collateral_balance < mmr_required,
        ErrorCode::NotLiquidatable
    );

    let reduced_notional = reduce_position(position, leg, close_qty)?;
    require!(reduced_notional > 0, ErrorCode::InvalidAmount);

    margin.total_notional = margin
        .total_notional
        .checked_sub(reduced_notional)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    funding_state.open_interest = funding_state
        .open_interest
        .checked_sub(reduced_notional)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    funding_state.skew = match leg {
        PositionLeg::Long => funding_state
            .skew
            .checked_sub(reduced_notional as i128)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?,
        PositionLeg::Short => funding_state
            .skew
            .checked_add(reduced_notional as i128)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?,
    };

    let penalty = mul_bps_u64(
        reduced_notional,
        ctx.accounts.engine_config.liquidation_penalty_bps as u64,
    )?;
    let keeper_portion = mul_bps_u64(penalty, 1_000)?;
    let insurance_portion = penalty
        .checked_sub(keeper_portion)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    let mut bad_debt = 0u64;
    if margin.collateral_balance >= penalty {
        margin.collateral_balance = margin
            .collateral_balance
            .checked_sub(penalty)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    } else {
        bad_debt = penalty
            .checked_sub(margin.collateral_balance)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
        margin.collateral_balance = 0;
    }

    transfer_from_collateral(
        &ctx.accounts.token_program,
        &ctx.accounts.collateral_vault,
        &ctx.accounts.lp_insurance_vault,
        &ctx.accounts.engine_authority,
        ctx.bumps.engine_authority,
        insurance_portion,
    )?;

    transfer_from_collateral(
        &ctx.accounts.token_program,
        &ctx.accounts.collateral_vault,
        &ctx.accounts.lp_protocol_fee_vault,
        &ctx.accounts.engine_authority,
        ctx.bumps.engine_authority,
        keeper_portion,
    )?;

    let insurance_after_credit = ctx
        .accounts
        .lp_insurance_vault
        .amount
        .checked_add(insurance_portion)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    if bad_debt > insurance_after_credit {
        funding_state.halted = true;
        return err!(ErrorCode::InsuranceShortfallMarketHalted);
    }

    cpi_apply_liquidation(&ctx, market_id, penalty, bad_debt)?;

    Ok(())
}

fn cpi_apply_liquidation(
    ctx: &Context<Liquidate>,
    market_id: u64,
    penalty: u64,
    bad_debt: u64,
) -> Result<()> {
    let seeds: &[&[u8]] = &[b"engine-authority", &[ctx.bumps.engine_authority]];
    let signer_seeds = &[seeds];

    let cpi_accounts = lp_vault::cpi::accounts::ApplyLiquidation {
        engine_authority: ctx.accounts.engine_authority.to_account_info(),
        pool: ctx.accounts.lp_pool.to_account_info(),
        keeper: ctx.accounts.executor.to_account_info(),
        keeper_rebate: ctx.accounts.keeper_rebate.to_account_info(),
        insurance_vault: ctx.accounts.lp_insurance_vault.to_account_info(),
    };

    lp_vault::cpi::apply_liquidation(
        CpiContext::new_with_signer(
            ctx.accounts.lp_vault_program.to_account_info(),
            cpi_accounts,
            signer_seeds,
        ),
        market_id,
        ctx.accounts.user_margin.owner,
        penalty,
        bad_debt,
    )
}

#[derive(Accounts)]
#[instruction(market_id: u64)]
pub struct Liquidate<'info> {
    pub executor: Signer<'info>,
    #[account(
        seeds = [b"engine-config"],
        bump = engine_config.bump,
    )]
    pub engine_config: Box<Account<'info, EngineConfig>>,
    pub market_registry_program: Program<'info, MarketRegistry>,
    #[account(address = engine_config.keeper_set)]
    pub keeper_set: Box<Account<'info, market_registry::KeeperSet>>,
    #[account(
        seeds = [b"market".as_ref(), &market_id.to_le_bytes()],
        seeds::program = market_registry_program.key(),
        bump = market.bump,
    )]
    pub market: Box<Account<'info, market_registry::Market>>,
    #[account(
        mut,
        seeds = [b"funding".as_ref(), &market_id.to_le_bytes()],
        bump = market_funding_state.bump,
    )]
    pub market_funding_state: Box<Account<'info, MarketFundingState>>,
    #[account(
        mut,
        seeds = [b"user-margin", user_margin.owner.as_ref()],
        bump = user_margin.bump,
    )]
    pub user_margin: Box<Account<'info, UserMargin>>,
    #[account(
        mut,
        seeds = [b"user-market-pos", user_margin.key().as_ref(), &market_id.to_le_bytes()],
        bump = user_market_position.bump,
    )]
    pub user_market_position: Box<Account<'info, UserMarketPosition>>,
    /// CHECK: engine authority PDA.
    #[account(seeds = [b"engine-authority"], bump)]
    pub engine_authority: UncheckedAccount<'info>,
    #[account(mut, address = engine_config.collateral_vault)]
    pub collateral_vault: Box<Account<'info, TokenAccount>>,

    pub lp_vault_program: Program<'info, LpVault>,
    #[account(mut, address = engine_config.lp_pool)]
    pub lp_pool: Box<Account<'info, lp_vault::Pool>>,
    #[account(mut, address = engine_config.lp_insurance_vault)]
    pub lp_insurance_vault: Box<Account<'info, TokenAccount>>,
    #[account(mut, address = engine_config.lp_protocol_fee_vault)]
    pub lp_protocol_fee_vault: Box<Account<'info, TokenAccount>>,
    #[account(mut)]
    pub keeper_rebate: Box<Account<'info, lp_vault::KeeperRebate>>,
    pub token_program: Program<'info, Token>,
}
