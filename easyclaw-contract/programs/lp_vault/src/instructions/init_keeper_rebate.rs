use anchor_lang::prelude::*;

use crate::state::{KeeperRebate, Pool};

pub fn handler(ctx: Context<InitKeeperRebate>) -> Result<()> {
    let rebate = &mut ctx.accounts.keeper_rebate;
    rebate.pool = ctx.accounts.pool.key();
    rebate.keeper = ctx.accounts.keeper.key();
    rebate.amount = 0;
    rebate.bump = ctx.bumps.keeper_rebate;
    Ok(())
}

#[derive(Accounts)]
pub struct InitKeeperRebate<'info> {
    #[account(mut)]
    pub keeper: Signer<'info>,
    #[account(
        seeds = [b"pool", pool.usdc_mint.as_ref()],
        bump = pool.bump,
    )]
    pub pool: Account<'info, Pool>,
    #[account(
        init,
        payer = keeper,
        seeds = [b"keeper-rebate", pool.key().as_ref(), keeper.key().as_ref()],
        bump,
        space = 8 + KeeperRebate::INIT_SPACE,
    )]
    pub keeper_rebate: Account<'info, KeeperRebate>,
    pub system_program: Program<'info, System>,
}
