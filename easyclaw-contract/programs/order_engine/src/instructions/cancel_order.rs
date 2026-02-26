use anchor_lang::prelude::*;
use market_registry::program::MarketRegistry;

use crate::{
    error::ErrorCode,
    helpers::estimate_order_reservation,
    state::{Order, OrderStatus, UserMargin},
};

pub fn handler(ctx: Context<CancelOrder>) -> Result<()> {
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
pub struct CancelOrder<'info> {
    pub user: Signer<'info>,
    pub market_registry_program: Program<'info, MarketRegistry>,
    #[account(
        seeds = [b"market".as_ref(), &order.market_id.to_le_bytes()],
        seeds::program = market_registry_program.key(),
        bump = market.bump,
    )]
    pub market: Account<'info, market_registry::Market>,
    #[account(
        mut,
        seeds = [b"user-margin", user.key().as_ref()],
        bump = user_margin.bump,
        constraint = user_margin.owner == user.key() @ ErrorCode::Unauthorized,
    )]
    pub user_margin: Account<'info, UserMargin>,
    #[account(
        mut,
        constraint = order.user == user.key() @ ErrorCode::Unauthorized,
        constraint = order.user_margin == user_margin.key() @ ErrorCode::MarginOrderMismatch,
    )]
    pub order: Account<'info, Order>,
}
