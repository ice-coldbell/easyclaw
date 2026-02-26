use anchor_lang::prelude::*;

#[account]
#[derive(InitSpace)]
pub struct UserMarketPosition {
    pub user_margin: Pubkey,
    pub market_id: u64,
    pub long_qty: u64,
    pub long_entry_notional: u128,
    pub short_qty: u64,
    pub short_entry_notional: u128,
    pub last_funding_index_long: i128,
    pub last_funding_index_short: i128,
    pub bump: u8,
}
