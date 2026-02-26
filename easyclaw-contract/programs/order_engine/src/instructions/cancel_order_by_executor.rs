use anchor_lang::prelude::*;
use market_registry::program::MarketRegistry;

use crate::{
    error::ErrorCode,
    helpers::{assert_executor_authorized, estimate_order_reservation},
    state::{EngineConfig, Order, OrderStatus, UserMargin},
};

pub fn handler(ctx: Context<CancelOrderByExecutor>) -> Result<()> {
    assert_executor_authorized(
        &ctx.accounts.executor,
        &ctx.accounts.global_config,
        &ctx.accounts.keeper_set,
    )?;

    let order = &mut ctx.accounts.order;
    require!(order.status == OrderStatus::Open, ErrorCode::OrderNotOpen);

    let reserved_collateral =
        estimate_order_reservation(order.reduce_only, order.margin, &ctx.accounts.market)?;

    ctx.accounts.user_margin.collateral_balance = ctx
        .accounts
        .user_margin
        .collateral_balance
        .checked_add(reserved_collateral)
        .ok_or_else(|| error!(ErrorCode::MathOverflow))?;

    order.status = OrderStatus::Cancelled;

    Ok(())
}

#[derive(Accounts)]
pub struct CancelOrderByExecutor<'info> {
    pub executor: Signer<'info>,
    #[account(
        seeds = [b"engine-config"],
        bump = engine_config.bump,
    )]
    pub engine_config: Account<'info, EngineConfig>,
    pub market_registry_program: Program<'info, MarketRegistry>,
    #[account(address = engine_config.registry_global_config)]
    pub global_config: Account<'info, market_registry::GlobalConfig>,
    #[account(address = engine_config.keeper_set)]
    pub keeper_set: Account<'info, market_registry::KeeperSet>,
    #[account(
        seeds = [b"market".as_ref(), &order.market_id.to_le_bytes()],
        seeds::program = market_registry_program.key(),
        bump = market.bump,
    )]
    pub market: Account<'info, market_registry::Market>,
    #[account(
        mut,
        seeds = [b"user-margin", order.user.as_ref()],
        bump = user_margin.bump,
        constraint = user_margin.owner == order.user @ ErrorCode::MarginOrderMismatch,
    )]
    pub user_margin: Account<'info, UserMargin>,
    #[account(
        mut,
        constraint = order.user_margin == user_margin.key() @ ErrorCode::MarginOrderMismatch,
    )]
    pub order: Account<'info, Order>,
}
