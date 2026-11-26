#!/usr/bin/env bash
# Plants good and bad scripts and holds check-early-pipe-readers.sh to a verdict
# on each, and to a failing verdict on an empty census.
# No pipefail here, and no pipe below: the planted bad cases are text in this
# file, and a pipefail script holding them would fail the gate it tests.
set -eu

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
gate="$root/scripts/check-early-pipe-readers.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

failures=0

# verdict <want: pass|fail> <name> <script body>
verdict() {
  local want="$1" name="$2" body="$3" got=pass
  printf '#!/usr/bin/env bash\nset -euo pipefail\n%s\n' "$body" >"$work/$name.sh"
  bash "$gate" "$work/$name.sh" >/dev/null 2>&1 || got=fail
  if [[ "$got" != "$want" ]]; then
    echo "FAIL: $name should $want, the gate said $got" >&2
    failures=$((failures + 1))
  fi
}

verdict fail grep-q 'if git ls-tree -r HEAD | grep -qx x; then :; fi'
verdict fail grep-q-split-flags 'if producer | grep -E -q x; then :; fi'
verdict fail grep-m 'v="$(producer | grep -m1 x)"'
verdict fail head 'v="$(producer | head -1)"'
verdict fail head-n 'v="$(producer | head -n 3)"'
verdict fail awk-exit 'v="$(producer | awk "/x/ { print; exit }")"'
verdict fail sed-quit 'v="$(producer | sed -n "/x/q;p")"'
verdict fail continuation 'v="$(producer \
  | head -1)"'
verdict fail trailing-pipe 'v="$(producer |
  head -1)"'

verdict fail awk-exit-on-later-line "v=\"\$(producer | awk '
  /x/ { print; exit }
')\""
verdict fail tar-extract 'git archive HEAD | tar -x -C /tmp/x'
verdict pass awk-exit-in-end-waived "v=\"\$(producer | awk '
  # early-pipe-ok: exit only in END
  END { exit 1 }
')\""

verdict pass here-string 'out="$(producer)"; grep -q x <<<"$out"'
verdict pass reads-to-end 'v="$(producer | sed -n 1p)"; w="$(producer | grep x >/dev/null)"'
verdict pass awk-no-exit 'v="$(producer | awk "NR <= 3 { print }")"'
verdict pass or-operator 'producer || head -1 file'
verdict pass comment-only '# producer | head -1'
verdict pass waived 'v="$(producer | head -1)" # early-pipe-ok: producer prints one line'

printf '#!/usr/bin/env bash\nproducer | head -1\n' >"$work/no-pipefail.sh"
if bash "$gate" "$work/no-pipefail.sh" >/dev/null 2>&1; then
  echo "FAIL: a census of zero pipefail scripts passed" >&2
  failures=$((failures + 1))
fi

if [[ "$failures" -gt 0 ]]; then
  echo "$failures check-early-pipe-readers case(s) failed" >&2
  exit 1
fi
echo "check-early-pipe-readers.test: all cases held"
