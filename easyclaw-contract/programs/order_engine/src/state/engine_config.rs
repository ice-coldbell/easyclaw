use anchor_lang::prelude::*;

#[account]
#[derive(InitSpace)]
pub struct EngineConfig {
    pub admin: Pubkey,
    pub usdc_mint: Pubkey,
    pub collateral_vault: Pubkey,
    pub registry_program: Pubkey,
    pub registry_global_config: Pubkey,
    pub keeper_set: Pubkey,
    pub lp_vault_program: Pubkey,
    pub lp_pool: Pubkey,
    pub lp_liquidity_vault: Pubkey,
    pub lp_insurance_vault: Pubkey,
    pub lp_protocol_fee_vault: Pubkey,
    pub engine_authority: Pubkey,
    pub max_ttl_secs: i64,
    pub liquidation_penalty_bps: u16,
    pub max_imr_bps: u16,
    pub bump: u8,
}
