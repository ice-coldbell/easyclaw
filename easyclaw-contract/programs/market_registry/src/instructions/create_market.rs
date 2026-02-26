use anchor_lang::prelude::*;

use crate::{
    error::ErrorCode,
    helpers::{require_admin, to_fixed_symbol},
    state::{
        FeeParams, FundingParams, GlobalConfig, KeeperSet, Market, MarketStatus, PricingParams,
        RiskParams,
    },
};

pub fn handler(
    ctx: Context<CreateMarket>,
    market_id: u64,
    symbol: String,
    pyth_feed: Pubkey,
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
    require!(
        !ctx.accounts.global_config.global_pause,
        ErrorCode::GlobalPaused
    );

    risk_params.validate()?;
    pricing_params.validate()?;
    funding_params.validate()?;
    fee_params.validate()?;

    let market = &mut ctx.accounts.market;
    market.market_id = market_id;
    market.symbol = to_fixed_symbol(&symbol)?;
    market.pyth_feed = pyth_feed;
    market.status = MarketStatus::Active;
    market.risk_params = risk_params;
    market.pricing_params = pricing_params;
    market.funding_params = funding_params;
    market.fee_params = fee_params;
    market.bump = ctx.bumps.market;

    Ok(())
}

#[derive(Accounts)]
#[instruction(market_id: u64)]
pub struct CreateMarket<'info> {
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
        init,
        payer = authority,
        seeds = [b"market".as_ref(), &market_id.to_le_bytes()],
        bump,
        space = 8 + Market::INIT_SPACE,
    )]
    pub market: Account<'info, Market>,
    pub system_program: Program<'info, System>,
}
