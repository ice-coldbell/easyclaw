use anchor_lang::prelude::*;

use crate::{
    error::ErrorCode,
    state::{PositionLeg, Side, UserMarketPosition},
};

pub fn apply_fill_to_position(
    position: &mut Account<UserMarketPosition>,
    side: Side,
    qty: u64,
    notional: u64,
) -> Result<()> {
    match side {
        Side::Buy => {
            position.long_qty = position
                .long_qty
                .checked_add(qty)
                .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
            position.long_entry_notional = position
                .long_entry_notional
                .checked_add(notional as u128)
                .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
        }
        Side::Sell => {
            position.short_qty = position
                .short_qty
                .checked_add(qty)
                .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
            position.short_entry_notional = position
                .short_entry_notional
                .checked_add(notional as u128)
                .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
        }
    }
    Ok(())
}

pub fn reduce_position(
    position: &mut Account<UserMarketPosition>,
    leg: PositionLeg,
    close_qty: u64,
) -> Result<u64> {
    match leg {
        PositionLeg::Long => {
            require!(position.long_qty >= close_qty, ErrorCode::InvalidCloseQty);
            let reduced_notional = ((position.long_entry_notional)
                .checked_mul(close_qty as u128)
                .ok_or_else(|| error!(ErrorCode::MathOverflow))?)
            .checked_div(position.long_qty as u128)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?
                as u64;

            position.long_qty = position
                .long_qty
                .checked_sub(close_qty)
                .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
            position.long_entry_notional = position
                .long_entry_notional
                .checked_sub(reduced_notional as u128)
                .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

            Ok(reduced_notional)
        }
        PositionLeg::Short => {
            require!(position.short_qty >= close_qty, ErrorCode::InvalidCloseQty);
            let reduced_notional = ((position.short_entry_notional)
                .checked_mul(close_qty as u128)
                .ok_or_else(|| error!(ErrorCode::MathOverflow))?)
            .checked_div(position.short_qty as u128)
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?
                as u64;

            position.short_qty = position
                .short_qty
                .checked_sub(close_qty)
                .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
            position.short_entry_notional = position
                .short_entry_notional
                .checked_sub(reduced_notional as u128)
                .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

            Ok(reduced_notional)
        }
    }
}
