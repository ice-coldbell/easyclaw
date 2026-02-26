#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${ENV_FILE:-${ROOT_DIR}/.env}"
if [[ -f "${ENV_FILE}" ]]; then
  set -a
  source "${ENV_FILE}"
  set +a
fi

CONTRACT_DIR="${ROOT_DIR}/easyclaw-contract"
LOCAL_ROOT="${ROOT_DIR}/.local"
LOCALNET_DIR="${LOCAL_ROOT}/localnet"
SECRET_DIR="${LOCAL_ROOT}/secret"

WORKSPACE_ROOT="${LOCALNET_DIR}/workspace"
WORK_CONTRACT_DIR="${WORKSPACE_ROOT}/easyclaw-contract"
ARTIFACT_DIR="${LOCALNET_DIR}/artifacts"
LOG_DIR="${LOCALNET_DIR}/logs"
TMP_DIR="${LOCALNET_DIR}/tmp"

SOLANA_CONFIG="${LOCALNET_DIR}/solana-config.yml"
USDC_MINT_KEYPAIR="${USDC_MINT_KEYPAIR:-${SECRET_DIR}/usdc-mint-keypair.json}"
LEGACY_USDC_MINT_KEYPAIR="${LOCALNET_DIR}/usdc-mint-keypair.json"
DEPLOYED_ENV="${LOCALNET_DIR}/deployed.env"
BACKEND_CONFIG_LOCAL="${ROOT_DIR}/easyclaw-backend/config/config-local.yaml"

RPC_URL="${RPC_URL:-http://127.0.0.1:8899}"
WALLET_KEYPAIR="${WALLET_KEYPAIR:-${SECRET_DIR}/deployer-wallet.json}"
BOOTSTRAP_MARKET_IDS="${BOOTSTRAP_MARKET_IDS:-1}"
LP_DEPOSIT_AMOUNT="${LP_DEPOSIT_AMOUNT:-10000000000}"
MINT_TOPUP_TOKENS="${MINT_TOPUP_TOKENS:-1000000}"

mkdir -p "${WORKSPACE_ROOT}" "${ARTIFACT_DIR}" "${LOG_DIR}" "${TMP_DIR}" "${SECRET_DIR}"
export TMPDIR="${TMP_DIR}"

if [[ ! -f "${WALLET_KEYPAIR}" ]]; then
  mkdir -p "$(dirname "${WALLET_KEYPAIR}")"
  solana-keygen new --no-bip39-passphrase --silent --force -o "${WALLET_KEYPAIR}" >/dev/null
fi

if ! solana --url "${RPC_URL}" cluster-version >/dev/null 2>&1; then
  echo "localnet is not running at ${RPC_URL}" >&2
  echo "run ./run-localnet-node.sh first" >&2
  exit 1
fi

for required_command in anchor solana spl-token npx rsync; do
  if ! command -v "${required_command}" >/dev/null 2>&1; then
    echo "required command not found: ${required_command}" >&2
    exit 1
  fi
done

run_cmd() {
  local name="$1"
  shift
  echo "[run] ${name}"
  "$@"
  echo "[ok] ${name}"
}

upsert_root_yaml_string() {
  local file="$1"
  local key="$2"
  local value="$3"
  local tmp_file

  tmp_file="$(mktemp)"
  awk -v key="${key}" -v value="${value}" '
BEGIN { done = 0 }
{
  if ($0 ~ ("^" key ":[[:space:]]*")) {
    print key ": \"" value "\""
    done = 1
    next
  }
  print
}
END {
  if (!done) {
    print key ": \"" value "\""
  }
}
' "${file}" > "${tmp_file}"

  mv "${tmp_file}" "${file}"
}

upsert_section_yaml_string() {
  local file="$1"
  local section="$2"
  local key="$3"
  local value="$4"
  local tmp_file

  tmp_file="$(mktemp)"
  awk -v section="${section}" -v key="${key}" -v value="${value}" '
BEGIN { in_section = 0; done = 0 }
{
  if ($0 ~ /^[^[:space:]].*:[[:space:]]*$/) {
    if (in_section && !done) {
      print "  " key ": \"" value "\""
      done = 1
    }
    if ($0 == section ":") {
      in_section = 1
    } else {
      in_section = 0
    }
    print
    next
  }

  if (in_section && $0 ~ ("^  " key ":[[:space:]]*")) {
    print "  " key ": \"" value "\""
    done = 1
    next
  }

  print
}
END {
  if (in_section && !done) {
    print "  " key ": \"" value "\""
    done = 1
  }
  if (!done) {
    print section ":"
    print "  " key ": \"" value "\""
  }
}
' "${file}" > "${tmp_file}"

  mv "${tmp_file}" "${file}"
}

