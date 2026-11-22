#!/usr/bin/env bash
# test-reap-build-caches.sh — prove reap-build-caches.sh keeps the right entry
# and refuses the cases where it cannot know.
#
# The script under test DELETES, so the failure that matters is not "it deleted
# nothing" but "it deleted the entry restore-keys was about to use", and the
# quiet one is a read that returns nothing being reported as a clean sweep. Both
# are covered here. It runs beside the reaper in the scheduled lane for the same
# reason `test-secret-scan.sh` runs beside the scan: a destructive tool that can
# only be observed succeeding is a tool nobody has checked.
#
# `gh` is stubbed on PATH, so no case reaches the network or a real cache.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
stub_dir="$(mktemp -d)"
trap 'rm -rf "$stub_dir"' EXIT
failures=0

# The stub answers the list call from CACHES_TSV and records deletions, so a case
# can assert on exactly which ids the script chose to remove.
cat >"$stub_dir/gh" <<'STUB'
#!/usr/bin/env bash
case "$*" in
*"-X DELETE"*)
	for arg in "$@"; do
		case "$arg" in */actions/caches/*) echo "${arg##*/}" >>"$DELETED_LOG" ;; esac
	done
	;;
*actions/caches*) printf '%s' "$CACHES_TSV" ;;
*) echo "unexpected gh call: $*" >&2; exit 1 ;;
esac
STUB
chmod +x "$stub_dir/gh"
export PATH="$stub_dir:$PATH"

# expect <name> <expected-exit> <expected-deleted-ids> <tsv>
expect() {
	local name="$1" want_exit="$2" want_deleted="$3" tsv="$4"
	local out status deleted
	export DELETED_LOG="$stub_dir/deleted"
	: >"$DELETED_LOG"
	set +e
	out="$(CACHES_TSV="$tsv" REPO=owner/repo REF="${REF:-}" "$root/scripts/reap-build-caches.sh" 2>&1)"
	status=$?
	set -e
	deleted="$(paste -sd, - <"$DELETED_LOG")"

	if [[ "$status" -ne "$want_exit" ]] || [[ "$deleted" != "$want_deleted" ]]; then
		echo "FAIL: $name"
		echo "  exit    want $want_exit got $status"
		echo "  deleted want '$want_deleted' got '$deleted'"
		printf '  output: %s\n' "$out" | head -5
		failures=$((failures + 1))
	else
		echo "ok: $name"
	fi
}

main=refs/heads/main
develop=refs/heads/develop
plain_new=$'2026-08-12T04:00:00Z\t101\t'"$main"$'\tgo-build-plain-Linux-X64-deadbeef-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\t3000000000'
plain_old=$'2026-08-12T03:00:00Z\t102\t'"$main"$'\tgo-build-plain-Linux-X64-deadbeef-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\t3000000000'
plain_older=$'2026-08-12T02:00:00Z\t103\t'"$main"$'\tgo-build-plain-Linux-X64-deadbeef-cccccccccccccccccccccccccccccccccccccccc\t3000000000'
integ=$'2026-08-12T03:30:00Z\t201\t'"$main"$'\tgo-build-integration-Linux-X64-deadbeef-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\t2000000000'
other_deps=$'2026-08-12T01:00:00Z\t301\t'"$main"$'\tgo-build-plain-Linux-X64-cafe1234-dddddddddddddddddddddddddddddddddddddddd\t3000000000'
unrelated=$'2026-08-12T03:00:00Z\t401\t'"$main"$'\tnode-cache-Linux-x64-pnpm-abc123\t70000000'
develop_new=$'2026-08-12T05:00:00Z\t501\t'"$develop"$'\tgo-build-plain-Linux-X64-deadbeef-eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee\t3000000000'
frontend_new=$'2026-08-12T04:00:00Z\t601\t'"$main"$'\tfrontend-incremental-Linux-X64-abc123-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\t90000000'
frontend_old=$'2026-08-12T01:00:00Z\t602\t'"$main"$'\tfrontend-incremental-Linux-X64-abc123-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\t90000000'
golangci_new=$'2026-08-12T04:00:00Z\t701\t'"$develop"$'\tgolangci-analysis-in-image-Linux-X64-abc123-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\t400000000'
golangci_old=$'2026-08-12T02:00:00Z\t702\t'"$develop"$'\tgolangci-analysis-in-image-Linux-X64-abc123-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\t400000000'
golangci_main=$'2026-08-12T01:00:00Z\t703\t'"$main"$'\tgolangci-analysis-in-image-Linux-X64-abc123-cccccccccccccccccccccccccccccccccccccccc\t400000000'
develop_old=$'2026-08-12T03:45:00Z\t502\t'"$develop"$'\tgo-build-plain-Linux-X64-deadbeef-ffffffffffffffffffffffffffffffffffffffff\t3000000000'

