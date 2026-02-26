use anchor_lang::prelude::*;

use crate::{constants::BPS_DENOM, error::ErrorCode};

pub fn mul_bps_u64(value: u64, bps: u64) -> Result<u64> {
    ((value as u128)
        .checked_mul(bps as u128)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?)
    .checked_div(BPS_DENOM)
    .ok_or_else(|| error!(ErrorCode::MathOverflow))
    .map(|v| v as u64)
}

pub fn abs_diff(a: u64, b: u64) -> u64 {
    if a > b {
        a - b
    } else {
        b - a
    }
}
