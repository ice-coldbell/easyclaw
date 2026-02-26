#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${ENV_FILE:-${ROOT_DIR}/.env}"
if [[ -f "${ENV_FILE}" ]]; then
  set -a
  source "${ENV_FILE}"
  set +a
fi

LOCAL_ROOT="${ROOT_DIR}/.local"
LOCALNET_DIR="${LOCAL_ROOT}/localnet"
SECRET_DIR="${LOCAL_ROOT}/secret"
LEDGER_DIR="${LOCALNET_DIR}/ledger"
LOG_FILE="${LOCALNET_DIR}/validator.log"
PID_FILE="${LOCALNET_DIR}/validator.pid"

RPC_PORT="${RPC_PORT:-8899}"
FAUCET_PORT="${FAUCET_PORT:-9900}"
RPC_URL="http://127.0.0.1:${RPC_PORT}"

MINT_KEYPAIR="${MINT_KEYPAIR:-${SECRET_DIR}/localnet-mint-authority.json}"
RESET_LEDGER="${RESET_LEDGER:-0}"

mkdir -p "${LOCALNET_DIR}" "${SECRET_DIR}"

if [[ ! -f "${MINT_KEYPAIR}" ]]; then
  mkdir -p "$(dirname "${MINT_KEYPAIR}")"
  solana-keygen new --no-bip39-passphrase --silent --force -o "${MINT_KEYPAIR}" >/dev/null
fi

print_ledger_migration_hint() {
  if grep -q "snapshot dir account paths mismatching" "${LOG_FILE}" 2>/dev/null; then
    echo "detected moved ledger path. run once with: RESET_LEDGER=1 ./run-localnet-node.sh" >&2
  fi
}

if [[ -f "${PID_FILE}" ]]; then
  EXISTING_PID="$(cat "${PID_FILE}")"
  if kill -0 "${EXISTING_PID}" 2>/dev/null; then
    echo "localnet already running (pid=${EXISTING_PID})"
    echo "rpc=${RPC_URL}"
    echo "log=${LOG_FILE}"
    exit 0
  fi
  rm -f "${PID_FILE}"
fi

if command -v lsof >/dev/null 2>&1; then
  if lsof -nP -iTCP:"${RPC_PORT}" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "port ${RPC_PORT} already in use. stop existing process first." >&2
    exit 1
  fi
fi

if [[ "${RESET_LEDGER}" == "1" ]]; then
  rm -rf "${LEDGER_DIR}"
fi
mkdir -p "${LEDGER_DIR}"

MINT_PUBKEY="$(solana-keygen pubkey "${MINT_KEYPAIR}")"

solana-test-validator \
  --ledger "${LEDGER_DIR}" \
  --rpc-port "${RPC_PORT}" \
  --faucet-port "${FAUCET_PORT}" \
  --bind-address 127.0.0.1 \
  --mint "${MINT_PUBKEY}" \
  >"${LOG_FILE}" 2>&1 &

VALIDATOR_PID=$!
echo "${VALIDATOR_PID}" > "${PID_FILE}"

sleep 1
if ! kill -0 "${VALIDATOR_PID}" 2>/dev/null; then
  echo "validator exited immediately. check ${LOG_FILE}" >&2
  rm -f "${PID_FILE}"
  print_ledger_migration_hint
  tail -n 120 "${LOG_FILE}" || true
  exit 1
fi

READY=0
for _ in $(seq 1 60); do
  if solana --url "${RPC_URL}" cluster-version >/dev/null 2>&1; then
    READY=1
    break
  fi
  sleep 1
done

if [[ "${READY}" != "1" ]]; then
  echo "validator did not become ready. check ${LOG_FILE}" >&2
  kill "${VALIDATOR_PID}" 2>/dev/null || true
  rm -f "${PID_FILE}"
  print_ledger_migration_hint
  tail -n 120 "${LOG_FILE}" || true
  exit 1
fi

disown "${VALIDATOR_PID}" 2>/dev/null || true

echo "localnet started"
echo "pid=${VALIDATOR_PID}"
echo "rpc=${RPC_URL}"
echo "ledger=${LEDGER_DIR}"
echo "log=${LOG_FILE}"
echo "stop: kill $(cat "${PID_FILE}")"
