use anchor_lang::prelude::*;

#[error_code]
pub enum ErrorCode {
    #[msg("Unauthorized")]
    Unauthorized,
    #[msg("Invalid token account")]
    InvalidTokenAccount,
    #[msg("Invalid program account")]
    InvalidProgramAccount,
    #[msg("Invalid engine authority PDA")]
    InvalidEngineAuthority,
    #[msg("Unauthorized engine signer")]
    UnauthorizedEngine,
    #[msg("Invalid amount")]
    InvalidAmount,
    #[msg("Invalid bps")]
    InvalidBps,
    #[msg("Math overflow")]
    MathOverflow,
    #[msg("Invalid cooldown")]
    InvalidCooldown,
    #[msg("Insufficient shares")]
    InsufficientShares,
    #[msg("Withdraw cooldown has not finished")]
    CooldownNotFinished,
    #[msg("Withdraw request already claimed")]
    AlreadyClaimed,
    #[msg("Liquidity buffer constraint violated")]
    LiquidityBufferViolation,
    #[msg("Invalid LP position")]
    InvalidLpPosition,
    #[msg("Invalid withdraw request")]
    InvalidWithdrawRequest,
    #[msg("Invalid keeper rebate account")]
    InvalidKeeperRebate,
    #[msg("Nothing to claim")]
    NothingToClaim,
    #[msg("Insufficient protocol fee vault balance")]
    InsufficientProtocolFeeVault,
    #[msg("Insufficient insurance vault balance")]
    InsufficientInsuranceVault,
    #[msg("Insurance fund shortfall")]
    InsuranceShortfall,
    #[msg("Invalid pool state")]
    InvalidPoolState,
}
