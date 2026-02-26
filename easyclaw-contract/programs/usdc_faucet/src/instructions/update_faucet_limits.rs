use anchor_lang::prelude::*;

use crate::{error::ErrorCode, state::FaucetConfig};

pub fn handler(
    ctx: Context<UpdateFaucetLimits>,
    default_amount: u64,
    max_claim_amount: u64,
) -> Result<()> {
    require!(default_amount > 0, ErrorCode::InvalidAmount);
    require!(max_claim_amount >= default_amount, ErrorCode::InvalidAmount);
    require_keys_eq!(
        ctx.accounts.admin.key(),
        ctx.accounts.faucet_config.admin,
        ErrorCode::Unauthorized
    );

    let faucet_config = &mut ctx.accounts.faucet_config;
    faucet_config.default_amount = default_amount;
    faucet_config.max_claim_amount = max_claim_amount;

    Ok(())
}

#[derive(Accounts)]
pub struct UpdateFaucetLimits<'info> {
    pub admin: Signer<'info>,
    #[account(
        mut,
        seeds = [b"faucet-config", faucet_config.mint.as_ref()],
        bump = faucet_config.bump,
    )]
    pub faucet_config: Account<'info, FaucetConfig>,
}
