#!/usr/bin/env bash
# SPDX-License-Identifier: BUSL-1.1
# SPDX-FileCopyrightText: 2026 Gradion
#
# aicert-run-cost.sh's own test. A journal is reused across runs, so the case
# plants a row from an older run beside this run's rows: a count that took it in
# would overstate the run, and look just as plausible as the right one.
#
# Usage: bash scripts/aicert-run-cost.test.sh

set -euo pipefail

SCRIPT="$(cd "$(dirname "$0")" && pwd)/aicert-run-cost.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

FAILURES=0
fail() { echo "FAIL: $*" >&2; FAILURES=$((FAILURES + 1)); }

now=$(date +%s)
iso() { jq -nr --argjson t "$1" '$t | strftime("%Y-%m-%dT%H:%M:%S.000000Z")'; }
mkdir -p "$TMP/aicert/resume"
{
	printf '{"at":"%s","outcome":{"served_model":"vendor/model-a","tokens_in":1000000,"tokens_out":200000}}\n' "$(iso $((now - 100)))"
	printf '{"at":"%s","outcome":{"served_model":"vendor/model-a","tokens_in":500000,"tokens_out":100000}}\n' "$(iso $((now - 50)))"
	printf '{"at":"%s","outcome":{"served_model":"vendor/model-a","tokens_in":9000000,"tokens_out":9000000}}\n' "$(iso $((now - 86400)))"
} >"$TMP/aicert/resume/aicert-resume.jsonl"
trace="$TMP/aicert/$(jq -nr --argjson t $((now - 150)) '$t | strftime("aicert-trace-%Y%m%dT%H%M%SZ")').jsonl"
{
	echo '{"role":"candidate","served_model":"vendor/model-a","served_provider":"Alpha"}'
	echo '{"role":"candidate","served_model":"vendor/model-a","served_provider":"Alpha"}'
	echo '{"role":"candidate","served_model":"vendor/model-a","served_provider":"Beta"}'
	echo '{"role":"judge","served_model":"claude","served_provider":""}'
} >"$trace"
{
	echo '2026/01/01 00:00:00 INFO ai.call task=summarize provider=openai_compatible tokens_in=1'
	echo '2026/01/01 00:00:01 INFO ai.call task=cert_judge provider=claude_cli tokens_in=1'
	echo '--- PASS: TestE2ECertify (120.40s)'
} >"$TMP/run.log"

out=$(PRICE_vendor_model_a="2,10" bash "$SCRIPT" "$TMP/run.log" "$TMP/aicert/resume")

grep -q '^Time: 2 minutes (120s)$' <<<"$out" || fail "time: $out"
grep -q '^Model calls: 1 (judge calls: 1)$' <<<"$out" || fail "calls: $out"
# 1.5M in at $2 and 0.3M out at $10: the older run's row is left out.
grep -qF "vendor/model-a: served by Alpha, 2 runs, tokens in / out 1500000 / 300000, cost \$6.00" <<<"$out" ||
	fail "model line: $out"

if bash "$SCRIPT" "$TMP/missing.log" "$TMP/aicert/resume" 2>/dev/null; then
	fail "a missing run log must fail"
fi

[[ $FAILURES -eq 0 ]] || exit 1
echo "aicert-run-cost: all cases pass"
