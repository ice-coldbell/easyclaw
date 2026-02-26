use anchor_lang::prelude::*;

use crate::{
    constants::{BPS_DENOM, PRICE_SCALE},
    error::ErrorCode,
    helpers::math::abs_diff,
    state::{OrderType, Side},
};

const PYTH_PUSH_ORACLE_PROGRAM_ID: Pubkey = pubkey!("pythWSnswVUd12oZpeFP8e9CVaEqJg25g1Vtc2biRsT");
const PRICE_UPDATE_V2_DISCRIMINATOR: [u8; 8] = [34, 241, 35, 99, 157, 126, 244, 205];

pub fn read_oracle_price_update(
    market: &Account<market_registry::Market>,
    price_update: &UncheckedAccount,
    clock: &Clock,
    fallback_oracle_price: u64,
    fallback_oracle_conf: u64,
    fallback_oracle_publish_time: i64,
) -> Result<(u64, u64, i64)> {
    if price_update.key() == anchor_lang::solana_program::system_program::ID {
        require!(fallback_oracle_price > 0, ErrorCode::InvalidOracle);
        let publish_time = if fallback_oracle_publish_time <= 0 {
            clock.unix_timestamp
        } else {
            fallback_oracle_publish_time
        };
        let age = clock
            .unix_timestamp
            .checked_sub(publish_time)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
        require!(age >= 0, ErrorCode::InvalidOracle);
        require!(
            age <= market.pricing_params.max_oracle_staleness_sec,
            ErrorCode::StaleOracle
        );

        return Ok((fallback_oracle_price, fallback_oracle_conf, publish_time));
    }

    require_keys_eq!(
        *price_update.owner,
        PYTH_PUSH_ORACLE_PROGRAM_ID,
        ErrorCode::InvalidOracle
    );

    let data = price_update
        .try_borrow_data()
        .map_err(|_| error!(ErrorCode::InvalidOracle))?;
    require!(
        data.len() >= PRICE_UPDATE_V2_DISCRIMINATOR.len(),
        ErrorCode::InvalidOracle
    );
    require!(
        data[..8] == PRICE_UPDATE_V2_DISCRIMINATOR,
        ErrorCode::InvalidOracle
    );

    let mut payload = &data[8..];
    let price_update = PriceUpdateV2Wire::deserialize(&mut payload)
        .map_err(|_| error!(ErrorCode::InvalidOracle))?;

    require!(
        matches!(price_update.verification_level, VerificationLevelWire::Full),
        ErrorCode::InvalidOracle
    );

    require!(
        price_update.price_message.feed_id == market.pyth_feed.to_bytes(),
        ErrorCode::InvalidOracle
    );

    let publish_time = price_update.price_message.publish_time;
    let age = clock
        .unix_timestamp
        .checked_sub(publish_time)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    require!(age >= 0, ErrorCode::InvalidOracle);
    require!(
        age <= market.pricing_params.max_oracle_staleness_sec,
        ErrorCode::StaleOracle
    );

    let oracle_price = scale_signed_price_to_engine(
        price_update.price_message.price,
        price_update.price_message.exponent,
    )?;
    let oracle_conf = scale_confidence_to_engine(
        price_update.price_message.conf,
        price_update.price_message.exponent,
    )?;
    Ok((oracle_price, oracle_conf, publish_time))
}

pub fn validate_oracle(
    market: &Account<market_registry::Market>,
    now: i64,
    fill_price: u64,
    oracle_price: u64,
    oracle_conf: u64,
    oracle_publish_time: i64,
) -> Result<()> {
    let age = now
        .checked_sub(oracle_publish_time)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    require!(age >= 0, ErrorCode::InvalidOracle);
    require!(
        age <= market.pricing_params.max_oracle_staleness_sec,
        ErrorCode::StaleOracle
    );

    let conf_bps = ((oracle_conf as u128)
        .checked_mul(BPS_DENOM)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?)
    .checked_div(oracle_price as u128)
    .ok_or_else(|| error!(ErrorCode::MathOverflow))? as u64;
    require!(
        conf_bps <= market.pricing_params.max_conf_bps as u64,
        ErrorCode::OracleConfidenceTooWide
    );

    let deviation = abs_diff(fill_price, oracle_price) as u128;
    let deviation_bps = deviation
        .checked_mul(BPS_DENOM)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?
        .checked_div(oracle_price as u128)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))? as u64;

    require!(
        deviation_bps <= market.pricing_params.max_fill_deviation_bps as u64,
        ErrorCode::FillPriceDeviationTooLarge
    );

    Ok(())
}

