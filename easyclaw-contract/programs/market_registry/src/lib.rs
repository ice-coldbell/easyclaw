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

declare_id!("Gswa2NzGEuUeM5MekCTJ9rLyzsocW1FENHue5f3uiDr7");

#[program]
pub mod market_registry {
    use super::*;

    pub fn initialize_global(
        ctx: Context<InitializeGlobal>,
        multisig: Pubkey,
        fee_split: FeeSplit,
        pause_flags: bool,
    ) -> Result<()> {
        instructions::initialize_global::handler(ctx, multisig, fee_split, pause_flags)
    }

    pub fn create_market(
        ctx: Context<CreateMarket>,
        market_id: u64,
        symbol: String,
        pyth_feed: Pubkey,
        risk_params: RiskParams,
        pricing_params: PricingParams,
        funding_params: FundingParams,
        fee_params: FeeParams,
    ) -> Result<()> {
        instructions::create_market::handler(
            ctx,
            market_id,
            symbol,
            pyth_feed,
            risk_params,
            pricing_params,
            funding_params,
            fee_params,
        )
    }

    pub fn update_market_params(
        ctx: Context<UpdateMarketParams>,
        risk_params: RiskParams,
        pricing_params: PricingParams,
        funding_params: FundingParams,
        fee_params: FeeParams,
    ) -> Result<()> {
        instructions::update_market_params::handler(
            ctx,
            risk_params,
            pricing_params,
            funding_params,
            fee_params,
        )
    }

    pub fn set_market_status(ctx: Context<SetMarketStatus>, status: MarketStatus) -> Result<()> {
        instructions::set_market_status::handler(ctx, status)
    }

    pub fn set_global_pause(ctx: Context<SetGlobalPause>, paused: bool) -> Result<()> {
        instructions::set_global_pause::handler(ctx, paused)
    }

    pub fn add_keeper(ctx: Context<AddKeeper>, keeper: Pubkey) -> Result<()> {
        instructions::add_keeper::handler(ctx, keeper)
    }

    pub fn remove_keeper(ctx: Context<RemoveKeeper>, keeper: Pubkey) -> Result<()> {
        instructions::remove_keeper::handler(ctx, keeper)
    }
}
