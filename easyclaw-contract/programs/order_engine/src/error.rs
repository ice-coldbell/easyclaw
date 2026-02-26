use anchor_lang::prelude::*;

#[error_code]
pub enum ErrorCode {
    #[msg("Unauthorized")]
    Unauthorized,
    #[msg("Invalid collateral mint")]
    InvalidCollateralMint,
    #[msg("Unauthorized executor")]
    UnauthorizedExecutor,
    #[msg("Invalid keeper rebate account")]
    InvalidKeeperRebateAccount,
    #[msg("Registry config mismatch")]
    RegistryConfigMismatch,
    #[msg("LP pool config mismatch")]
    LpPoolConfigMismatch,
    #[msg("Invalid amount")]
    InvalidAmount,
    #[msg("Invalid bps")]
    InvalidBps,
    #[msg("Invalid ttl")]
    InvalidTtl,
    #[msg("TTL is too long")]
    TtlTooLong,
    #[msg("Math overflow")]
    MathOverflow,
    #[msg("Insufficient collateral")]
    InsufficientCollateral,
    #[msg("Margin requirement violation")]
    MarginRequirementViolation,
    #[msg("Invalid limit price")]
    InvalidLimitPrice,
    #[msg("Order is not open")]
    OrderNotOpen,
    #[msg("Market mismatch")]
    MarketMismatch,
    #[msg("Global pause is enabled")]
    GlobalPaused,
    #[msg("Market is not active")]
    MarketNotActive,
    #[msg("Order has expired")]
    OrderExpired,
    #[msg("Margin and order account mismatch")]
    MarginOrderMismatch,
    #[msg("Position owner mismatch")]
    PositionOwnerMismatch,
    #[msg("Stale oracle")]
    StaleOracle,
    #[msg("Oracle confidence too wide")]
    OracleConfidenceTooWide,
    #[msg("Fill deviation too large")]
    FillPriceDeviationTooLarge,
    #[msg("OI cap exceeded")]
    OiCapExceeded,
    #[msg("Skew cap exceeded")]
    SkewCapExceeded,
    #[msg("Max trade notional exceeded")]
    MaxTradeNotionalExceeded,
    #[msg("Impact price validation failed")]
    ImpactPriceViolation,
    #[msg("Limit price violation")]
    LimitPriceViolation,
    #[msg("Leverage exceeded")]
    LeverageExceeded,
    #[msg("Invalid price")]
    InvalidPrice,
    #[msg("Invalid oracle input")]
    InvalidOracle,
    #[msg("Not liquidatable")]
    NotLiquidatable,
    #[msg("Invalid close quantity")]
    InvalidCloseQty,
    #[msg("Insurance shortfall triggered market halt")]
    InsuranceShortfallMarketHalted,
    #[msg("Market is locally halted")]
    MarketHaltedLocal,
    #[msg("Invalid funding params")]
    InvalidFundingParams,
}
