#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONTRACT_DIR="${ROOT_DIR}/easyclaw-contract"
BACKEND_DIR="${ROOT_DIR}/easyclaw-backend"

DEFAULT_ENV_FILE="${ROOT_DIR}/.env"
DEFAULT_DEPLOYED_ENV_FILE="${ROOT_DIR}/.local/localnet/deployed.env"

if [[ -f "${DEFAULT_ENV_FILE}" ]]; then
  set -a
  source "${DEFAULT_ENV_FILE}"
  set +a
fi

if [[ -f "${DEFAULT_DEPLOYED_ENV_FILE}" ]]; then
  set -a
  source "${DEFAULT_DEPLOYED_ENV_FILE}"
  set +a
fi

RPC_URL="${RPC_URL:-${SOLANA_RPC_URL:-http://127.0.0.1:8899}}"
WALLET_KEYPAIR="${WALLET_KEYPAIR:-${KEEPER_KEYPAIR_PATH:-${ROOT_DIR}/.local/secret/deployer-wallet.json}}"

SKIP_BUILD=0
REGENERATE_CLIENTS=1
PROGRAMS=()

ALL_PROGRAMS=("market_registry" "lp_vault" "order_engine" "usdc_faucet")

usage() {
  cat <<EOF
Usage:
  $(basename "$0") [program ...] [options]

Programs:
  market_registry | lp_vault | order_engine | usdc_faucet | all
  (default: order_engine)

Options:
  --all                 Upgrade all programs
  --program <name>      Add a program to upgrade (repeatable)
  --rpc-url <url>       Override RPC URL (default: ${RPC_URL})
  --wallet <path>       Upgrade authority keypair path (default: ${WALLET_KEYPAIR})
  --skip-build          Skip anchor build and deploy existing .so files
  --no-client-gen       Skip go-anchor client regeneration
  -h, --help            Show this help

Examples:
  $(basename "$0")
  $(basename "$0") order_engine
  $(basename "$0") --all
  $(basename "$0") --program order_engine --program lp_vault
  $(basename "$0") order_engine --rpc-url http://127.0.0.1:8899 --wallet ${ROOT_DIR}/.local/secret/deployer-wallet.json
EOF
}

is_valid_program() {
  local name="$1"
  case "${name}" in
    market_registry|lp_vault|order_engine|usdc_faucet) return 0 ;;
    *) return 1 ;;
  esac
}

add_program() {
  local name="$1"
  if [[ "${name}" == "all" ]]; then
    PROGRAMS=("${ALL_PROGRAMS[@]}")
    return
  fi
  if ! is_valid_program "${name}"; then
    echo "invalid program name: ${name}" >&2
    usage
    exit 1
  fi
  PROGRAMS+=("${name}")
}

contains_program() {
  local needle="$1"
  shift
  local item
  for item in "$@"; do
    if [[ "${item}" == "${needle}" ]]; then
      return 0
    fi
  done
  return 1
}

