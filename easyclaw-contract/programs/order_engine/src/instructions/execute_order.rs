use anchor_lang::prelude::*;
use anchor_spl::token::{Token, TokenAccount};
use lp_vault::program::LpVault;
use market_registry::program::MarketRegistry;

use crate::{
    constants::PRICE_SCALE,
    error::ErrorCode,
    helpers::{
        apply_fill_to_position, assert_executor_authorized, estimate_order_reservation,
        mul_bps_u64, read_oracle_price_update, reduce_position, settle_user_funding,
        transfer_from_collateral, update_funding_index, validate_impact_price, validate_oracle,
        validate_order_price,
    },
    state::{
        EngineConfig, MarketFundingState, Order, OrderStatus, PositionLeg, UserMargin,
        UserMarketPosition,
    },
};

#[allow(clippy::too_many_arguments)]
pub fn handler(
    ctx: Context<ExecuteOrder>,
    fill_price: u64,
    oracle_price: u64,
    oracle_conf: u64,
    oracle_publish_time: i64,
) -> Result<()> {
    require!(fill_price > 0, ErrorCode::InvalidPrice);

    let clock = Clock::get()?;
    let now = clock.unix_timestamp;
    let global_config = &ctx.accounts.global_config;
    let keeper_set = &ctx.accounts.keeper_set;
    let market = &ctx.accounts.market;
    let funding_state = &mut ctx.accounts.market_funding_state;
    let margin = &mut ctx.accounts.user_margin;
    let order = &mut ctx.accounts.order;
    let position = &mut ctx.accounts.user_market_position;
    let keeper_rebate = &ctx.accounts.keeper_rebate;
    let order_side = order.side;
    let order_type = order.order_type;
    let order_price = order.price;
    let order_margin = order.margin;

    assert_executor_authorized(&ctx.accounts.executor, global_config, keeper_set)?;
    require!(!global_config.global_pause, ErrorCode::GlobalPaused);
    require!(
        market.status == market_registry::MarketStatus::Active,
        ErrorCode::MarketNotActive
    );
    require!(!funding_state.halted, ErrorCode::MarketHaltedLocal);

    require!(order.status == OrderStatus::Open, ErrorCode::OrderNotOpen);
    require!(
        order.market_id == market.market_id,
        ErrorCode::MarketMismatch
    );

    let reserved_collateral = estimate_order_reservation(order.reduce_only, order.margin, market)?;

    if now > order.expires_at {
        margin.collateral_balance = margin
            .collateral_balance
            .checked_add(reserved_collateral)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
        order.status = OrderStatus::Expired;
        return Ok(());
    }

    require_keys_eq!(margin.owner, order.user, ErrorCode::MarginOrderMismatch);
    require_keys_eq!(
        order.user_margin,
        margin.key(),
        ErrorCode::MarginOrderMismatch
    );
    require_keys_eq!(
        position.user_margin,
        margin.key(),
        ErrorCode::PositionOwnerMismatch
    );
    require!(
        position.market_id == market.market_id,
        ErrorCode::MarketMismatch
    );
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

    margin.collateral_balance = margin
        .collateral_balance
        .checked_add(reserved_collateral)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    let (oracle_price, oracle_conf, oracle_publish_time) = read_oracle_price_update(
        market,
        &ctx.accounts.oracle_price_update,
        &clock,
        oracle_price,
        oracle_conf,
        oracle_publish_time,
    )?;

    validate_oracle(
        market,
        now,
        fill_price,
        oracle_price,
        oracle_conf,
        oracle_publish_time,
    )?;

    let notional = order_margin;
    require!(notional > 0, ErrorCode::InvalidAmount);
    require!(
        notional <= market.risk_params.max_trade_notional,
        ErrorCode::MaxTradeNotionalExceeded
    );

    let raw_qty = ((notional as u128)
        .checked_mul(PRICE_SCALE)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?)
    .checked_div(fill_price as u128)
    .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    let order_qty: u64 = raw_qty
        .try_into()
        .map_err(|_| error!(ErrorCode::MathOverflow))?;
    require!(order_qty > 0, ErrorCode::InvalidAmount);

    validate_order_price(order_side, order_type, order_price, fill_price)?;

    update_funding_index(
        funding_state,
        now,
        &market.funding_params,
        market.risk_params.oi_cap,
    )?;
    settle_user_funding(position, funding_state, margin)?;

    if order.reduce_only {
        let close_leg = match order_side {
            crate::state::Side::Buy => PositionLeg::Short,
            crate::state::Side::Sell => PositionLeg::Long,
        };
        let reduced_notional = reduce_position(position, close_leg, order_qty)?;

        margin.total_notional = margin
            .total_notional
            .checked_sub(reduced_notional)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

        funding_state.open_interest = funding_state
            .open_interest
            .checked_sub(reduced_notional)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

        funding_state.skew = match close_leg {
            PositionLeg::Long => funding_state
                .skew
                .checked_sub(reduced_notional as i128)
                .ok_or_else(|| error!(ErrorCode::MathOverflow))?,
            PositionLeg::Short => funding_state
                .skew
                .checked_add(reduced_notional as i128)
                .ok_or_else(|| error!(ErrorCode::MathOverflow))?,
        };

        let fee = mul_bps_u64(notional, market.fee_params.taker_fee_bps as u64)?;
        require!(
            margin.collateral_balance >= fee,
            ErrorCode::InsufficientCollateral
        );
        margin.collateral_balance = margin
            .collateral_balance
            .checked_sub(fee)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

        order.status = OrderStatus::Executed;
        transfer_fee_split(&ctx, fee)?;
        cpi_apply_trade_fill(&ctx, notional, fee)?;

        return Ok(());
    }

    let projected_oi = funding_state
        .open_interest
        .checked_add(notional)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    require!(
        projected_oi <= market.risk_params.oi_cap,
        ErrorCode::OiCapExceeded
    );

    let projected_skew = match order_side {
        crate::state::Side::Buy => funding_state
            .skew
            .checked_add(notional as i128)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?,
        crate::state::Side::Sell => funding_state
            .skew
            .checked_sub(notional as i128)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?,
    };

    require!(
        projected_skew.unsigned_abs() <= market.risk_params.skew_cap as u128,
        ErrorCode::SkewCapExceeded
    );

    validate_impact_price(
        order_side,
        fill_price,
        oracle_price,
        projected_skew,
        projected_oi,
        &market.pricing_params,
    )?;

    let fee = mul_bps_u64(notional, market.fee_params.taker_fee_bps as u64)?;
    require!(
        margin.collateral_balance >= fee,
        ErrorCode::InsufficientCollateral
    );

    margin.collateral_balance = margin
        .collateral_balance
        .checked_sub(fee)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    let new_total_notional = margin
        .total_notional
        .checked_add(notional)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    let imr_required = mul_bps_u64(new_total_notional, market.risk_params.imr_bps as u64)?;
    require!(
        margin.collateral_balance >= imr_required,
        ErrorCode::MarginRequirementViolation
    );

    let leverage_num = new_total_notional
        .checked_mul(1)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    let leverage_den = margin.collateral_balance.max(1);
    require!(
        leverage_num <= leverage_den.saturating_mul(market.risk_params.max_leverage as u64),
        ErrorCode::LeverageExceeded
    );

    apply_fill_to_position(position, order_side, order_qty, notional)?;

    funding_state.open_interest = projected_oi;
    funding_state.skew = projected_skew;
    margin.total_notional = new_total_notional;
    order.status = OrderStatus::Executed;

    transfer_fee_split(&ctx, fee)?;
    cpi_apply_trade_fill(&ctx, notional, fee)?;

    Ok(())
}

