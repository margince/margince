#!/usr/bin/env bash
set -euo pipefail

# Composition hashes the SDK source tree, so intermediate contracts must stay outside it.

gen_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
gen_tmp="$(mktemp -d "${TMPDIR:-/tmp}/margince-crm-contract.XXXXXX")"
cleanup() {
  local gen_status=$?
  trap - EXIT
  if ! rm -rf -- "$gen_tmp"; then
    echo "CRM SDK generation: could not remove its temporary contracts" >&2
    if [[ "$gen_status" -eq 0 ]]; then gen_status=1; fi
  fi
  exit "$gen_status"
}
trap cleanup EXIT

cd "$gen_root/backend/pkg/extension/crm"
go run github.com/margince/margince/backend/tools/contract-subset \
  -in ../../../api/crm.yaml -out "$gen_tmp/subset31.yaml" \
  -schemas Activity,CreateActivityRequest,Deal,Company,Signal
go run github.com/margince/margince/backend/tools/contract-overlay \
  -in "$gen_tmp/subset31.yaml" -out "$gen_tmp/subset30.yaml"
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.7.1 \
  --config oapi.yaml "$gen_tmp/subset30.yaml"
