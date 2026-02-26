use anchor_lang::prelude::*;
use anchor_spl::token::{self, Token, TokenAccount, Transfer};

pub fn transfer_from_collateral<'info>(
    token_program: &Program<'info, Token>,
    from: &Account<'info, TokenAccount>,
    to: &Account<'info, TokenAccount>,
    authority: &UncheckedAccount<'info>,
    authority_bump: u8,
    amount: u64,
) -> Result<()> {
    if amount == 0 {
        return Ok(());
    }

    let seeds: &[&[u8]] = &[b"engine-authority", &[authority_bump]];
    let signer = &[seeds];

    let cpi_accounts = Transfer {
        from: from.to_account_info(),
        to: to.to_account_info(),
        authority: authority.to_account_info(),
    };

    token::transfer(
        CpiContext::new_with_signer(token_program.to_account_info(), cpi_accounts, signer),
        amount,
    )
}