fn transfer_fee_split(ctx: &Context<ExecuteOrder>, fee: u64) -> Result<()> {
    if fee == 0 {
        return Ok(());
    }

    let lp_fee = mul_bps_u64(fee, ctx.accounts.lp_pool.lp_fee_bps as u64)?;
    let insurance_fee = mul_bps_u64(fee, ctx.accounts.lp_pool.insurance_fee_bps as u64)?;
    let protocol_fee = fee
        .checked_sub(lp_fee)
        .and_then(|x| x.checked_sub(insurance_fee))
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    transfer_from_collateral(
        &ctx.accounts.token_program,
        &ctx.accounts.collateral_vault,
        &ctx.accounts.lp_liquidity_vault,
        &ctx.accounts.engine_authority,
        ctx.bumps.engine_authority,
        lp_fee,
    )?;

    transfer_from_collateral(
        &ctx.accounts.token_program,
        &ctx.accounts.collateral_vault,
        &ctx.accounts.lp_insurance_vault,
        &ctx.accounts.engine_authority,
        ctx.bumps.engine_authority,
        insurance_fee,
    )?;

    transfer_from_collateral(
        &ctx.accounts.token_program,
        &ctx.accounts.collateral_vault,
        &ctx.accounts.lp_protocol_fee_vault,
        &ctx.accounts.engine_authority,
        ctx.bumps.engine_authority,
        protocol_fee,
    )?;

    Ok(())
}

