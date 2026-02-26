use anchor_lang::prelude::*;

#[account]
#[derive(InitSpace)]
pub struct MarketFundingState {
    pub market_id: u64,
    pub funding_index: i128,
    pub last_update_ts: i64,
    pub open_interest: u64,
    pub skew: i128,
    pub halted: bool,
    pub bump: u8,
}