# The newest of a deps-hash group survives; the rest go. Fed oldest-first on
# purpose — the script must not depend on the order the API happened to use.
expect "keeps the newest, deletes the superseded" 0 "102,103" \
	"$plain_older"$'\n'"$plain_old"$'\n'"$plain_new"

# Flavours are separate groups: one entry each means nothing to reap.
expect "flavours do not shadow each other" 0 "" "$plain_new"$'\n'"$integ"

# A different deps hash is a different group and restore-keys can still fall
# back to it after a go.sum bump, so it is not superseded.
expect "an older deps hash is left alone" 0 "" "$plain_new"$'\n'"$other_deps"

# Everything that is not a build cache is out of scope entirely.
expect "non-build caches are never touched" 0 "" "$unrelated"$'\n'"$plain_new"

# Each ref serves readers of its own, so the newest develop entry must not
# delete main's: pull requests into main and the merge queue restore that one.
expect "keeps the newest of each ref" 0 "502,102" \
	"$plain_old"$'\n'"$develop_old"$'\n'"$plain_new"$'\n'"$develop_new"

# The frontend's incremental state is keyed the same way and reaped the same way.
expect "keeps the newest frontend entry" 0 "602" \
	"$frontend_old"$'\n'"$frontend_new"$'\n'"$plain_new"

# The golangci analysis cache is keyed the same way, per ref: lint-cache.yml prunes
# develop's with REF and leaves main's, which is older, alone.
REF="$develop" expect "keeps the newest golangci entry of the ref" 0 "702" \
	"$golangci_main"$'\n'"$golangci_old"$'\n'"$golangci_new"$'\n'"$develop_new"

# REF limits the sweep to one ref: main's superseded entry is not this run's to delete.
REF="$develop" expect "REF leaves other refs alone" 0 "502" \
	"$plain_old"$'\n'"$develop_old"$'\n'"$plain_new"$'\n'"$develop_new"

# REF naming a ref with no build cache means the writer saved nothing there.
REF="$develop" expect "REF with no build cache on that ref fails loudly" 1 "" \
	"$plain_new"$'\n'"$plain_old"

# A frontend entry alone does not stand in for the Go build caches.
expect "frontend entries alone fail loudly" 1 "" "$frontend_new"

# Without a ref the script cannot tell two branches apart, so it refuses.
expect "a cache with no ref fails loudly" 1 "" \
	$'2026-08-12T04:00:00Z\t101\t\tgo-build-plain-Linux-X64-deadbeef-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\t3000000000'

# A read that returned nothing must not pass for a clean sweep.
expect "an empty listing fails loudly" 1 "" ""

# If the key stops carrying a -<sha> suffix, grouping would silently merge
# unrelated entries — refuse rather than guess.
expect "a key shape change fails loudly" 1 "" $'2026-08-12T04:00:00Z\t101\trefs/heads/main\tgo-build-plain\t3000000000'

# A listing with caches but no build caches means the prefix moved.
expect "a missing build-cache prefix fails loudly" 1 "" "$unrelated"

if [[ "$failures" -ne 0 ]]; then
	echo "FAIL: $failures case(s)" >&2
	exit 1
fi
echo "OK: reap-build-caches keeps the reachable entry and refuses what it cannot judge"
