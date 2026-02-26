use anchor_lang::prelude::*;

use crate::state::FeeSplit;

#[account]
#[derive(InitSpace)]
pub struct GlobalConfig {
    pub multisig: Pubkey,
    pub global_pause: bool,
    pub fee_split: FeeSplit,
    pub keeper_set: Pubkey,
    pub created_at: i64,
    pub last_updated_at: i64,
    pub bump: u8,
}
