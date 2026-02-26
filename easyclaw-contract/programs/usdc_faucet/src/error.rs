use anchor_lang::prelude::*;

#[error_code]
pub enum ErrorCode {
    #[msg("invalid amount")]
    InvalidAmount,
    #[msg("unauthorized")]
    Unauthorized,
    #[msg("invalid mint authority")]
    InvalidMintAuthority,
    #[msg("claim amount exceeds maximum")]
    ClaimTooLarge,
    #[msg("invalid token account owner")]
    InvalidTokenAccountOwner,
    #[msg("invalid token account mint")]
    InvalidTokenMint,
}
