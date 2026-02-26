use anchor_lang::prelude::*;

use crate::{
    error::ErrorCode,
    state::{EngineConfig, UserMargin, UserMarketPosition},
};

pub fn handler(ctx: Context<CreateUserMarketPosition>, market_id: u64) -> Result<()> {
    require_keys_eq!(
        ctx.accounts.user_margin.owner,
        ctx.accounts.user.key(),
        ErrorCode::Unauthorized
    );

    let pos = &mut ctx.accounts.user_market_position;
    pos.user_margin = ctx.accounts.user_margin.key();
    pos.market_id = market_id;
    pos.long_qty = 0;
    pos.long_entry_notional = 0;
    pos.short_qty = 0;
    pos.short_entry_notional = 0;
    pos.last_funding_index_long = 0;
    pos.last_funding_index_short = 0;
    pos.bump = ctx.bumps.user_market_position;

    Ok(())
}

#[derive(Accounts)]
#[instruction(market_id: u64)]
pub struct CreateUserMarketPosition<'info> {
    #[account(mut)]
    pub user: Signer<'info>,
    #[account(
        seeds = [b"engine-config"],
        bump = engine_config.bump,
    )]
    pub engine_config: Account<'info, EngineConfig>,
    #[account(
        mut,
        seeds = [b"user-margin", user.key().as_ref()],
        bump = user_margin.bump,
    )]
    pub user_margin: Account<'info, UserMargin>,
    #[account(
        init,
        payer = user,
        seeds = [b"user-market-pos", user_margin.key().as_ref(), &market_id.to_le_bytes()],
        bump,
        space = 8 + UserMarketPosition::INIT_SPACE,
    )]
    pub user_market_position: Account<'info, UserMarketPosition>,
    pub system_program: Program<'info, System>,
}