run_cmd() {
  local name="$1"
  shift
  echo "[run] ${name}"
  "$@"
  echo "[ok] ${name}"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --all)
      PROGRAMS=("${ALL_PROGRAMS[@]}")
      shift
      ;;
    --program)
      if [[ $# -lt 2 ]]; then
        echo "--program requires a value" >&2
        exit 1
      fi
      add_program "$2"
      shift 2
      ;;
    --rpc-url)
      if [[ $# -lt 2 ]]; then
        echo "--rpc-url requires a value" >&2
        exit 1
      fi
      RPC_URL="$2"
      shift 2
      ;;
    --wallet|--keypair)
      if [[ $# -lt 2 ]]; then
        echo "--wallet requires a value" >&2
        exit 1
      fi
      WALLET_KEYPAIR="$2"
      shift 2
      ;;
    --skip-build)
      SKIP_BUILD=1
      shift
      ;;
    --no-client-gen)
      REGENERATE_CLIENTS=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      add_program "$1"
      shift
      ;;
  esac
done

if [[ ${#PROGRAMS[@]} -eq 0 ]]; then
  PROGRAMS=("order_engine")
fi

UNIQUE_PROGRAMS=()
for program in "${PROGRAMS[@]}"; do
  if ! contains_program "${program}" "${UNIQUE_PROGRAMS[@]:-}"; then
    UNIQUE_PROGRAMS+=("${program}")
  fi
done
PROGRAMS=("${UNIQUE_PROGRAMS[@]}")

for required_command in anchor solana solana-keygen; do
  if ! command -v "${required_command}" >/dev/null 2>&1; then
    echo "required command not found: ${required_command}" >&2
    exit 1
  fi
done

if [[ ! -f "${WALLET_KEYPAIR}" ]]; then
  echo "wallet keypair not found: ${WALLET_KEYPAIR}" >&2
  exit 1
fi

if ! solana --url "${RPC_URL}" cluster-version >/dev/null 2>&1; then
  echo "cannot reach Solana RPC: ${RPC_URL}" >&2
  exit 1
fi

echo "RPC_URL=${RPC_URL}"
echo "WALLET_KEYPAIR=${WALLET_KEYPAIR}"
echo "PROGRAMS=${PROGRAMS[*]}"

if [[ "${SKIP_BUILD}" -eq 0 ]]; then
  run_cmd anchor_keys_sync bash -lc "cd '${CONTRACT_DIR}' && ANCHOR_WALLET='${WALLET_KEYPAIR}' anchor keys sync"
  run_cmd anchor_build bash -lc "cd '${CONTRACT_DIR}' && ANCHOR_WALLET='${WALLET_KEYPAIR}' anchor build"
else
  echo "[skip] anchor build"
fi

for program in "${PROGRAMS[@]}"; do
  so_path="${CONTRACT_DIR}/target/deploy/${program}.so"
  keypair_path="${CONTRACT_DIR}/target/deploy/${program}-keypair.json"
  if [[ ! -f "${so_path}" ]]; then
    echo "missing binary: ${so_path}" >&2
    exit 1
  fi
  if [[ ! -f "${keypair_path}" ]]; then
    echo "missing program keypair: ${keypair_path}" >&2
    exit 1
  fi

  program_id="$(solana-keygen pubkey "${keypair_path}")"

  run_cmd "deploy_${program}" \
    solana --url "${RPC_URL}" program deploy "${so_path}" \
      --program-id "${keypair_path}" \
      --keypair "${WALLET_KEYPAIR}"

  run_cmd "show_${program}" solana --url "${RPC_URL}" program show "${program_id}"
done

if [[ "${REGENERATE_CLIENTS}" -eq 1 ]] && { contains_program "order_engine" "${PROGRAMS[@]}" || contains_program "lp_vault" "${PROGRAMS[@]}"; }; then
  if ! command -v go >/dev/null 2>&1; then
    echo "go is required for client generation but not found. rerun with --no-client-gen or install go." >&2
    exit 1
  fi

  ORDER_ENGINE_PROGRAM_ID="$(solana-keygen pubkey "${CONTRACT_DIR}/target/deploy/order_engine-keypair.json")"
  LP_VAULT_PROGRAM_ID="$(solana-keygen pubkey "${CONTRACT_DIR}/target/deploy/lp_vault-keypair.json")"

  run_cmd generate_anchor_clients \
    bash -lc "cd '${BACKEND_DIR}' && ORDER_ENGINE_PROGRAM_ID='${ORDER_ENGINE_PROGRAM_ID}' LP_VAULT_PROGRAM_ID='${LP_VAULT_PROGRAM_ID}' ./scripts/generate-anchor-clients.sh ../easyclaw-contract/target/idl"
else
  echo "[skip] go-anchor client generation"
fi

echo "[done] program upgrade finished"
