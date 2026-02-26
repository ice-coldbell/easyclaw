use anchor_lang::prelude::*;

use crate::{
    helpers::require_admin,
    state::{GlobalConfig, KeeperSet},
};

pub fn handler(ctx: Context<SetGlobalPause>, paused: bool) -> Result<()> {
    require_admin(
        &ctx.accounts.authority,
        &ctx.accounts.global_config,
        &ctx.accounts.keeper_set,
    )?;

    ctx.accounts.global_config.global_pause = paused;
    ctx.accounts.global_config.last_updated_at = Clock::get()?.unix_timestamp;

    Ok(())
}

#[derive(Accounts)]
pub struct SetGlobalPause<'info> {
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
}
