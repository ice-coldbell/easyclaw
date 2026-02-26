use anchor_lang::prelude::*;

use crate::{
    constants::SYMBOL_LEN,
    state::{FeeParams, FundingParams, MarketStatus, PricingParams, RiskParams},
};

#[account]
#[derive(InitSpace)]
pub struct Market {
    pub market_id: u64,
    pub symbol: [u8; SYMBOL_LEN],
    pub pyth_feed: Pubkey,
    pub status: MarketStatus,
    pub risk_params: RiskParams,
    pub pricing_params: PricingParams,
    pub funding_params: FundingParams,
    pub fee_params: FeeParams,
    pub bump: u8,
}
