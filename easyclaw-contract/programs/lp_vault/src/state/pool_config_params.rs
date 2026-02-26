use anchor_lang::prelude::*;

use crate::{constants::BPS_DENOM, error::ErrorCode};

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, InitSpace)]
pub struct PoolConfigParams {
    pub cooldown_secs: i64,
    pub min_liquidity_buffer_bps: u16,
    pub lp_fee_bps: u16,
    pub insurance_fee_bps: u16,
    pub protocol_fee_bps: u16,
    pub execution_rebate_usdc: u64,
}

impl PoolConfigParams {
    pub fn validate(&self) -> Result<()> {
        require!(self.cooldown_secs >= 0, ErrorCode::InvalidCooldown);
        require!(
            self.min_liquidity_buffer_bps <= BPS_DENOM as u16,
            ErrorCode::InvalidBps
        );

        let sum = self
            .lp_fee_bps
            .checked_add(self.insurance_fee_bps)
            .and_then(|x| x.checked_add(self.protocol_fee_bps))
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
        require!(sum == BPS_DENOM as u16, ErrorCode::InvalidBps);

        Ok(())
    }
}
