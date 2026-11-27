#!/usr/bin/env bash
# test-ci-integration-proven.sh — prove the merge queue skips the integration
# lane only on a tree a run of the entry's own pull request head passed.
#
# The case that matters is a wrong `proven=true`: the queue would then merge a
# tree no shard ever ran. Each refusal below is one way the proof can be forged
# or incomplete. `gh` is stubbed on PATH, so no case reaches the network.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
stub_dir="$(mktemp -d)"
trap 'rm -rf "$stub_dir"' EXIT
failures=0

# The stub answers each read from a STUB_* variable, applies the --jq filter the
# script passes, and fails a read whose variable is FAIL.
cat >"$stub_dir/gh" <<'STUB'
#!/usr/bin/env bash
filter=""
endpoint=""
while [[ $# -gt 0 ]]; do
	case "$1" in
	api | --paginate) shift ;;
	--jq) filter="$2"; shift 2 ;;
	*) endpoint="$1"; shift ;;
	esac
done
case "$endpoint" in
*/git/commits/*) body="$STUB_COMMIT" ;;
*/pulls/*) body="$STUB_PULL" ;;
*/actions/artifacts\?*) body="$STUB_ARTIFACTS" ;;
*/jobs\?*) body="$STUB_JOBS" ;;
*/actions/runs/*) body="$STUB_RUN" ;;
*) echo "unexpected gh call: $endpoint" >&2; exit 1 ;;
esac
[[ "$body" != FAIL ]] || { echo "HTTP 502" >&2; exit 1; }
jq -r "$filter" <<<"$body"
STUB
chmod +x "$stub_dir/gh"
export PATH="$stub_dir:$PATH"

pr_head=1111111111111111111111111111111111111111
other_head=2222222222222222222222222222222222222222
ref=refs/heads/gh-readonly-queue/main/pr-42-3333333333333333333333333333333333333333

artifact() { printf '{"expired":%s,"workflow_run":{"id":%s,"head_sha":"%s"}}' "$1" "$2" "$3"; }
artifacts() { local IFS=,; printf '{"artifacts":[%s]}' "$*"; }
job() { printf '{"name":"%s","conclusion":%s}' "$1" "$2"; }
jobs() { local IFS=,; printf '{"jobs":[%s]}' "$*"; }

green_lane="$(jobs "$(job 'integration / integration build' '"success"')" \
	"$(job 'integration / integration shard (1/8)' '"success"')" \
	"$(job 'frontend / fe-unit' '"failure"')")"

# expect <name> <want-output> [VAR=value...]
expect() {
	local name="$1" want="$2" got
	shift 2
	got="$(env STUB_COMMIT='{"tree":{"sha":"abc"}}' STUB_PULL="{\"head\":{\"sha\":\"$pr_head\"}}" \
		STUB_ARTIFACTS="$(artifacts "$(artifact false 7 "$pr_head")")" \
		STUB_RUN='{"path":".github/workflows/ci.yml"}' STUB_JOBS="$green_lane" \
		REPO=owner/repo MERGE_GROUP_SHA=4444 MERGE_GROUP_REF="$ref" \
		"$@" "$root/scripts/ci-integration-proven.sh" 2>/dev/null)"
	if [[ "$got" == "$want" ]]; then
		echo "ok: $name"
	else
		echo "FAIL: $name: want '$want', got '$got'" >&2
		failures=$((failures + 1))
	fi
}

# A job outside the lane may fail; the merge queue runs that lane itself.
expect "the pull request's green run proves the tree" proven=true

expect "no artifact names the tree" proven=false STUB_ARTIFACTS='{"artifacts":[]}'
expect "a run of another head cannot vouch for the tree" proven=false \
	STUB_ARTIFACTS="$(artifacts "$(artifact false 7 "$other_head")")"
expect "an expired artifact is no proof" proven=false \
	STUB_ARTIFACTS="$(artifacts "$(artifact true 7 "$pr_head")")"
expect "a run of another workflow is no proof" proven=false \
	STUB_RUN='{"path":".github/workflows/main-health.yml"}'
expect "a failed lane job is no proof" proven=false \
	STUB_JOBS="$(jobs "$(job 'integration / integration build' '"success"')" \
		"$(job 'integration / integration (extension migration gate)' '"failure"')")"
expect "an unfinished lane job is no proof" proven=false \
	STUB_JOBS="$(jobs "$(job 'integration / integration build' '"success"')" \
		"$(job 'integration / integration shard (1/8)' null)")"
expect "a run with no lane jobs is no proof" proven=false \
	STUB_JOBS="$(jobs "$(job 'frontend / fe-unit' '"success"')")"

# A failed read runs the lane rather than failing the merge group.
expect "a failed tree read runs the lane" proven=false STUB_COMMIT=FAIL
expect "a failed artifact read runs the lane" proven=false STUB_ARTIFACTS=FAIL
expect "a failed job read runs the lane" proven=false STUB_JOBS=FAIL
expect "a ref without a pull request runs the lane" proven=false \
	MERGE_GROUP_REF=refs/heads/gh-readonly-queue/main/unexpected

if [[ "$failures" -ne 0 ]]; then
	echo "FAIL: $failures case(s) failed" >&2
	exit 1
fi
echo "OK: ci-integration-proven skips the lane only on a proven tree"
