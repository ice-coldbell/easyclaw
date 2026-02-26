use anchor_lang::prelude::*;

use crate::error::ErrorCode;

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, InitSpace, PartialEq, Eq)]
pub enum MarketStatus {
    Active,
    Paused,
    Halted,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, InitSpace)]
pub struct FeeSplit {
    pub lp_bps: u16,
    pub insurance_bps: u16,
    pub protocol_bps: u16,
}

impl FeeSplit {
    pub fn validate(&self) -> Result<()> {
        let sum = self
            .lp_bps
            .checked_add(self.insurance_bps)
            .and_then(|x| x.checked_add(self.protocol_bps))
            .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
        require!(sum == 10_000, ErrorCode::InvalidFeeSplit);
        Ok(())
    }
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, InitSpace)]
pub struct RiskParams {
    pub max_leverage: u16,
    pub imr_bps: u16,
    pub mmr_bps: u16,
    pub oi_cap: u64,
    pub skew_cap: u64,
    pub max_trade_notional: u64,
}

impl RiskParams {
    pub fn validate(&self) -> Result<()> {
        require!(self.max_leverage >= 1, ErrorCode::InvalidRiskParams);
        require!(self.imr_bps > self.mmr_bps, ErrorCode::InvalidRiskParams);
        require!(self.imr_bps <= 10_000, ErrorCode::InvalidRiskParams);
        require!(self.mmr_bps <= 10_000, ErrorCode::InvalidRiskParams);
        require!(self.oi_cap > 0, ErrorCode::InvalidRiskParams);
        require!(self.max_trade_notional > 0, ErrorCode::InvalidRiskParams);
        Ok(())
    }
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, InitSpace)]
pub struct PricingParams {
    pub base_spread_bps: u16,
    pub skew_coeff_bps: u16,
    pub max_fill_deviation_bps: u16,
    pub max_oracle_staleness_sec: i64,
    pub max_conf_bps: u16,
}

impl PricingParams {
    pub fn validate(&self) -> Result<()> {
        require!(
            self.max_fill_deviation_bps <= 5_000,
            ErrorCode::InvalidPricingParams
        );
        require!(
            self.max_oracle_staleness_sec > 0,
            ErrorCode::InvalidPricingParams
        );
        require!(self.max_conf_bps <= 10_000, ErrorCode::InvalidPricingParams);
        Ok(())
    }
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, InitSpace)]
pub struct FundingParams {
    pub interval_sec: i64,
    pub funding_velocity_cap_bps_per_day: i64,
    pub premium_clamp_bps: i64,
}

impl FundingParams {
    pub fn validate(&self) -> Result<()> {
        require!(self.interval_sec > 0, ErrorCode::InvalidFundingParams);
        require!(
            self.funding_velocity_cap_bps_per_day >= 0,
            ErrorCode::InvalidFundingParams
        );
        require!(self.premium_clamp_bps >= 0, ErrorCode::InvalidFundingParams);
        Ok(())
    }
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, InitSpace)]
pub struct FeeParams {
    pub taker_fee_bps: u16,
    pub maker_fee_bps: u16,
}

impl FeeParams {
    pub fn validate(&self) -> Result<()> {
        require!(self.taker_fee_bps <= 1_000, ErrorCode::InvalidFeeParams);
        require!(self.maker_fee_bps <= 1_000, ErrorCode::InvalidFeeParams);
        Ok(())
    }
}
