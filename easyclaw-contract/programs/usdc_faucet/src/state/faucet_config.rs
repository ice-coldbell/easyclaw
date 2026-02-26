use anchor_lang::prelude::*;

#[account]
#[derive(InitSpace)]
pub struct FaucetConfig {
    pub admin: Pubkey,
    pub mint: Pubkey,
    pub default_amount: u64,
    pub max_claim_amount: u64,
    pub bump: u8,
    pub authority_bump: u8,
}
