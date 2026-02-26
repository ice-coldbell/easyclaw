use anchor_lang::prelude::*;

use crate::{
    constants::MAX_KEEPERS,
    error::ErrorCode,
    helpers::require_admin,
    state::{GlobalConfig, KeeperSet},
};

pub fn handler(ctx: Context<AddKeeper>, keeper: Pubkey) -> Result<()> {
    require_admin(
        &ctx.accounts.authority,
        &ctx.accounts.global_config,
        &ctx.accounts.keeper_set,
    )?;

    let set = &mut ctx.accounts.keeper_set;
    require!(set.keepers.len() < MAX_KEEPERS, ErrorCode::KeeperSetFull);
    require!(
        !set.keepers.contains(&keeper),
        ErrorCode::KeeperAlreadyExists
    );

    set.keepers.push(keeper);
    ctx.accounts.global_config.last_updated_at = Clock::get()?.unix_timestamp;

    Ok(())
}

#[derive(Accounts)]
pub struct AddKeeper<'info> {
    #[account(mut)]
    pub authority: Signer<'info>,
    #[account(
        mut,
        seeds = [b"global-config"],
        bump = global_config.bump,
    )]
    pub global_config: Account<'info, GlobalConfig>,
    #[account(
        mut,
        seeds = [b"keeper-set"],
        bump = keeper_set.bump,
    )]
    pub keeper_set: Account<'info, KeeperSet>,
}
