#!/usr/bin/env bash
# craft-review.sh — the OPT-IN, model-driven half of the craftsmanship gate.
#
# `craft static` is the half that is ENFORCED: the pre-push hook runs it
# diff-scoped and CI runs it as a required job. This is the other half. It sends
# the diff to an external model API, judges what a syntax tree cannot see, and
# blocks nothing: no hook calls it and no CI job calls it. Somebody runs it
# because they want a second reading before review.
#
# The failure this script exists to prevent is the reviewer's own answer when it
# cannot run. With no key it exits 0 and prints verdict PASS with an empty
# findings list, and `craft verdict` reads that field — so an unactivated
# reviewer and a clean diff are the same output all the way down the chain. A
# wrapper that just relayed it would report a clean review to everybody who has
# not set a key, which is the shape that reports PASS while judging nothing.
#
# So it refuses twice. The unset variable is the early, readable message; the
# skipped RESULT is the one that cannot fail short, because it judges what the
# run did rather than what was set before it.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
base="${BASE:-origin/main}"
out="${root}/.tmp/craft/review-result.json"

if [[ -z "${ANTHROPIC_API_KEY:-}" ]]; then
	cat >&2 <<'MSG'
FAIL: craft-review needs ANTHROPIC_API_KEY.

This is the opt-in model-driven arm of the craftsmanship gate. It calls an
external API and costs money per run; nothing sets the key for you.

  ANTHROPIC_API_KEY=... make craft-review          # review vs origin/main
  ANTHROPIC_API_KEY=... BASE=<ref> make craft-review

The enforced arm needs no key and is what blocks a push: `make craft-static`.
MSG
	exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
	echo "FAIL: craft-review reads the reviewer's result with jq, which is not on PATH." >&2
	exit 1
fi

bin="$("$root/scripts/craft-pin.sh")"
mkdir -p "$(dirname "$out")"

echo "craft review: ${base}...HEAD (external model API, advisory — blocks no push)"
"$bin" review --base "$base" --head HEAD --root "$root" >"$out"

# A result the reviewer did not produce by reviewing. It records why in
# `scratchpad` and still says PASS, so this is the only place the two are told
# apart — matched on the reviewer's own prefix rather than on one message, so a
# second reason to skip is caught by the same line that catches the first.
scratchpad="$(jq -r '.scratchpad // ""' "$out")"
if [[ "$scratchpad" == skipped:* ]]; then
	echo "FAIL: the reviewer did not run: ${scratchpad}" >&2
	echo "Its PASS says nothing about this diff. Result kept at ${out}." >&2
	exit 1
fi

jq -r '"gate " + .gate_version + " — " + .verdict + ", " + (.findings | length | tostring) + " finding(s)"' "$out"
jq -r '.findings[]? | "  [" + (.severity // "?") + "] " + (.file // "?") + ": " + (.title // .summary // "?")' "$out"
echo "result: ${out}"

# The verdict is the gate's to give, not this script's to re-derive from the
# findings — a wrapper counting them itself would be a second implementation of
# the same judgement, and the two would disagree the first time a severity
# stopped blocking.
"$bin" verdict --result "$out"
