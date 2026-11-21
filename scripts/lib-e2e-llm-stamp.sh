# shellcheck shell=bash
# lib-e2e-llm-stamp.sh — what every verdict of one sweep measured, worked out
# once. Sourced by scripts/e2e-llm.sh and scripts/e2e-llm-sweep.sh, so a sweep
# of many batches compiles the compose test package once, not once a batch.
#
# e2e_llm_stamp <repo root> <door: mcp|rest> <out dir> writes <out dir>/stamps.json
# and <out dir>/commit. The published catalog is checked against what the tree
# serves first, or a stamp would name copy the model never saw.

# The verdict folders the lane itself writes, which do not make a tree dirty.
E2E_LLM_OWN_OUTPUTS=(
  ':(exclude)backend/internal/compose/aicert/records/mcp_e2e'
  ':(exclude)backend/internal/compose/aicert/records/mcp_e2e_rest'
)

e2e_llm_stamp() {
  local root="$1" door="$2" out="$3" commit
  mkdir -p "$out"
  if ! (cd "$root/backend" && go test ./internal/compose/ -run '^TestPublishedMCPSurfaceMatchesWhatAClientIsServed$' \
    -count=1 >"$out/surface.log" 2>&1); then
    echo "docs/reference/mcp-info.json is not what this tree serves, so a stamp would name copy no model saw:" >&2
    tail -20 "$out/surface.log" >&2
    echo "  regenerate it: cd backend && go test ./internal/compose/ -run TestPublishedMCPSurface -update-mcp-info" >&2
    return 1
  fi
  (cd "$root/backend" && go run ./internal/compose/mcpe2e/mcpe2ecmd stamps -door "$door" -out "$out/stamps.json") || return 1
  commit="$(e2e_llm_commit "$root")" || return 1
  printf '%s\n' "$commit" >"$out/commit"
}

# e2e_llm_commit <repo root> prints the commit a run measures, marked
# +uncommitted when the tree holds a change the lane did not write itself.
e2e_llm_commit() {
  local root="$1" commit changes
  commit="$(git -C "$root" rev-parse HEAD)" || return 1
  changes="$(git -C "$root" status --porcelain -- . "${E2E_LLM_OWN_OUTPUTS[@]}")" || return 1
  [[ -z "$changes" ]] || commit="$commit+uncommitted"
  printf '%s\n' "$commit"
}
