use anchor_lang::prelude::*;

#[account]
#[derive(InitSpace)]
pub struct LpPosition {
    pub owner: Pubkey,
    pub pool: Pubkey,
    pub shares: u128,
    pub pending_shares: u128,
    pub withdraw_nonce: u64,
    pub bump: u8,
}
