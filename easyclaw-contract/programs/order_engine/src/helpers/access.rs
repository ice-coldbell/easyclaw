use anchor_lang::prelude::*;

use crate::{error::ErrorCode, state::EngineConfig};

pub fn require_admin(admin: &Signer<'_>, config: &Account<EngineConfig>) -> Result<()> {
    require_keys_eq!(admin.key(), config.admin, ErrorCode::Unauthorized);
    Ok(())
}

pub fn assert_executor_authorized(
    executor: &Signer<'_>,
    global_config: &Account<market_registry::GlobalConfig>,
    keeper_set: &Account<market_registry::KeeperSet>,
) -> Result<()> {
    if executor.key() == global_config.multisig {
        return Ok(());
    }

    require!(
        keeper_set.keepers.contains(&executor.key()),
        ErrorCode::UnauthorizedExecutor
    );
    Ok(())
}

pub fn assert_keeper_only(
    executor: &Signer<'_>,
    keeper_set: &Account<market_registry::KeeperSet>,
) -> Result<()> {
    require!(
        keeper_set.keepers.contains(&executor.key()),
        ErrorCode::UnauthorizedExecutor
    );
    Ok(())
}
