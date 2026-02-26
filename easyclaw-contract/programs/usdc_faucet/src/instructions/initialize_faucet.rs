use anchor_lang::prelude::*;
use anchor_lang::solana_program::program_option::COption;
use anchor_spl::token::{
    set_authority, spl_token::instruction::AuthorityType, Mint, SetAuthority, Token,
};

use crate::{error::ErrorCode, state::FaucetConfig};

pub fn handler(
    ctx: Context<InitializeFaucet>,
    default_amount: u64,
    max_claim_amount: u64,
) -> Result<()> {
    require!(default_amount > 0, ErrorCode::InvalidAmount);
    require!(max_claim_amount >= default_amount, ErrorCode::InvalidAmount);

    let faucet_authority = ctx.accounts.faucet_authority.key();
    if ctx.accounts.mint.mint_authority != COption::Some(faucet_authority) {
        require!(
            ctx.accounts.mint.mint_authority == COption::Some(ctx.accounts.admin.key()),
            ErrorCode::InvalidMintAuthority
        );

        set_authority(
            CpiContext::new(
                ctx.accounts.token_program.to_account_info(),
                SetAuthority {
                    account_or_mint: ctx.accounts.mint.to_account_info(),
                    current_authority: ctx.accounts.admin.to_account_info(),
                },
            ),
            AuthorityType::MintTokens,
            Some(faucet_authority),
        )?;
    }

    let faucet_config = &mut ctx.accounts.faucet_config;
    faucet_config.admin = ctx.accounts.admin.key();
    faucet_config.mint = ctx.accounts.mint.key();
    faucet_config.default_amount = default_amount;
    faucet_config.max_claim_amount = max_claim_amount;
    faucet_config.bump = ctx.bumps.faucet_config;
    faucet_config.authority_bump = ctx.bumps.faucet_authority;

    Ok(())
}

#[derive(Accounts)]
pub struct InitializeFaucet<'info> {
    #[account(mut)]
    pub admin: Signer<'info>,
    #[account(mut)]
    pub mint: Account<'info, Mint>,
    /// CHECK: PDA signer for mint authority.
    #[account(
        seeds = [b"faucet-authority", mint.key().as_ref()],
        bump,
    )]
    pub faucet_authority: UncheckedAccount<'info>,
    #[account(
        init,
        payer = admin,
        seeds = [b"faucet-config", mint.key().as_ref()],
        bump,
        space = 8 + FaucetConfig::INIT_SPACE,
    )]
    pub faucet_config: Account<'info, FaucetConfig>,
    pub token_program: Program<'info, Token>,
    pub system_program: Program<'info, System>,
}
