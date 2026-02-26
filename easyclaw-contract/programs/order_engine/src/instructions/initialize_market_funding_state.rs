use anchor_lang::prelude::*;
use market_registry::program::MarketRegistry;

use crate::{
    helpers::require_admin,
    state::{EngineConfig, MarketFundingState},
};

pub fn handler(ctx: Context<InitializeMarketFundingState>, market_id: u64) -> Result<()> {
    require_admin(&ctx.accounts.admin, &ctx.accounts.engine_config)?;
    require!(
        ctx.accounts.market.market_id == market_id,
        crate::error::ErrorCode::MarketMismatch
    );

    let state = &mut ctx.accounts.market_funding_state;
    state.market_id = market_id;
    state.funding_index = 0;
    state.last_update_ts = Clock::get()?.unix_timestamp;
    state.open_interest = 0;
    state.skew = 0;
    state.halted = false;
    state.bump = ctx.bumps.market_funding_state;

    Ok(())
}

#[derive(Accounts)]
#[instruction(market_id: u64)]
pub struct InitializeMarketFundingState<'info> {
    #[account(mut)]
    pub admin: Signer<'info>,
    #[account(
        seeds = [b"engine-config"],
        bump = engine_config.bump,
    )]
    pub engine_config: Account<'info, EngineConfig>,
    pub market_registry_program: Program<'info, MarketRegistry>,
    #[account(
        seeds = [b"market".as_ref(), &market_id.to_le_bytes()],
        seeds::program = market_registry_program.key(),
        bump = market.bump,
    )]
    pub market: Account<'info, market_registry::Market>,
    #[account(
        init,
        payer = admin,
        seeds = [b"funding".as_ref(), &market_id.to_le_bytes()],
        bump,
        space = 8 + MarketFundingState::INIT_SPACE,
    )]
    pub market_funding_state: Account<'info, MarketFundingState>,
    pub system_program: Program<'info, System>,
}
