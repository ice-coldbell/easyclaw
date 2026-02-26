use anchor_lang::prelude::*;

use crate::state::{LpPosition, Pool};

pub fn handler(ctx: Context<CreateLpPosition>) -> Result<()> {
    let lp = &mut ctx.accounts.lp_position;
    lp.owner = ctx.accounts.user.key();
    lp.pool = ctx.accounts.pool.key();
    lp.shares = 0;
    lp.pending_shares = 0;
    lp.withdraw_nonce = 0;
    lp.bump = ctx.bumps.lp_position;
    Ok(())
}

#[derive(Accounts)]
pub struct CreateLpPosition<'info> {
    #[account(mut)]
    pub user: Signer<'info>,
    #[account(
        seeds = [b"pool", pool.usdc_mint.as_ref()],
        bump = pool.bump,
    )]
    pub pool: Account<'info, Pool>,
    #[account(
        init,
        payer = user,
        seeds = [b"lp-pos", pool.key().as_ref(), user.key().as_ref()],
        bump,
        space = 8 + LpPosition::INIT_SPACE,
    )]
    pub lp_position: Account<'info, LpPosition>,
    pub system_program: Program<'info, System>,
}
