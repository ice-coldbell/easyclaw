use anchor_lang::prelude::*;
use anchor_lang::solana_program::program_option::COption;
use anchor_spl::token::{mint_to, Mint, MintTo, Token, TokenAccount};

use crate::{error::ErrorCode, state::FaucetConfig};

pub fn handler(ctx: Context<ClaimFromFaucet>, amount: u64) -> Result<()> {
    let faucet_config = &ctx.accounts.faucet_config;
    let claim_amount = if amount == 0 {
        faucet_config.default_amount
    } else {
        amount
    };
    require!(claim_amount > 0, ErrorCode::InvalidAmount);
    require!(
        claim_amount <= faucet_config.max_claim_amount,
        ErrorCode::ClaimTooLarge
    );
    require!(
        ctx.accounts.mint.mint_authority == COption::Some(ctx.accounts.faucet_authority.key()),
        ErrorCode::InvalidMintAuthority
    );

    let mint_key = ctx.accounts.mint.key();
    let signer_seeds: &[&[u8]] = &[
        b"faucet-authority",
        mint_key.as_ref(),
        &[faucet_config.authority_bump],
    ];

    mint_to(
        CpiContext::new_with_signer(
            ctx.accounts.token_program.to_account_info(),
            MintTo {
                mint: ctx.accounts.mint.to_account_info(),
                to: ctx.accounts.user_token_account.to_account_info(),
                authority: ctx.accounts.faucet_authority.to_account_info(),
            },
            &[signer_seeds],
        ),
        claim_amount,
    )?;

    Ok(())
}

#[derive(Accounts)]
pub struct ClaimFromFaucet<'info> {
    pub user: Signer<'info>,
    #[account(
        seeds = [b"faucet-config", mint.key().as_ref()],
        bump = faucet_config.bump,
    )]
    pub faucet_config: Account<'info, FaucetConfig>,
    #[account(mut, address = faucet_config.mint)]
    pub mint: Account<'info, Mint>,
    /// CHECK: PDA signer for mint authority.
    #[account(
        seeds = [b"faucet-authority", mint.key().as_ref()],
        bump = faucet_config.authority_bump,
    )]
    pub faucet_authority: UncheckedAccount<'info>,
    #[account(
        mut,
        constraint = user_token_account.owner == user.key() @ ErrorCode::InvalidTokenAccountOwner,
        constraint = user_token_account.mint == mint.key() @ ErrorCode::InvalidTokenMint,
    )]
    pub user_token_account: Account<'info, TokenAccount>,
    pub token_program: Program<'info, Token>,
}
