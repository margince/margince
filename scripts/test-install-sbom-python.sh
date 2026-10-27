#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin"
requirements="$work/pinned requirements.txt"
printf 'fixture==1.0 --hash=sha256:fixture\n' >"$requirements"
cat >"$work/bin/pip" <<'PIP'
#!/usr/bin/env bash
set -euo pipefail
hashes=false
no_cache=false
read_requirement=false
requirement=''
for argument in "$@"; do
	if $read_requirement; then
		requirement="$argument"
		read_requirement=false
	fi
	case "$argument" in
		--require-hashes) hashes=true ;;
		--no-cache-dir) no_cache=true ;;
		-r) read_requirement=true ;;
	esac
done
if ! $hashes || ! $no_cache || [[ "$requirement" != "$FIXTURE_REQUIREMENTS" ]]; then
	echo 'fixture: installation lost hash verification or the literal requirements path' >&2
	exit 90
fi
attempt=0
if [[ -f "$FIXTURE_ATTEMPTS" ]]; then
	read -r attempt <"$FIXTURE_ATTEMPTS"
fi
attempt=$((attempt + 1))
printf '%s\n' "$attempt" >"$FIXTURE_ATTEMPTS"
if ((attempt < FIXTURE_SUCCEED_AT)); then
	echo 'fixture: verified dependency download failed' >&2
	exit 1
fi
PIP
chmod +x "$work/bin/pip"

run_case() {
	local name="$1" succeed_at="$2" expected_status="$3" expected_attempts="$4" status=0
	rm -f "$work/attempts"
	PATH="$work/bin:$PATH" FIXTURE_REQUIREMENTS="$requirements" FIXTURE_ATTEMPTS="$work/attempts" \
		FIXTURE_SUCCEED_AT="$succeed_at" bash "$root/scripts/install-sbom-python.sh" "$requirements" \
		>"$work/stdout" 2>"$work/stderr" || status=$?
	if [[ "$status" -ne "$expected_status" || "$(cat "$work/attempts")" -ne "$expected_attempts" ]]; then
		cat "$work/stderr" >&2
		echo "FAIL: $name" >&2
		exit 1
	fi
	echo "ok: $name"
}

run_case 'a verified installation returns immediately' 1 0 1
run_case 'a failed download can recover with the same hash checks' 3 0 3
run_case 'persistent verification failure fails after the retry budget' 99 1 3
