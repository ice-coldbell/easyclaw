use anchor_lang::prelude::*;

#[account]
#[derive(InitSpace)]
pub struct KeeperRebate {
    pub pool: Pubkey,
    pub keeper: Pubkey,
    pub amount: u64,
    pub bump: u8,
}
