#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
source "$root/scripts/lib-git-fixture.sh"
fixture="$(mktemp -d)"
trap 'rm -rf "$fixture"' EXIT
mkdir -p "$fixture/scripts" "$fixture/backend/api" "$fixture/bin"
cp "$root/scripts/check-contract-breaking.sh" "$fixture/scripts/"
cd "$fixture"
export NOTICE_FIXTURE="$fixture"
cat > bin/go <<'GO'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  env) echo "$NOTICE_FIXTURE/bin" ;;
  version) printf '\tmod\tgithub.com/oasdiff/oasdiff\tv1.22.0\n' ;;
  run)
    shift
    if [[ "$1" == ./backend/tools/check-openenums ]]; then
      echo checked > "$NOTICE_FIXTURE/checked-enums"
      exit "${ENUM_FAILURE:-0}"
    fi
    [[ "$1" == ./backend/tools/check-apideprecation ]] || exit 70
    shift
    while (($#)); do
      case "$1" in
        -base) cp "$2" "$NOTICE_FIXTURE/checked-base"; shift 2 ;;
        -release) echo "$2" > "$NOTICE_FIXTURE/checked-release"; shift 2 ;;
        *) shift ;;
      esac
    done
    exit "${NOTICE_FAILURE:-0}"
    ;;
  *) exit 71 ;;
esac
GO
cat > bin/oasdiff <<'OAS'
#!/usr/bin/env bash
exit 0
OAS
chmod +x bin/go bin/oasdiff
export PATH="$fixture/bin:$PATH"
git init -q
git config user.name Fixture
git config user.email fixture@example.test
echo common > backend/api/crm.yaml
git add backend/api/crm.yaml
git commit -qm common
common="$(git rev-parse HEAD)"
git switch -qc feature
echo feature > README
git add README
git commit -qm feature
git switch -qc comparison "$common"
echo newer-base > backend/api/crm.yaml
git add backend/api/crm.yaml
git commit -qm comparison
git switch -q feature
bash scripts/check-contract-breaking.sh comparison > /dev/null
[[ "$(cat checked-base)" == common ]] || { echo 'PR check did not use the merge base' >&2; exit 1; }
bash scripts/check-contract-breaking.sh comparison v1.1.0 > /dev/null
[[ "$(cat checked-base)" == newer-base ]] || { echo 'Release check discarded the exact patch base' >&2; exit 1; }
[[ "$(cat checked-release)" == v1.1.0 ]] || { echo 'Release version did not reach the notice check' >&2; exit 1; }
if NOTICE_FAILURE=1 CONTRACT_STABILITY=pre-live bash scripts/check-contract-breaking.sh comparison > /dev/null 2>&1; then
  echo 'Pre-live mode bypassed the notice check' >&2
  exit 1
fi
old="$(git rev-parse "$common:backend/api/crm.yaml")"
new="$(git hash-object backend/api/crm.yaml)"
printf '%s %s fixture\n' "$old" "$new" > scripts/contract-breaking-allowlist.txt
if NOTICE_FAILURE=1 bash scripts/check-contract-breaking.sh comparison > /dev/null 2>&1; then
  echo 'An exact compatibility exception bypassed the notice check' >&2
  exit 1
fi
[[ -f checked-enums ]] || { echo 'The enum rule was not checked' >&2; exit 1; }
if ENUM_FAILURE=1 CONTRACT_STABILITY=pre-live bash scripts/check-contract-breaking.sh comparison > /dev/null 2>&1; then
  echo 'A closed response enum passed the breaking check' >&2
  exit 1
fi
if ENUM_FAILURE=1 bash scripts/check-contract-breaking.sh absent > /dev/null 2>&1; then
  echo 'A closed response enum passed when the base was absent' >&2
  exit 1
fi
if CONTRACT_BREAKING_REQUIRE_BASE=1 bash scripts/check-contract-breaking.sh absent > /dev/null 2>&1; then
  echo 'A required absent base was silently skipped' >&2
  exit 1
fi
echo 'OK: API notice and enum checks keep exact release bases and cannot be bypassed'
