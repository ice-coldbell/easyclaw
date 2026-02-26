use anchor_lang::prelude::*;

use crate::{
    constants::SYMBOL_LEN,
    error::ErrorCode,
    state::{GlobalConfig, KeeperSet},
};

pub fn require_admin(
    authority: &Signer<'_>,
    global_config: &Account<GlobalConfig>,
    keeper_set: &Account<KeeperSet>,
) -> Result<()> {
    require_keys_eq!(
        authority.key(),
        global_config.multisig,
        ErrorCode::Unauthorized
    );
    require_keys_eq!(
        keeper_set.key(),
        global_config.keeper_set,
        ErrorCode::InvalidKeeperSet
    );
    Ok(())
}

pub fn to_fixed_symbol(symbol: &str) -> Result<[u8; SYMBOL_LEN]> {
    let bytes = symbol.as_bytes();
    require!(
        !bytes.is_empty() && bytes.len() <= SYMBOL_LEN,
        ErrorCode::InvalidSymbolLength
    );

    let mut out = [0u8; SYMBOL_LEN];
    out[..bytes.len()].copy_from_slice(bytes);
    Ok(out)
}
