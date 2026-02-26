use anchor_lang::prelude::*;

pub mod error;
pub mod instructions;
pub mod state;

pub use error::*;
pub use instructions::*;
pub use state::*;

declare_id!("C94QPEetRNiB2pSQ8ZsYM8euZbKRmsTnwgDy1bTEjr1m");

#[program]
pub mod usdc_faucet {
    use super::*;

    pub fn initialize_faucet(
        ctx: Context<InitializeFaucet>,
        default_amount: u64,
        max_claim_amount: u64,
    ) -> Result<()> {
        instructions::initialize_faucet::handler(ctx, default_amount, max_claim_amount)
    }

    pub fn update_faucet_limits(
        ctx: Context<UpdateFaucetLimits>,
        default_amount: u64,
        max_claim_amount: u64,
    ) -> Result<()> {
        instructions::update_faucet_limits::handler(ctx, default_amount, max_claim_amount)
    }

    pub fn claim_from_faucet(ctx: Context<ClaimFromFaucet>, amount: u64) -> Result<()> {
        instructions::claim_from_faucet::handler(ctx, amount)
    }
}
