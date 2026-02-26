use anchor_lang::prelude::*;

pub mod constants;
pub mod error;
pub mod helpers;
pub mod instructions;
pub mod state;

pub use constants::*;
pub use error::*;
pub use helpers::*;
pub use instructions::*;
pub use state::*;

declare_id!("7zpq5Xg74SEX1NchVzQXgG3JEFmHhnBJzFR9LXGuaBSj");

#[program]
pub mod order_engine {
    use super::*;

    pub fn initialize_engine(
        ctx: Context<InitializeEngine>,
        max_ttl_secs: i64,
        liquidation_penalty_bps: u16,
        max_imr_bps: u16,
    ) -> Result<()> {
        instructions::initialize_engine::handler(
            ctx,
            max_ttl_secs,
            liquidation_penalty_bps,
            max_imr_bps,
        )
    }

    pub fn initialize_market_funding_state(
        ctx: Context<InitializeMarketFundingState>,
        market_id: u64,
    ) -> Result<()> {
        instructions::initialize_market_funding_state::handler(ctx, market_id)
    }

    pub fn create_margin_account(ctx: Context<CreateMarginAccount>) -> Result<()> {
        instructions::create_margin_account::handler(ctx)
    }

    pub fn create_user_market_position(
        ctx: Context<CreateUserMarketPosition>,
        market_id: u64,
    ) -> Result<()> {
        instructions::create_user_market_position::handler(ctx, market_id)
    }

    pub fn deposit_collateral(ctx: Context<DepositCollateral>, amount: u64) -> Result<()> {
        instructions::deposit_collateral::handler(ctx, amount)
    }

    pub fn withdraw_collateral(ctx: Context<WithdrawCollateral>, amount: u64) -> Result<()> {
        instructions::withdraw_collateral::handler(ctx, amount)
    }

    pub fn place_order(
        ctx: Context<PlaceOrder>,
        market_id: u64,
        side: Side,
        order_type: OrderType,
        reduce_only: bool,
        margin: u64,
        price: u64,
        ttl_secs: i64,
        client_order_id: u64,
    ) -> Result<()> {
        instructions::place_order::handler(
            ctx,
            market_id,
            side,
            order_type,
            reduce_only,
            margin,
            price,
            ttl_secs,
            client_order_id,
        )
    }

    pub fn cancel_order(ctx: Context<CancelOrder>) -> Result<()> {
        instructions::cancel_order::handler(ctx)
    }

    pub fn cancel_order_by_executor(ctx: Context<CancelOrderByExecutor>) -> Result<()> {
        instructions::cancel_order_by_executor::handler(ctx)
    }

    pub fn execute_order(
        ctx: Context<ExecuteOrder>,
        fill_price: u64,
        oracle_price: u64,
        oracle_conf: u64,
        oracle_publish_time: i64,
    ) -> Result<()> {
        instructions::execute_order::handler(
            ctx,
            fill_price,
            oracle_price,
            oracle_conf,
            oracle_publish_time,
        )
    }

    pub fn liquidate(
        ctx: Context<Liquidate>,
        market_id: u64,
        leg: PositionLeg,
        close_qty: u64,
    ) -> Result<()> {
        instructions::liquidate::handler(ctx, market_id, leg, close_qty)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_mul_bps() {
        assert_eq!(mul_bps_u64(1_000_000, 500).unwrap(), 50_000);
        assert_eq!(mul_bps_u64(2_500_000, 10_000).unwrap(), 2_500_000);
    }

    #[test]
    fn test_abs_diff() {
        assert_eq!(abs_diff(100, 90), 10);
        assert_eq!(abs_diff(90, 100), 10);
    }
}