pub fn validate_impact_price(
    side: Side,
    fill_price: u64,
    oracle_price: u64,
    projected_skew: i128,
    projected_oi: u64,
    pricing: &market_registry::PricingParams,
) -> Result<()> {
    let skew_ratio_bps = if projected_oi == 0 {
        0
    } else {
        ((projected_skew.unsigned_abs())
            .checked_mul(BPS_DENOM)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?
            .checked_div(projected_oi as u128)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?) as u64
    };

    let skew_impact = ((pricing.skew_coeff_bps as u128)
        .checked_mul(skew_ratio_bps as u128)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?)
    .checked_div(BPS_DENOM)
    .ok_or_else(|| error!(ErrorCode::MathOverflow))? as u64;

    let impact_bps = (pricing.base_spread_bps as u64)
        .checked_add(skew_impact)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    require!(
        impact_bps < BPS_DENOM as u64,
        ErrorCode::ImpactPriceViolation
    );

    let upper = ((oracle_price as u128)
        .checked_mul(BPS_DENOM + impact_bps as u128)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?)
    .checked_div(BPS_DENOM)
    .ok_or_else(|| error!(ErrorCode::MathOverflow))? as u64;

    let lower = ((oracle_price as u128)
        .checked_mul(BPS_DENOM - impact_bps as u128)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?)
    .checked_div(BPS_DENOM)
    .ok_or_else(|| error!(ErrorCode::MathOverflow))? as u64;

    match side {
        Side::Buy => require!(fill_price >= upper, ErrorCode::ImpactPriceViolation),
        Side::Sell => require!(fill_price <= lower, ErrorCode::ImpactPriceViolation),
    }

    Ok(())
}

pub fn validate_order_price(
    side: Side,
    _order_type: OrderType,
    price: u64,
    fill_price: u64,
) -> Result<()> {
    require!(price > 0, ErrorCode::InvalidLimitPrice);

    match side {
        Side::Buy => require!(fill_price <= price, ErrorCode::LimitPriceViolation),
        Side::Sell => require!(fill_price >= price, ErrorCode::LimitPriceViolation),
    }

    Ok(())
}

fn scale_signed_price_to_engine(price: i64, exponent: i32) -> Result<u64> {
    require!(price > 0, ErrorCode::InvalidPrice);
    let base = u128::try_from(price).map_err(|_| error!(ErrorCode::InvalidPrice))?;
    let scaled = scale_unsigned_value(base, exponent)?;
    require!(scaled > 0, ErrorCode::InvalidPrice);
    u64::try_from(scaled).map_err(|_| error!(ErrorCode::MathOverflow))
}

fn scale_confidence_to_engine(conf: u64, exponent: i32) -> Result<u64> {
    let scaled = scale_unsigned_value_ceil(conf as u128, exponent)?;
    u64::try_from(scaled).map_err(|_| error!(ErrorCode::MathOverflow))
}

fn scale_unsigned_value(value: u128, exponent: i32) -> Result<u128> {
    if exponent >= 0 {
        let power = pow10_u128(exponent as u32)?;
        value
            .checked_mul(power)
            .and_then(|x| x.checked_mul(PRICE_SCALE))
            .ok_or_else(|| error!(ErrorCode::MathOverflow))
    } else {
        let divisor = pow10_u128((-exponent) as u32)?;
        value
            .checked_mul(PRICE_SCALE)
            .and_then(|x| x.checked_div(divisor))
            .ok_or_else(|| error!(ErrorCode::MathOverflow))
    }
}

fn scale_unsigned_value_ceil(value: u128, exponent: i32) -> Result<u128> {
    if exponent >= 0 {
        return scale_unsigned_value(value, exponent);
    }

    let divisor = pow10_u128((-exponent) as u32)?;
    let numerator = value
        .checked_mul(PRICE_SCALE)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    ceil_div_u128(numerator, divisor)
}

fn ceil_div_u128(numerator: u128, denominator: u128) -> Result<u128> {
    if denominator == 0 {
        return Err(error!(ErrorCode::MathOverflow));
    }
    let adjusted = numerator
        .checked_add(denominator - 1)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    adjusted
        .checked_div(denominator)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))
}

fn pow10_u128(power: u32) -> Result<u128> {
    10_u128
        .checked_pow(power)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))
}

#[derive(AnchorSerialize, AnchorDeserialize, Copy, Clone, PartialEq, Eq, Debug)]
enum VerificationLevelWire {
    Partial { num_signatures: u8 },
    Full,
}

#[derive(AnchorSerialize, AnchorDeserialize, Copy, Clone, Debug)]
struct PriceFeedMessageWire {
    feed_id: [u8; 32],
    price: i64,
    conf: u64,
    exponent: i32,
    publish_time: i64,
    prev_publish_time: i64,
    ema_price: i64,
    ema_conf: u64,
}

#[derive(AnchorSerialize, AnchorDeserialize, Copy, Clone, Debug)]
struct PriceUpdateV2Wire {
    write_authority: Pubkey,
    verification_level: VerificationLevelWire,
    price_message: PriceFeedMessageWire,
    posted_slot: u64,
}
