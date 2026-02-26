use anchor_lang::prelude::*;

pub mod constants;
pub mod error;
pub mod helpers;
pub mod instructions;
pub mod state;

pub use constants::*;
pub use error::*;
pub use instructions::*;
pub use state::*;

declare_id!("7A7BCZChVprUa3WBbC7mER54E7BVGA7PussRB8fHC5tz");

#[program]
pub mod lp_vault {
    use super::*;

    pub fn initialize_pool(
        ctx: Context<InitializePool>,
        cooldown_secs: i64,
        min_liquidity_buffer_bps: u16,
    ) -> Result<()> {
        instructions::initialize_pool::handler(ctx, cooldown_secs, min_liquidity_buffer_bps)
    }

    pub fn configure_pool(ctx: Context<ConfigurePool>, params: PoolConfigParams) -> Result<()> {
        instructions::configure_pool::handler(ctx, params)
    }

    pub fn create_lp_position(ctx: Context<CreateLpPosition>) -> Result<()> {
        instructions::create_lp_position::handler(ctx)
    }

    pub fn init_keeper_rebate(ctx: Context<InitKeeperRebate>) -> Result<()> {
        instructions::init_keeper_rebate::handler(ctx)
    }

    pub fn deposit_lp(ctx: Context<DepositLp>, amount: u64) -> Result<()> {
        instructions::deposit_lp::handler(ctx, amount)
    }

    pub fn request_withdraw_lp(ctx: Context<RequestWithdrawLp>, share_amount: u128) -> Result<()> {
        instructions::request_withdraw_lp::handler(ctx, share_amount)
    }

    pub fn claim_withdraw_lp(ctx: Context<ClaimWithdrawLp>) -> Result<()> {
        instructions::claim_withdraw_lp::handler(ctx)
    }

    pub fn apply_trade_fill(
        ctx: Context<ApplyTradeFill>,
        market_id: u64,
        user: Pubkey,
        order_id: u64,
        notional: u64,
        fee: u64,
        pnl_delta: i64,
    ) -> Result<()> {
        instructions::apply_trade_fill::handler(
            ctx, market_id, user, order_id, notional, fee, pnl_delta,
        )
    }

    pub fn apply_liquidation(
        ctx: Context<ApplyLiquidation>,
        market_id: u64,
        user: Pubkey,
        penalty: u64,
        bad_debt: u64,
    ) -> Result<()> {
        instructions::apply_liquidation::handler(ctx, market_id, user, penalty, bad_debt)
    }

    pub fn claim_keeper_rebate(ctx: Context<ClaimKeeperRebate>) -> Result<()> {
        instructions::claim_keeper_rebate::handler(ctx)
    }
}
