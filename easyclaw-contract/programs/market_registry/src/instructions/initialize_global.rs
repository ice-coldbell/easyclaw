use anchor_lang::prelude::*;

use crate::{
    constants::MAX_KEEPERS,
    state::{FeeSplit, GlobalConfig, KeeperSet},
};

pub fn handler(
    ctx: Context<InitializeGlobal>,
    multisig: Pubkey,
    fee_split: FeeSplit,
    pause_flags: bool,
) -> Result<()> {
    fee_split.validate()?;
    let now = Clock::get()?.unix_timestamp;

    let global = &mut ctx.accounts.global_config;
    global.multisig = multisig;
    global.global_pause = pause_flags;
    global.fee_split = fee_split;
    global.keeper_set = ctx.accounts.keeper_set.key();
    global.created_at = now;
    global.last_updated_at = now;
    global.bump = ctx.bumps.global_config;

    let keeper_set = &mut ctx.accounts.keeper_set;
    keeper_set.authority = multisig;
    keeper_set.keepers = Vec::with_capacity(MAX_KEEPERS);
    keeper_set.bump = ctx.bumps.keeper_set;

    Ok(())
}

#[derive(Accounts)]
pub struct InitializeGlobal<'info> {
    #[account(mut)]
    pub payer: Signer<'info>,
    #[account(
        init,
        payer = payer,
        seeds = [b"global-config"],
        bump,
        space = 8 + GlobalConfig::INIT_SPACE,
    )]
    pub global_config: Account<'info, GlobalConfig>,
    #[account(
        init,
        payer = payer,
        seeds = [b"keeper-set"],
        bump,
        space = 8 + KeeperSet::INIT_SPACE,
    )]
    pub keeper_set: Account<'info, KeeperSet>,
    pub system_program: Program<'info, System>,
}
