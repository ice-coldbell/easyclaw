use anchor_lang::prelude::*;
use market_registry::Market;

use crate::{error::ErrorCode, helpers::mul_bps_u64};

fn estimate_order_notional(margin: u64, _market: &Account<Market>) -> Result<u64> {
    require!(margin > 0, ErrorCode::InvalidAmount);
    Ok(margin)
}

pub fn estimate_order_reservation(
    reduce_only: bool,
    margin: u64,
    market: &Account<Market>,
) -> Result<u64> {
    if reduce_only {
        return Ok(0);
    }

    let notional = estimate_order_notional(margin, market)?;
    let imr = mul_bps_u64(notional, market.risk_params.imr_bps as u64)?;
    let fee = mul_bps_u64(notional, market.fee_params.taker_fee_bps as u64)?;

    imr.checked_add(fee)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))
}
