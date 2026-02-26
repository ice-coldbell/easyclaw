use anchor_lang::prelude::*;

use crate::state::{EngineConfig, UserMargin};

pub fn handler(ctx: Context<CreateMarginAccount>) -> Result<()> {
    let margin = &mut ctx.accounts.user_margin;
    margin.owner = ctx.accounts.user.key();
    margin.collateral_balance = 0;
    margin.next_order_nonce = 0;
    margin.total_notional = 0;
    margin.bump = ctx.bumps.user_margin;

    Ok(())
}

#[derive(Accounts)]
pub struct CreateMarginAccount<'info> {
    #[account(mut)]
    pub user: Signer<'info>,
    #[account(
        seeds = [b"engine-config"],
        bump = engine_config.bump,
    )]
    pub engine_config: Account<'info, EngineConfig>,
    #[account(
        init,
        payer = user,
        seeds = [b"user-margin", user.key().as_ref()],
        bump,
        space = 8 + UserMargin::INIT_SPACE,
    )]
    pub user_margin: Account<'info, UserMargin>,
    pub system_program: Program<'info, System>,
}
