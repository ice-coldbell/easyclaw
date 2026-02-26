use anchor_lang::prelude::*;

use crate::{constants::BPS_DENOM, error::ErrorCode, state::Pool};

pub fn require_admin(admin: &Signer<'_>, pool: &Account<Pool>) -> Result<()> {
    require_keys_eq!(admin.key(), pool.admin, ErrorCode::Unauthorized);
    Ok(())
}

pub fn assert_engine_authority(pool: &Account<Pool>, engine_authority: &Signer<'_>) -> Result<()> {
    require_keys_eq!(
        pool.engine_authority,
        engine_authority.key(),
        ErrorCode::UnauthorizedEngine
    );
    Ok(())
}

pub fn mul_bps(value: u64, bps: u64) -> Result<u64> {
    ((value as u128)
        .checked_mul(bps as u128)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?)
    .checked_div(BPS_DENOM as u128)
    .ok_or_else(|| error!(ErrorCode::MathOverflow))
    .map(|v| v as u64)
}
