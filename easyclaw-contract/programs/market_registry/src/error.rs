use anchor_lang::prelude::*;

#[error_code]
pub enum ErrorCode {
    #[msg("Unauthorized")]
    Unauthorized,
    #[msg("Invalid symbol length")]
    InvalidSymbolLength,
    #[msg("Global pause is enabled")]
    GlobalPaused,
    #[msg("Keeper set is full")]
    KeeperSetFull,
    #[msg("Keeper already exists")]
    KeeperAlreadyExists,
    #[msg("Keeper not found")]
    KeeperNotFound,
    #[msg("Invalid keeper set")]
    InvalidKeeperSet,
    #[msg("Invalid fee split")]
    InvalidFeeSplit,
    #[msg("Invalid risk params")]
    InvalidRiskParams,
    #[msg("Invalid pricing params")]
    InvalidPricingParams,
    #[msg("Invalid funding params")]
    InvalidFundingParams,
    #[msg("Invalid fee params")]
    InvalidFeeParams,
    #[msg("Math overflow")]
    MathOverflow,
}
