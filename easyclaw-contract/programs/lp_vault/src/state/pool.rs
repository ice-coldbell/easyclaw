use anchor_lang::prelude::*;

#[account]
#[derive(InitSpace)]
pub struct Pool {
    pub admin: Pubkey,
    pub usdc_mint: Pubkey,
    pub order_engine_program: Pubkey,
    pub engine_authority: Pubkey,
    pub liquidity_vault: Pubkey,
    pub insurance_vault: Pubkey,
    pub protocol_fee_vault: Pubkey,
    pub cooldown_secs: i64,
    pub min_liquidity_buffer_bps: u16,
    pub lp_fee_bps: u16,
    pub insurance_fee_bps: u16,
    pub protocol_fee_bps: u16,
    pub execution_rebate_usdc: u64,
    pub total_shares: u128,
    pub pending_keeper_rebates: u64,
    pub total_trading_fees: u64,
    pub cumulative_trader_pnl: i128,
    pub bump: u8,
}
