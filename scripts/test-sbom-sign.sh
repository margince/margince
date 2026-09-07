#!/usr/bin/env bash
# Prove `make sbom-sign` is recoverable: signing again is what orphans a Rekor
# entry, so a file that already carries a current bundle must not be signed a
# second time — and one whose SBOM has moved under its bundle must be.
#
# A Rekor entry is permanent and cannot be retracted. A re-run that mints a
# second entry leaves the first as a claim in a public append-only log that no
# published artifact corroborates, which is the defect this behaviour exists to
# prevent. Nothing else in the tree fails when the skip goes away: the target
# would simply sign again and look exactly like a clean run.
#
# cosign is stubbed. The real one needs an OIDC token and writes to a public
# log, so the thing under test here is WHICH files reach it — the decision the
# Makefile makes — and never sigstore's own behaviour.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

stub="$work/cosign-stub"
cat >"$stub" <<'STUB'
#!/usr/bin/env bash
# Records one line per invocation and writes the bundle cosign would write.
bundle=''
blob=''
prev=''
for arg in "$@"; do
  case "$prev" in --bundle) bundle="$arg" ;; esac
  case "$arg" in -*) ;; *) [ "$prev" = "--bundle" ] || blob="$arg" ;; esac
  prev="$arg"
done
echo "$blob" >>"$COSIGN_STUB_LOG"
[ -n "$bundle" ] && printf 'stub-bundle\n' >"$bundle"
exit 0
STUB
chmod +x "$stub"

# The SBOMs the target signs, faked. The parity and validate gates run pinned
# containers over the real tree and are each tested where they live, so they are
# lifted off rather than satisfied — what is under test here is the skip
# decision, which runs after them either way.
run_sign() {
  export COSIGN_STUB_LOG="$1"
  : >"$COSIGN_STUB_LOG"
  make -s sbom-sign COSIGN="$stub" SBOM_DIR="$sboms" SBOM_SIGN_DEPS= >/dev/null 2>&1 || return $?
}

fail() { echo "FAIL: $*" >&2; exit 1; }

sboms="$work/sboms"
mkdir -p "$sboms"
for name in margince.cdx.json margince.spdx221.json margince.spdx300.json; do
  printf '{}\n' >"$sboms/$name"
done

log="$work/first.log"
if ! run_sign "$log"; then fail "the first sbom-sign run did not succeed"; fi
signed=$(grep -c . "$log" || true)
[ "$signed" -eq 3 ] || fail "the first run signed $signed file(s), want 3 — an unsigned SBOM is the failure this target exists to prevent"

log="$work/second.log"
if ! run_sign "$log"; then fail "the re-run did not succeed"; fi
signed=$(grep -c . "$log" || true)
[ "$signed" -eq 0 ] || fail "the re-run signed $signed file(s) that already carry a current bundle — every one of those is a second permanent Rekor entry, and the first is left corroborating nothing"

# An SBOM regenerated under its bundle IS signed again: the old bundle covers
# bytes that are gone, and skipping there would publish a signature that
# verifies against nothing.
sleep 1
printf '{"changed":true}\n' >"$sboms/margince.cdx.json"
log="$work/third.log"
if ! run_sign "$log"; then fail "the run over a regenerated SBOM did not succeed"; fi
signed=$(grep -c . "$log" || true)
[ "$signed" -eq 1 ] || fail "a regenerated SBOM under an older bundle was signed $signed time(s), want 1 — its bundle covers bytes that no longer exist"
grep -q 'margince.cdx.json$' "$log" || fail "the run signed something other than the SBOM that changed"

# An empty bundle is not a signature. A cancelled write can leave one.
: >"$sboms/margince.spdx221.json.cosign.bundle"
log="$work/fourth.log"
if ! run_sign "$log"; then fail "the run over an empty bundle did not succeed"; fi
grep -q 'margince.spdx221.json$' "$log" || fail "a zero-byte bundle was accepted as a signature — that is what a write cancelled mid-flight leaves behind"

echo "OK: sbom-sign signs what is unsigned and does not mint a second Rekor entry for what is not"
