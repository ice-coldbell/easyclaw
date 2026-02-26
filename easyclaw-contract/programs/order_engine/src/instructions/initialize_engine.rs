use anchor_lang::prelude::*;
use anchor_spl::token::{Mint, Token, TokenAccount};
use lp_vault::program::LpVault;
use market_registry::program::MarketRegistry;

use crate::{error::ErrorCode, state::EngineConfig};

pub fn handler(
    ctx: Context<InitializeEngine>,
    max_ttl_secs: i64,
    liquidation_penalty_bps: u16,
    max_imr_bps: u16,
) -> Result<()> {
    require!(max_ttl_secs > 0, ErrorCode::InvalidTtl);
    require!(liquidation_penalty_bps <= 5_000, ErrorCode::InvalidBps);
    require!(max_imr_bps <= 10_000, ErrorCode::InvalidBps);

    require_keys_eq!(
        ctx.accounts.lp_pool.usdc_mint,
        ctx.accounts.usdc_mint.key(),
        ErrorCode::LpPoolConfigMismatch
    );
    require_keys_eq!(
        ctx.accounts.global_config.keeper_set,
        ctx.accounts.keeper_set.key(),
        ErrorCode::RegistryConfigMismatch
    );
    require_keys_eq!(
        ctx.accounts.lp_pool.order_engine_program,
        crate::ID,
        ErrorCode::LpPoolConfigMismatch
    );
    require_keys_eq!(
        ctx.accounts.lp_pool.engine_authority,
        ctx.accounts.engine_authority.key(),
        ErrorCode::LpPoolConfigMismatch
    );
    require_keys_eq!(
        ctx.accounts.lp_pool.liquidity_vault,
        ctx.accounts.lp_liquidity_vault.key(),
        ErrorCode::LpPoolConfigMismatch
    );
    require_keys_eq!(
        ctx.accounts.lp_pool.insurance_vault,
        ctx.accounts.lp_insurance_vault.key(),
        ErrorCode::LpPoolConfigMismatch
    );
    require_keys_eq!(
        ctx.accounts.lp_pool.protocol_fee_vault,
        ctx.accounts.lp_protocol_fee_vault.key(),
        ErrorCode::LpPoolConfigMismatch
    );

    let config = &mut ctx.accounts.engine_config;
    config.admin = ctx.accounts.admin.key();
    config.usdc_mint = ctx.accounts.usdc_mint.key();
    config.collateral_vault = ctx.accounts.collateral_vault.key();
    config.registry_program = ctx.accounts.market_registry_program.key();
    config.registry_global_config = ctx.accounts.global_config.key();
    config.keeper_set = ctx.accounts.keeper_set.key();
    config.lp_vault_program = ctx.accounts.lp_vault_program.key();
    config.lp_pool = ctx.accounts.lp_pool.key();
    config.lp_liquidity_vault = ctx.accounts.lp_liquidity_vault.key();
    config.lp_insurance_vault = ctx.accounts.lp_insurance_vault.key();
    config.lp_protocol_fee_vault = ctx.accounts.lp_protocol_fee_vault.key();
    config.engine_authority = ctx.accounts.engine_authority.key();
    config.max_ttl_secs = max_ttl_secs;
    config.liquidation_penalty_bps = liquidation_penalty_bps;
    config.max_imr_bps = max_imr_bps;
    config.bump = ctx.bumps.engine_config;

    Ok(())
}

#[derive(Accounts)]
pub struct InitializeEngine<'info> {
    #[account(mut)]
    pub admin: Signer<'info>,
    pub usdc_mint: Box<Account<'info, Mint>>,
    pub market_registry_program: Program<'info, MarketRegistry>,
    #[account(mut)]
    pub global_config: Box<Account<'info, market_registry::GlobalConfig>>,
    pub keeper_set: Box<Account<'info, market_registry::KeeperSet>>,
    pub lp_vault_program: Program<'info, LpVault>,
    pub lp_pool: Box<Account<'info, lp_vault::Pool>>,
    #[account(address = lp_pool.liquidity_vault)]
    pub lp_liquidity_vault: Box<Account<'info, TokenAccount>>,
    #[account(address = lp_pool.insurance_vault)]
    pub lp_insurance_vault: Box<Account<'info, TokenAccount>>,
    #[account(address = lp_pool.protocol_fee_vault)]
    pub lp_protocol_fee_vault: Box<Account<'info, TokenAccount>>,
    /// CHECK: engine authority PDA used for vault signing.
    #[account(seeds = [b"engine-authority"], bump)]
    pub engine_authority: UncheckedAccount<'info>,
    #[account(
        init,
        payer = admin,
        seeds = [b"engine-config"],
        bump,
        space = 8 + EngineConfig::INIT_SPACE,
    )]
    pub engine_config: Box<Account<'info, EngineConfig>>,
    #[account(
        init,
        payer = admin,
        seeds = [b"collateral-vault"],
        bump,
        token::mint = usdc_mint,
        token::authority = engine_authority,
    )]
    pub collateral_vault: Box<Account<'info, TokenAccount>>,
    pub token_program: Program<'info, Token>,
    pub system_program: Program<'info, System>,
}