sync_backend_local_config() {
  if [[ ! -f "${BACKEND_CONFIG_LOCAL}" ]]; then
    echo "[warn] backend config not found: ${BACKEND_CONFIG_LOCAL}"
    return 0
  fi

  upsert_section_yaml_string "${BACKEND_CONFIG_LOCAL}" "solana" "rpc_url" "${RPC_URL}"
  upsert_root_yaml_string "${BACKEND_CONFIG_LOCAL}" "market_registry_program_id" "${MARKET_REGISTRY_PROGRAM_ID}"
  upsert_root_yaml_string "${BACKEND_CONFIG_LOCAL}" "lp_vault_program_id" "${LP_VAULT_PROGRAM_ID}"
  upsert_root_yaml_string "${BACKEND_CONFIG_LOCAL}" "order_engine_program_id" "${ORDER_ENGINE_PROGRAM_ID}"
  upsert_section_yaml_string "${BACKEND_CONFIG_LOCAL}" "keeper" "keypair_path" "${WALLET_KEYPAIR}"

  echo "[ok] synced backend config: ${BACKEND_CONFIG_LOCAL}"
}

WALLET_PUBKEY="$(solana-keygen pubkey "${WALLET_KEYPAIR}")"
solana --url "${RPC_URL}" airdrop 100 "${WALLET_PUBKEY}" >/dev/null 2>&1 || true
solana config set --config "${SOLANA_CONFIG}" --url "${RPC_URL}" --keypair "${WALLET_KEYPAIR}" >/dev/null

rm -rf "${WORK_CONTRACT_DIR}"
mkdir -p "${WORK_CONTRACT_DIR}"
run_cmd prepare_workspace rsync -a --exclude node_modules --exclude target --exclude .git "${CONTRACT_DIR}/" "${WORK_CONTRACT_DIR}/"
ln -s "${CONTRACT_DIR}/node_modules" "${WORK_CONTRACT_DIR}/node_modules"
mkdir -p "${WORK_CONTRACT_DIR}/target/deploy"
cp "${CONTRACT_DIR}/target/deploy/"*-keypair.json "${WORK_CONTRACT_DIR}/target/deploy/"

run_cmd anchor_keys_sync bash -lc "cd '${WORK_CONTRACT_DIR}' && ANCHOR_WALLET='${WALLET_KEYPAIR}' anchor keys sync"
run_cmd anchor_build bash -lc "cd '${WORK_CONTRACT_DIR}' && ANCHOR_WALLET='${WALLET_KEYPAIR}' anchor build"

cp "${WORK_CONTRACT_DIR}/target/deploy/"*.so "${ARTIFACT_DIR}/"
cp "${WORK_CONTRACT_DIR}/target/deploy/"*-keypair.json "${ARTIFACT_DIR}/"
cp "${WORK_CONTRACT_DIR}/target/idl/"*.json "${ARTIFACT_DIR}/"

MARKET_REGISTRY_PROGRAM_ID="$(solana-keygen pubkey "${WORK_CONTRACT_DIR}/target/deploy/market_registry-keypair.json")"
LP_VAULT_PROGRAM_ID="$(solana-keygen pubkey "${WORK_CONTRACT_DIR}/target/deploy/lp_vault-keypair.json")"
ORDER_ENGINE_PROGRAM_ID="$(solana-keygen pubkey "${WORK_CONTRACT_DIR}/target/deploy/order_engine-keypair.json")"
FAUCET_PROGRAM_ID="$(solana-keygen pubkey "${WORK_CONTRACT_DIR}/target/deploy/usdc_faucet-keypair.json")"

run_cmd deploy_market_registry \
  solana --config "${SOLANA_CONFIG}" --url "${RPC_URL}" \
    program deploy "${WORK_CONTRACT_DIR}/target/deploy/market_registry.so" \
    --program-id "${WORK_CONTRACT_DIR}/target/deploy/market_registry-keypair.json" \
    --keypair "${WALLET_KEYPAIR}"

run_cmd deploy_lp_vault \
  solana --config "${SOLANA_CONFIG}" --url "${RPC_URL}" \
    program deploy "${WORK_CONTRACT_DIR}/target/deploy/lp_vault.so" \
    --program-id "${WORK_CONTRACT_DIR}/target/deploy/lp_vault-keypair.json" \
    --keypair "${WALLET_KEYPAIR}"

run_cmd deploy_order_engine \
  solana --config "${SOLANA_CONFIG}" --url "${RPC_URL}" \
    program deploy "${WORK_CONTRACT_DIR}/target/deploy/order_engine.so" \
    --program-id "${WORK_CONTRACT_DIR}/target/deploy/order_engine-keypair.json" \
    --keypair "${WALLET_KEYPAIR}"

