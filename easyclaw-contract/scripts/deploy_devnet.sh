#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONTRACT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_DIR="$(cd "${CONTRACT_DIR}/.." && pwd)"

RPC_URL="${RPC_URL:-https://api.devnet.solana.com}"
WALLET_KEYPAIR="${WALLET_KEYPAIR:-${REPO_DIR}/.local/secret/deployer-wallet.json}"
SKIP_BUILD=0
PROGRAMS=()

ALL_PROGRAMS=("market_registry" "lp_vault" "order_engine" "usdc_faucet")

usage() {
  cat <<EOF
Usage:
  $(basename "$0") [program ...] [options]

Programs:
  market_registry | lp_vault | order_engine | usdc_faucet | all
  (default: all)

Options:
  --all                 Deploy all programs
  --program <name>      Add a program to deploy (repeatable)
  --rpc-url <url>       RPC URL (default: ${RPC_URL})
  --wallet <path>       Deployer keypair path (default: ${WALLET_KEYPAIR})
  --skip-build          Skip 'anchor keys sync' and 'anchor build'
  -h, --help            Show this help

Examples:
  $(basename "$0")
  $(basename "$0") --program order_engine --program lp_vault
  $(basename "$0") --wallet ${REPO_DIR}/.local/secret/deployer-wallet.json
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
  PROGRAMS=("${ALL_PROGRAMS[@]}")
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

WALLET_PUBKEY="$(solana-keygen pubkey "${WALLET_KEYPAIR}")"
WALLET_BALANCE="$(solana --url "${RPC_URL}" balance "${WALLET_PUBKEY}" || true)"

echo "RPC_URL=${RPC_URL}"
echo "WALLET_KEYPAIR=${WALLET_KEYPAIR}"
echo "WALLET_PUBKEY=${WALLET_PUBKEY}"
echo "WALLET_BALANCE=${WALLET_BALANCE}"
echo "PROGRAMS=${PROGRAMS[*]}"

if [[ "${SKIP_BUILD}" -eq 0 ]]; then
  run_cmd anchor_keys_sync bash -lc "cd '${CONTRACT_DIR}' && ANCHOR_WALLET='${WALLET_KEYPAIR}' anchor keys sync"
  run_cmd anchor_build bash -lc "cd '${CONTRACT_DIR}' && ANCHOR_WALLET='${WALLET_KEYPAIR}' anchor build"
else
  echo "[skip] anchor keys sync"
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
  echo "[info] ${program} program id: ${program_id}"

  run_cmd "deploy_${program}" \
    solana --url "${RPC_URL}" program deploy "${so_path}" \
      --program-id "${keypair_path}" \
      --keypair "${WALLET_KEYPAIR}" \
      --fee-payer "${WALLET_KEYPAIR}"

  run_cmd "show_${program}" solana --url "${RPC_URL}" program show "${program_id}"
done

MARKET_REGISTRY_PROGRAM_ID="$(solana-keygen pubkey "${CONTRACT_DIR}/target/deploy/market_registry-keypair.json")"
LP_VAULT_PROGRAM_ID="$(solana-keygen pubkey "${CONTRACT_DIR}/target/deploy/lp_vault-keypair.json")"
ORDER_ENGINE_PROGRAM_ID="$(solana-keygen pubkey "${CONTRACT_DIR}/target/deploy/order_engine-keypair.json")"
FAUCET_PROGRAM_ID="$(solana-keygen pubkey "${CONTRACT_DIR}/target/deploy/usdc_faucet-keypair.json")"

echo
echo "[done] devnet deploy finished"
echo "WALLET_PUBKEY=${WALLET_PUBKEY}"
echo "MARKET_REGISTRY_PROGRAM_ID=${MARKET_REGISTRY_PROGRAM_ID}"
echo "LP_VAULT_PROGRAM_ID=${LP_VAULT_PROGRAM_ID}"
echo "ORDER_ENGINE_PROGRAM_ID=${ORDER_ENGINE_PROGRAM_ID}"
echo "FAUCET_PROGRAM_ID=${FAUCET_PROGRAM_ID}"
