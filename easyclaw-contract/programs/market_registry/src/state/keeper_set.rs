use anchor_lang::prelude::*;

use crate::constants::MAX_KEEPERS;

#[account]
#[derive(InitSpace)]
pub struct KeeperSet {
    pub authority: Pubkey,
    #[max_len(MAX_KEEPERS)]
    pub keepers: Vec<Pubkey>,
    pub bump: u8,
}
