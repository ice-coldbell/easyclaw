use anchor_lang::prelude::*;

#[account]
#[derive(InitSpace)]
pub struct UserMargin {
    pub owner: Pubkey,
    pub collateral_balance: u64,
    pub next_order_nonce: u64,
    pub total_notional: u64,
    pub bump: u8,
}
