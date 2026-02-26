use anchor_lang::prelude::*;

use crate::{
    helpers::require_admin,
    state::{Pool, PoolConfigParams},
};

pub fn handler(ctx: Context<ConfigurePool>, params: PoolConfigParams) -> Result<()> {
    require_admin(&ctx.accounts.admin, &ctx.accounts.pool)?;
    params.validate()?;

    let pool = &mut ctx.accounts.pool;
    pool.cooldown_secs = params.cooldown_secs;
    pool.min_liquidity_buffer_bps = params.min_liquidity_buffer_bps;
    pool.lp_fee_bps = params.lp_fee_bps;
    pool.insurance_fee_bps = params.insurance_fee_bps;
    pool.protocol_fee_bps = params.protocol_fee_bps;
    pool.execution_rebate_usdc = params.execution_rebate_usdc;

    Ok(())
}

#[derive(Accounts)]
pub struct ConfigurePool<'info> {
    pub admin: Signer<'info>,
    #[account(
        mut,
        seeds = [b"pool", pool.usdc_mint.as_ref()],
        bump = pool.bump,
    )]
    pub pool: Account<'info, Pool>,
}
