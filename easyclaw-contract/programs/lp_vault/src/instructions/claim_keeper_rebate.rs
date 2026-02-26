use anchor_lang::prelude::*;
use anchor_spl::token::{self, Token, TokenAccount, Transfer};

use crate::{
    error::ErrorCode,
    state::{KeeperRebate, Pool},
};

pub fn handler(ctx: Context<ClaimKeeperRebate>) -> Result<()> {
    require_keys_eq!(
        ctx.accounts.keeper_rebate.pool,
        ctx.accounts.pool.key(),
        ErrorCode::InvalidKeeperRebate
    );
    require_keys_eq!(
        ctx.accounts.keeper_rebate.keeper,
        ctx.accounts.keeper.key(),
        ErrorCode::InvalidKeeperRebate
    );

    let amount = ctx.accounts.keeper_rebate.amount;
    require!(amount > 0, ErrorCode::NothingToClaim);
    require!(
        amount <= ctx.accounts.protocol_fee_vault.amount,
        ErrorCode::InsufficientProtocolFeeVault
    );

    let protocol_fee_auth_bump = ctx.bumps.protocol_fee_auth;
    let protocol_fee_auth_key = ctx.accounts.pool.key();
    let signer_seed_group: &[&[u8]] = &[
        b"protocol-fee-auth",
        protocol_fee_auth_key.as_ref(),
        &[protocol_fee_auth_bump],
    ];
    let signer_seeds = &[signer_seed_group];
    let cpi_accounts = Transfer {
        from: ctx.accounts.protocol_fee_vault.to_account_info(),
        to: ctx.accounts.keeper_token_account.to_account_info(),
        authority: ctx.accounts.protocol_fee_auth.to_account_info(),
    };
    token::transfer(
        CpiContext::new_with_signer(
            ctx.accounts.token_program.to_account_info(),
            cpi_accounts,
            signer_seeds,
        ),
        amount,
    )?;

    ctx.accounts.keeper_rebate.amount = 0;
    let pool = &mut ctx.accounts.pool;
    pool.pending_keeper_rebates = pool
        .pending_keeper_rebates
        .checked_sub(amount)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    Ok(())
}

#[derive(Accounts)]
pub struct ClaimKeeperRebate<'info> {
    #[account(mut)]
    pub keeper: Signer<'info>,
    #[account(
        mut,
        seeds = [b"pool", pool.usdc_mint.as_ref()],
        bump = pool.bump,
    )]
    pub pool: Account<'info, Pool>,
    #[account(
        mut,
        seeds = [b"keeper-rebate", pool.key().as_ref(), keeper.key().as_ref()],
        bump = keeper_rebate.bump,
    )]
    pub keeper_rebate: Account<'info, KeeperRebate>,
    /// CHECK: protocol fee authority PDA.
    #[account(seeds = [b"protocol-fee-auth", pool.key().as_ref()], bump)]
    pub protocol_fee_auth: UncheckedAccount<'info>,
    #[account(mut, address = pool.protocol_fee_vault)]
    pub protocol_fee_vault: Account<'info, TokenAccount>,
    #[account(
        mut,
        constraint = keeper_token_account.mint == pool.usdc_mint @ ErrorCode::InvalidTokenAccount,
        constraint = keeper_token_account.owner == keeper.key() @ ErrorCode::Unauthorized,
    )]
    pub keeper_token_account: Account<'info, TokenAccount>,
    pub token_program: Program<'info, Token>,
}
