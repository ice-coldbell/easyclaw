use anchor_lang::prelude::*;

use crate::{
    constants::{BPS_DENOM, FUNDING_SCALE},
    error::ErrorCode,
    state::{MarketFundingState, UserMargin, UserMarketPosition},
};

pub fn update_funding_index(
    funding_state: &mut Account<MarketFundingState>,
    now: i64,
    params: &market_registry::FundingParams,
    oi_cap: u64,
) -> Result<()> {
    require!(params.interval_sec > 0, ErrorCode::InvalidFundingParams);
    if now <= funding_state.last_update_ts {
        return Ok(());
    }

    let elapsed = now
        .checked_sub(funding_state.last_update_ts)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    let premium_bps = if oi_cap == 0 {
        0i128
    } else {
        ((funding_state.skew)
            .checked_mul(BPS_DENOM as i128)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?)
        .checked_div(oi_cap as i128)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?
    };

    let clamped = premium_bps
        .max(-(params.premium_clamp_bps as i128))
        .min(params.premium_clamp_bps as i128);

    let velocity_bound = ((params.funding_velocity_cap_bps_per_day as i128)
        .checked_mul(elapsed as i128)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?)
    .checked_div(86_400)
    .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    let interval_scaled = clamped
        .checked_mul(FUNDING_SCALE)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?
        .checked_mul(elapsed as i128)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?
        .checked_div(params.interval_sec as i128)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    let max_scaled = velocity_bound
        .checked_mul(FUNDING_SCALE)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    let delta = interval_scaled.max(-max_scaled).min(max_scaled);

    funding_state.funding_index = funding_state
        .funding_index
        .checked_add(delta)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    funding_state.last_update_ts = now;

    Ok(())
}

pub fn settle_user_funding(
    position: &mut Account<UserMarketPosition>,
    funding_state: &Account<MarketFundingState>,
    margin: &mut Account<UserMargin>,
) -> Result<()> {
    let delta_long = funding_state
        .funding_index
        .checked_sub(position.last_funding_index_long)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    let delta_short = funding_state
        .funding_index
        .checked_sub(position.last_funding_index_short)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    // Positive funding index means longs pay and shorts receive.
    let long_payment = ((position.long_qty as i128)
        .checked_mul(delta_long)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?)
    .checked_div(FUNDING_SCALE)
    .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    let short_payment = ((position.short_qty as i128)
        .checked_mul(delta_short)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?)
    .checked_div(FUNDING_SCALE)
    .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    let net_delta = short_payment
        .checked_sub(long_payment)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    if net_delta >= 0 {
        margin.collateral_balance = margin
            .collateral_balance
            .checked_add(net_delta as u64)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    } else {
        let debit = (-net_delta) as u64;
        margin.collateral_balance = margin.collateral_balance.saturating_sub(debit);
    }

    position.last_funding_index_long = funding_state.funding_index;
    position.last_funding_index_short = funding_state.funding_index;

    Ok(())
}
