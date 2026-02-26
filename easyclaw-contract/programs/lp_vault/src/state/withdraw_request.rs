use anchor_lang::prelude::*;

#[account]
#[derive(InitSpace)]
pub struct WithdrawRequest {
    pub owner: Pubkey,
    pub pool: Pubkey,
    pub nonce: u64,
    pub share_amount: u128,
    pub requested_at: i64,
    pub claimed: bool,
    pub bump: u8,
}
