#!/usr/bin/env bash
# test-craft-review.sh — prove `craft-review` refuses a reading that did not happen.
#
# The reviewer answers `verdict: PASS` with no findings when it cannot reach the
# model, so the one thing the wrapper must never do is relay that. Every case
# here is a way the reviewer can come back without having read the diff, and each
# is indistinguishable from a clean review by exit code alone — which is why the
# assertions are on the exit code AND on what was said.
#
# The reviewer is stubbed rather than called: it is an external HTTP boundary,
# and a test that reached it would send this diff to a paid API to assert on a
# refusal that never gets that far. The script under test is the real one, copied
# into a temp root so it resolves the stub through its own resolver.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fails=0

fail() {
	echo "FAIL: $*" >&2
	fails=$((fails + 1))
}

# stage <result-json> [verdict-exit] — a temp root holding the real script and a
# stubbed gate that answers `review` with the given result and `verdict` with the
# given exit code (0 unless a case is about a blocked one).
stage() {
	local result="$1" verdict="${2:-0}" dir
	dir="$(mktemp -d)"
	mkdir -p "$dir/scripts"
	cp "$root/scripts/craft-review.sh" "$dir/scripts/craft-review.sh"
	cat >"$dir/scripts/craft-pin.sh" <<'PIN'
#!/usr/bin/env bash
echo "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/craft-stub.sh"
PIN
	cat >"$dir/scripts/craft-stub.sh" <<PINSTUB
#!/usr/bin/env bash
case "\$1" in
review) cat <<'RESULT'
$result
RESULT
;;
verdict)
	exit $verdict
;;
esac
PINSTUB
	chmod +x "$dir/scripts/craft-pin.sh" "$dir/scripts/craft-stub.sh"
	echo "$dir"
}

skipped_result='{"gate_version":"p1","verdict":"PASS","findings":[],"scratchpad":"skipped: ANTHROPIC_API_KEY unset (craftsmanship gate not yet activated)"}'
read_result='{"gate_version":"p1","verdict":"PASS","findings":[],"scratchpad":"read 4 files"}'

# 1. No key: refuse, and name the variable. The message is asserted because an
#    exit code alone sends the reader to the script to find out what to set.
dir="$(stage "$read_result")"
if out="$(cd "$dir" && env -u ANTHROPIC_API_KEY ./scripts/craft-review.sh 2>&1)"; then
	fail "ran with no ANTHROPIC_API_KEY — it would have reported PASS without reviewing"
elif [[ "$out" != *ANTHROPIC_API_KEY* ]]; then
	fail "refused without an API key but did not name ANTHROPIC_API_KEY: $out"
fi

# 2. A key set and the reviewer skipped anyway. This is the case the variable
#    check cannot see: the key can be present and the run still not happen, and
#    the result still says PASS.
dir="$(stage "$skipped_result")"
if out="$(cd "$dir" && ANTHROPIC_API_KEY=stub ./scripts/craft-review.sh 2>&1)"; then
	fail "relayed a skipped result as a pass — an unactivated reviewer would read as a clean diff"
elif [[ "$out" != *skipped* ]]; then
	fail "refused a skipped result without saying it was skipped: $out"
fi

# 3. The reviewer did read the diff and found nothing. The refusals above have to
#    be the two cases they name and not a wrapper that never passes — a gate that
#    always fails is as useless as one that always passes, and this is what tells
#    them apart.
dir="$(stage "$read_result")"
if ! out="$(cd "$dir" && ANTHROPIC_API_KEY=stub ./scripts/craft-review.sh 2>&1)"; then
	fail "refused a result the reviewer actually produced: $out"
fi

# 4. The reviewer read the diff and blocked. The verdict is the gate's, and a
#    wrapper that swallowed it would turn every finding advisory without saying
#    so — the same silence as a review that never ran, one step later.
dir="$(stage "$read_result" 1)"
if (cd "$dir" && ANTHROPIC_API_KEY=stub ./scripts/craft-review.sh >/dev/null 2>&1); then
	fail "reported success over a blocking verdict — the gate's judgement did not reach the caller"
fi

if [[ "$fails" -gt 0 ]]; then
	echo "test-craft-review: $fails failure(s)" >&2
	exit 1
fi
echo "OK: test-craft-review — 4 case(s): no key, a skipped result, a real reading, and a blocked one"
