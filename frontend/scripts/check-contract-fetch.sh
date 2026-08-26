#!/usr/bin/env bash
# Contract-transport gate: a path the OpenAPI contract covers is reached through
# the generated client, never through a hand-written fetch.
#
# What a bare fetch to a contract path gives up, all of which api.GET/api.POST
# provide: the path stops being an operation and becomes a string, so a contract
# rename leaves the call compiling and 404-ing at runtime with no drift gate able
# to see it; request and response bodies lose the types derived from crm.yaml;
# and the shared problem-document path is re-derived locally, one screen at a
# time.
#
# The refused prefix is READ FROM the contract's own `servers:` block rather than
# written here. A gate that hard-codes part of its subject has become a second
# copy of it: moving the mount to /v2 must change what this refuses, in the same
# edit, without anyone remembering this file exists.
#
# A genuine exception declares itself in source, beside the call, on the line
# above it:
#
#   // contract-fetch:allow <reason>
#   const response = await fetch("/v1/attachments", { method: "POST", body: form })
#
# That is the multipart case — the generated client serializes JSON only — and a
# reason is required because a waiver nobody has to justify is not a waiver.
#
# Usage: frontend/scripts/check-contract-fetch.sh   (wired into `make frontend-check`)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SRC_DIR="$FE_DIR/src"
ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# The unit trees are swept too: a unit's screen calls the same API over the same
# session, and a gate that stopped at frontend/src would hold the core to a
# standard the extension tier escapes. Overridable so the gate's own test can
# point it at a fixture.
EXT_DIR="${MARGINCE_EXT_DIR:-$ROOT/extensions}"
CONTRACT="${MARGINCE_CONTRACT:-$ROOT/backend/api/crm.yaml}"

if [[ ! -f "$CONTRACT" ]]; then
  echo "FAIL: no contract at $CONTRACT — the gate cannot derive what to refuse" >&2
  exit 1
fi

# The mount is the path component of the servers' url, and every server entry
# has to agree on it: two different mounts would mean two answers to "what does
# a contract path look like", and this gate would have to pick one.
MOUNTS="$(
  awk '/^servers:/ { inservers = 1; next }
       inservers && /^[a-zA-Z]/ { exit }
       inservers && /^[[:space:]]*-[[:space:]]*url:/ { print }' "$CONTRACT" \
    | sed -E 's|.*url:[[:space:]]*||; s|^[a-z]+://[^/]+||; s|/+$||' \
    | sort -u
)"
MOUNT_COUNT="$(grep -c . <<<"$MOUNTS" || true)"
if [[ -z "$MOUNTS" || "$MOUNT_COUNT" -ne 1 ]]; then
  echo "FAIL: the contract's servers do not agree on one mount path:" >&2
  sed 's/^/      /' <<<"$MOUNTS" >&2
  echo "      This gate refuses hand-written calls to that mount, so it needs" >&2
  echo "      exactly one. Reconcile crm.yaml's servers, or teach this gate" >&2
  echo "      which of them the client uses." >&2
  exit 1
fi
MOUNT="$MOUNTS"
if [[ "$MOUNT" != /* ]]; then
  echo "FAIL: derived mount '$MOUNT' is not a path — check crm.yaml's servers block" >&2
  exit 1
fi

# Excluded: the generated client (its transport IS fetch — that is the seam this
# gate exists to funnel everything through), generated contract types, and test
# files, which stub transports rather than ship them.
FILES=()
while IFS= read -r -d '' f; do FILES+=("$f"); done < <(
  find "$SRC_DIR" -type f \( -name "*.ts" -o -name "*.tsx" \) \
    -not -name "*.test.*" \
    -not -name "schema.d.ts" \
    -not -path "$SRC_DIR/api/client.ts" \
    -print0 2>/dev/null
)
while IFS= read -r -d '' f; do FILES+=("$f"); done < <(
  find "$EXT_DIR" -type f \( -name "*.ts" -o -name "*.tsx" \) \
    -path "*/frontend/*" \
    -not -path "*/node_modules/*" \
    -not -name "*.test.*" \
    -print0 2>/dev/null
)

# An empty scan means the gate is pointed at the wrong tree — fail closed.
if [[ "${#FILES[@]}" -eq 0 ]]; then
  echo "FAIL: contract-fetch found no files under $SRC_DIR or $EXT_DIR — the gate is miswired" >&2
  exit 1
fi

echo "==> contract-fetch check (mount $MOUNT, ${#FILES[@]} files under frontend/src + extensions/*/frontend)"

# A call to the mount, however the URL is spelled: a quoted literal, an absolute
# URL, or a template literal built onto the origin. What is matched is the MOUNT
# appearing inside the string the call is given — anchoring on a line shape
# instead is what would miss the spelling nobody thought of, and the origin form
# is the one that got past the first draft of this gate.
#
# The mount may be followed by the path separator OR by the end of the string,
# because a path assembled across adjacent literals — `fetch("/v1" + "/deals")`
# — puts the separator in the NEXT literal. Requiring the slash read past that
# spelling entirely, which is the one shape of miss this gate must not have: it
# would have reported PASS over a hand-written call to the contract.
PATTERN="fetch\\([^)]*[\"\`'][^\"\`']*${MOUNT}(/|[\"\`'])"

HITS="$(printf '%s\0' "${FILES[@]}" | xargs -0 grep -nHE "$PATTERN" || true)"

EXIT=0
UNWAIVED=""
while IFS= read -r hit; do
  [[ -z "$hit" ]] && continue
  file="${hit%%:*}"
  rest="${hit#*:}"
  line="${rest%%:*}"
  # The waiver sits on the line above the call, so a reader meets the reason
  # before the exception. An empty reason is not a waiver.
  above=""
  if [[ "$line" -gt 1 ]]; then
    above="$(sed -n "$((line - 1))p" "$file")"
  fi
  if [[ "$above" =~ contract-fetch:allow[[:space:]]+[^[:space:]] ]]; then
    continue
  fi
  UNWAIVED+="$hit"$'\n'
  EXIT=1
done <<<"$HITS"

if [[ "$EXIT" == "0" ]]; then
  echo "PASS — every $MOUNT path goes through the generated client"
else
  echo ""
  echo "FAIL: hand-written fetch to a contract path (use api.GET/api.POST from src/api/client)"
  printf '%s' "$UNWAIVED"
  echo ""
  echo "A path under $MOUNT is a contract OPERATION, and the generated client is"
  echo "how it stays one: the path is checked, the bodies are typed from crm.yaml,"
  echo "and errors take the shared problem-document route. If the client genuinely"
  echo "cannot carry this call — a multipart upload is the known case — say so on"
  echo "the line above it:  // contract-fetch:allow <reason>"
fi

exit $EXIT
