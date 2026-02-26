use anchor_lang::prelude::*;
use market_registry::program::MarketRegistry;

use crate::{
    error::ErrorCode,
    helpers::estimate_order_reservation,
    state::{EngineConfig, Order, OrderStatus, OrderType, Side, UserMargin},
};

pub fn handler(
    ctx: Context<PlaceOrder>,
    market_id: u64,
    side: Side,
    order_type: OrderType,
    reduce_only: bool,
    order_margin: u64,
    price: u64,
    ttl_secs: i64,
    client_order_id: u64,
) -> Result<()> {
    require!(order_margin > 0, ErrorCode::InvalidAmount);
    require!(ttl_secs > 0, ErrorCode::InvalidTtl);
    require!(
        ttl_secs <= ctx.accounts.engine_config.max_ttl_secs,
        ErrorCode::TtlTooLong
    );
    require!(
        ctx.accounts.market.market_id == market_id,
        ErrorCode::MarketMismatch
    );
    require!(
        !ctx.accounts.global_config.global_pause,
        ErrorCode::GlobalPaused
    );
    require!(
        ctx.accounts.market.status == market_registry::MarketStatus::Active,
        ErrorCode::MarketNotActive
    );

    require!(price > 0, ErrorCode::InvalidLimitPrice);

    let now = Clock::get()?.unix_timestamp;
    let margin = &mut ctx.accounts.user_margin;
    require_keys_eq!(
        margin.owner,
        ctx.accounts.user.key(),
        ErrorCode::Unauthorized
    );

    let reserved_collateral =
        estimate_order_reservation(reduce_only, order_margin, &ctx.accounts.market)?;
    require!(
        margin.collateral_balance >= reserved_collateral,
        ErrorCode::InsufficientCollateral
    );
    margin.collateral_balance = margin
        .collateral_balance
        .checked_sub(reserved_collateral)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    let order = &mut ctx.accounts.order;
    order.id = margin.next_order_nonce;
    order.user_margin = margin.key();
    order.user = ctx.accounts.user.key();
    order.market_id = market_id;
    order.side = side;
    order.order_type = order_type;
    order.reduce_only = reduce_only;
    order.margin = order_margin;
    order.price = price;
    order.created_at = now;
    order.expires_at = now
        .checked_add(ttl_secs)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;
    order.client_order_id = client_order_id;
    order.status = OrderStatus::Open;
    order.bump = ctx.bumps.order;

    margin.next_order_nonce = margin
        .next_order_nonce
        .checked_add(1)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    Ok(())
}

#[derive(Accounts)]
#[instruction(market_id: u64)]
pub struct PlaceOrder<'info> {
    #[account(mut)]
    pub user: Signer<'info>,
    #[account(
        seeds = [b"engine-config"],
        bump = engine_config.bump,
    )]
    pub engine_config: Account<'info, EngineConfig>,
    pub market_registry_program: Program<'info, MarketRegistry>,
    #[account(address = engine_config.registry_global_config)]
    pub global_config: Account<'info, market_registry::GlobalConfig>,
    #[account(
        seeds = [b"market".as_ref(), &market_id.to_le_bytes()],
        seeds::program = market_registry_program.key(),
        bump = market.bump,
    )]
    pub market: Account<'info, market_registry::Market>,
    #[account(
        mut,
        seeds = [b"user-margin", user.key().as_ref()],
        bump = user_margin.bump,
    )]
    pub user_margin: Account<'info, UserMargin>,
    #[account(
        init,
        payer = user,
        seeds = [b"order", user_margin.key().as_ref(), &user_margin.next_order_nonce.to_le_bytes()],
        bump,
        space = 8 + Order::INIT_SPACE,
    )]
    pub order: Account<'info, Order>,
    pub system_program: Program<'info, System>,
}
