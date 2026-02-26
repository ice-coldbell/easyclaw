use anchor_lang::prelude::*;

use crate::{
    helpers::require_admin,
    state::{FeeParams, FundingParams, GlobalConfig, KeeperSet, Market, PricingParams, RiskParams},
};

pub fn handler(
    ctx: Context<UpdateMarketParams>,
    risk_params: RiskParams,
    pricing_params: PricingParams,
    funding_params: FundingParams,
    fee_params: FeeParams,
) -> Result<()> {
    require_admin(
        &ctx.accounts.authority,
        &ctx.accounts.global_config,
        &ctx.accounts.keeper_set,
    )?;

    risk_params.validate()?;
    pricing_params.validate()?;
    funding_params.validate()?;
    fee_params.validate()?;

    let market = &mut ctx.accounts.market;
    market.risk_params = risk_params;
    market.pricing_params = pricing_params;
    market.funding_params = funding_params;
    market.fee_params = fee_params;

    ctx.accounts.global_config.last_updated_at = Clock::get()?.unix_timestamp;

    Ok(())
}

#[derive(Accounts)]
pub struct UpdateMarketParams<'info> {
    #[account(mut)]
    pub authority: Signer<'info>,
    #[account(
        mut,
        seeds = [b"global-config"],
        bump = global_config.bump,
    )]
    pub global_config: Account<'info, GlobalConfig>,
    #[account(
        seeds = [b"keeper-set"],
        bump = keeper_set.bump,
    )]
    pub keeper_set: Account<'info, KeeperSet>,
    #[account(
        mut,
        seeds = [b"market".as_ref(), &market.market_id.to_le_bytes()],
        bump = market.bump,
    )]
    pub market: Account<'info, Market>,
}
