use anchor_lang::prelude::*;

use crate::{
    helpers::require_admin,
    state::{GlobalConfig, KeeperSet, Market, MarketStatus},
};

pub fn handler(ctx: Context<SetMarketStatus>, status: MarketStatus) -> Result<()> {
    require_admin(
        &ctx.accounts.authority,
        &ctx.accounts.global_config,
        &ctx.accounts.keeper_set,
    )?;

    ctx.accounts.market.status = status;
    ctx.accounts.global_config.last_updated_at = Clock::get()?.unix_timestamp;

    Ok(())
}

#[derive(Accounts)]
pub struct SetMarketStatus<'info> {
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