if [[ ! -f "${USDC_MINT_KEYPAIR}" ]] && [[ -f "${LEGACY_USDC_MINT_KEYPAIR}" ]]; then
  mkdir -p "$(dirname "${USDC_MINT_KEYPAIR}")"
  mv "${LEGACY_USDC_MINT_KEYPAIR}" "${USDC_MINT_KEYPAIR}"
fi

if [[ ! -f "${USDC_MINT_KEYPAIR}" ]]; then
  mkdir -p "$(dirname "${USDC_MINT_KEYPAIR}")"
  solana-keygen new --no-bip39-passphrase --silent --force -o "${USDC_MINT_KEYPAIR}" >/dev/null
fi

USDC_MINT="$(solana-keygen pubkey "${USDC_MINT_KEYPAIR}")"
if ! solana --url "${RPC_URL}" account "${USDC_MINT}" >/dev/null 2>&1; then
  run_cmd create_usdc_mint \
    spl-token -C "${SOLANA_CONFIG}" --url "${RPC_URL}" \
      create-token "${USDC_MINT_KEYPAIR}" --decimals 6 --mint-authority "${WALLET_PUBKEY}"
else
  echo "[skip] usdc mint already exists: ${USDC_MINT}"
fi

WALLET_ATA="$(
  spl-token -C "${SOLANA_CONFIG}" --url "${RPC_URL}" address --verbose --owner "${WALLET_PUBKEY}" --token "${USDC_MINT}" \
    | awk '/Associated token address:/ {print $4}'
)"
if ! solana --url "${RPC_URL}" account "${WALLET_ATA}" >/dev/null 2>&1; then
  run_cmd create_wallet_usdc_account \
    spl-token -C "${SOLANA_CONFIG}" --url "${RPC_URL}" create-account "${USDC_MINT}"
else
  echo "[skip] wallet ATA already exists: ${WALLET_ATA}"
fi

run_cmd mint_usdc_to_wallet \
  spl-token -C "${SOLANA_CONFIG}" --url "${RPC_URL}" \
    mint "${USDC_MINT}" "${MINT_TOPUP_TOKENS}" --mint-authority "${WALLET_KEYPAIR}"

COMMON_ENV="ANCHOR_PROVIDER_URL='${RPC_URL}' \
ANCHOR_WALLET='${WALLET_KEYPAIR}' \
MARKET_REGISTRY_PROGRAM_ID='${MARKET_REGISTRY_PROGRAM_ID}' \
LP_VAULT_PROGRAM_ID='${LP_VAULT_PROGRAM_ID}' \
ORDER_ENGINE_PROGRAM_ID='${ORDER_ENGINE_PROGRAM_ID}' \
FAUCET_PROGRAM_ID='${FAUCET_PROGRAM_ID}' \
USDC_MINT='${USDC_MINT}'"

run_cmd bootstrap_btc_market \
  bash -lc "cd '${WORK_CONTRACT_DIR}' && ${COMMON_ENV} BOOTSTRAP_MARKET_IDS='${BOOTSTRAP_MARKET_IDS}' npx tsx scripts/bootstrap.ts"

run_cmd fund_lp \
  bash -lc "cd '${WORK_CONTRACT_DIR}' && ${COMMON_ENV} LP_DEPOSIT_AMOUNT='${LP_DEPOSIT_AMOUNT}' npx tsx scripts/fund_lp.ts"

run_cmd wallet_usdc_balance \
  spl-token -C "${SOLANA_CONFIG}" --url "${RPC_URL}" balance --owner "${WALLET_PUBKEY}" "${USDC_MINT}"

cat > "${DEPLOYED_ENV}" <<EOF
RPC_URL=${RPC_URL}
WALLET_KEYPAIR=${WALLET_KEYPAIR}
WALLET_PUBKEY=${WALLET_PUBKEY}
USDC_MINT=${USDC_MINT}
MARKET_REGISTRY_PROGRAM_ID=${MARKET_REGISTRY_PROGRAM_ID}
LP_VAULT_PROGRAM_ID=${LP_VAULT_PROGRAM_ID}
ORDER_ENGINE_PROGRAM_ID=${ORDER_ENGINE_PROGRAM_ID}
FAUCET_PROGRAM_ID=${FAUCET_PROGRAM_ID}
BOOTSTRAP_MARKET_IDS=${BOOTSTRAP_MARKET_IDS}
LP_DEPOSIT_AMOUNT=${LP_DEPOSIT_AMOUNT}
EOF

sync_backend_local_config

echo "[done] deployed and funded LP"
echo "[done] saved env: ${DEPLOYED_ENV}"
