use anchor_lang::prelude::*;

#[account]
#[derive(InitSpace)]
pub struct Order {
    pub id: u64,
    pub user_margin: Pubkey,
    pub user: Pubkey,
    pub market_id: u64,
    pub side: Side,
    pub order_type: OrderType,
    pub reduce_only: bool,
    pub margin: u64,
    pub price: u64,
    pub created_at: i64,
    pub expires_at: i64,
    pub client_order_id: u64,
    pub status: OrderStatus,
    pub bump: u8,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, InitSpace, PartialEq, Eq)]
pub enum Side {
    Buy,
    Sell,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, InitSpace, PartialEq, Eq)]
pub enum OrderType {
    Market,
    Limit,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, InitSpace, PartialEq, Eq)]
pub enum OrderStatus {
    Open,
    Executed,
    Cancelled,
    Expired,
}