fn cpi_apply_trade_fill(ctx: &Context<ExecuteOrder>, notional: u64, fee: u64) -> Result<()> {
    let seeds: &[&[u8]] = &[b"engine-authority", &[ctx.bumps.engine_authority]];
    let signer_seeds = &[seeds];

    let cpi_accounts = lp_vault::cpi::accounts::ApplyTradeFill {
        engine_authority: ctx.accounts.engine_authority.to_account_info(),
        pool: ctx.accounts.lp_pool.to_account_info(),
        keeper: ctx.accounts.executor.to_account_info(),
        keeper_rebate: ctx.accounts.keeper_rebate.to_account_info(),
        liquidity_vault: ctx.accounts.lp_liquidity_vault.to_account_info(),
        insurance_vault: ctx.accounts.lp_insurance_vault.to_account_info(),
        protocol_fee_vault: ctx.accounts.lp_protocol_fee_vault.to_account_info(),
    };

    lp_vault::cpi::apply_trade_fill(
        CpiContext::new_with_signer(
            ctx.accounts.lp_vault_program.to_account_info(),
            cpi_accounts,
            signer_seeds,
        ),
        ctx.accounts.market.market_id,
        ctx.accounts.user_margin.owner,
        ctx.accounts.order.id,
        notional,
        fee,
        0,
    )
}

#[derive(Accounts)]
pub struct ExecuteOrder<'info> {
    pub executor: Signer<'info>,
    #[account(
        seeds = [b"engine-config"],
        bump = engine_config.bump,
    )]
    pub engine_config: Box<Account<'info, EngineConfig>>,
    pub market_registry_program: Program<'info, MarketRegistry>,
    #[account(address = engine_config.registry_global_config)]
    pub global_config: Box<Account<'info, market_registry::GlobalConfig>>,
    #[account(address = engine_config.keeper_set)]
    pub keeper_set: Box<Account<'info, market_registry::KeeperSet>>,
    #[account(mut)]
    pub order: Box<Account<'info, Order>>,
    #[account(
        seeds = [b"market".as_ref(), &order.market_id.to_le_bytes()],
        seeds::program = market_registry_program.key(),
        bump = market.bump,
    )]
    pub market: Box<Account<'info, market_registry::Market>>,
    /// CHECK: validated in `read_oracle_price_update` helper (owner/discriminator/feed id/staleness or fallback source).
    pub oracle_price_update: UncheckedAccount<'info>,
    #[account(
        mut,
        seeds = [b"funding".as_ref(), &order.market_id.to_le_bytes()],
        bump = market_funding_state.bump,
    )]
    pub market_funding_state: Box<Account<'info, MarketFundingState>>,
    #[account(
        mut,
        seeds = [b"user-margin", order.user.as_ref()],
        bump = user_margin.bump,
    )]
    pub user_margin: Box<Account<'info, UserMargin>>,
    #[account(
        mut,
        seeds = [b"user-market-pos", user_margin.key().as_ref(), &order.market_id.to_le_bytes()],
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
    #[account(mut, address = engine_config.lp_liquidity_vault)]
    pub lp_liquidity_vault: Box<Account<'info, TokenAccount>>,
    #[account(mut, address = engine_config.lp_insurance_vault)]
    pub lp_insurance_vault: Box<Account<'info, TokenAccount>>,
    #[account(mut, address = engine_config.lp_protocol_fee_vault)]
    pub lp_protocol_fee_vault: Box<Account<'info, TokenAccount>>,
    #[account(mut)]
    pub keeper_rebate: Box<Account<'info, lp_vault::KeeperRebate>>,
    pub token_program: Program<'info, Token>,
}
