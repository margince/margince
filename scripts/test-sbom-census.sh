#!/usr/bin/env bash
# Prove `make sbom-census` fails on a short census: an SBOM that lists fewer of
# the npm packages the lockfiles resolve than the lockfiles hold.
#
# grant judges only what an SBOM lists, so a scanner that reads part of a
# lockfile still yields a valid, non-empty SBOM and a green license gate over
# the part it read. pnpm writes the lockfile as a stream of YAML documents, the
# package manager's own binaries first and the workspace after, and a reader
# that stops after the first one loses every package the web app uses. Nothing
# else in the tree fails when that happens, so each case plants lockfiles and an
# SBOM and asserts the verdict sentence on the stream sbom-gate shows, stderr.
#
# The YAML reader is the real one, in the pinned Python image: reading every
# document is the behaviour under test, and a stub would prove nothing about
# it. So this needs Docker and network, which the license-gate job has.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
bash scripts/test-install-sbom-python.sh

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0

# The shape pnpm writes: an env document with the package manager's own
# binaries, then the workspace. Its snapshots carry peer suffixes that are not
# packages of their own, and one package resolves at two versions.
env_document="$(cat <<'YAML'
---
lockfileVersion: '9.0'

importers:

  .:
    configDependencies: {}
    packageManagerDependencies:
      toolchain:
        specifier: 1.0.0
        version: 1.0.0

packages:

  '@toolchain/exe.linux-x64@1.0.0':
    resolution: {integrity: sha512-AAAA}
    cpu: [x64]
    os: [linux]

  toolchain@1.0.0:
    resolution: {integrity: sha512-BBBB}

snapshots:

  '@toolchain/exe.linux-x64@1.0.0':
    optional: true

  toolchain@1.0.0:
    optionalDependencies:
      '@toolchain/exe.linux-x64': 1.0.0
YAML
)"

# workspace_document <version> <key holding the packages>
workspace_document() {
	cat <<YAML

---
lockfileVersion: '$1'

settings:
  autoInstallPeers: true
  excludeLinksFromLockfile: false

importers:

  .: {}

  frontend:
    dependencies:
      alpha:
        specifier: 2.0.0
        version: 2.0.0
      delta:
        specifier: 3.1.4
        version: 3.1.4(alpha@1.0.0)
    devDependencies:
      '@beta/gamma':
        specifier: 2.0.0
        version: 2.0.0

$2:

  '@beta/gamma@2.0.0':
    resolution: {integrity: sha512-CCCC}

  alpha@1.0.0:
    resolution: {integrity: sha512-DDDD}

  alpha@2.0.0:
    resolution: {integrity: sha512-EEEE}

  delta@3.1.4:
    resolution: {integrity: sha512-FFFF}
    peerDependencies:
      alpha: ^1.0.0

snapshots:

  '@beta/gamma@2.0.0': {}

  alpha@1.0.0: {}

  alpha@2.0.0: {}

  delta@3.1.4(alpha@1.0.0):
    dependencies:
      alpha: 1.0.0
YAML
}

lockfile="$work/pnpm-lock.yaml"
printf '%s\n%s\n' "$env_document" "$(workspace_document 9.0 packages)" >"$lockfile"

# sbom_with <name@version>... — a CycloneDX document listing these npm
# packages, beside a file and a PyPI package that shares a name and version
# with one the lockfile resolves. Neither is an npm package, so neither counts.
sbom_with() {
	local dir components='' package name version
	dir="$(mktemp -d "$work/sboms.XXXXXX")"
	for package in "$@"; do
		name="${package%@*}"
		version="${package##*@}"
		components+="$(jq -cn --arg n "$name" --arg v "$version" \
			'{type: "library", name: $n, version: $v, purl: ("pkg:npm/" + ($n | sub("^@"; "%40")) + "@" + $v)}'),"
	done
	components+='{"type":"library","name":"delta","version":"3.1.4","purl":"pkg:pypi/delta@3.1.4"},'
	components+='{"type":"file","name":"frontend/package.json"}'
	printf '{"bomFormat":"CycloneDX","components":[%s]}\n' "$components" >"$dir/margince.cdx.json"
	echo "$dir"
}

# case_is <name> <pass|fail> <stdout|stderr> <text the stream must carry>
#         <sbom dir> <lockfiles> [<text the stream must not carry>]
case_is() {
	local name="$1" want="$2" stream="$3" carries="$4" sboms="$5" lockfiles="$6" lacks="${7:-}" status=0 text
	make -s --no-print-directory sbom-census SBOM_DIR="$sboms" SBOM_LOCKFILES="$lockfiles" \
		>"$work/stdout" 2>"$work/stderr" || status=$?
	text="$(cat "$work/$stream")"
	if [[ "$want" == pass && $status -ne 0 ]] || [[ "$want" == fail && $status -eq 0 ]]; then
		echo "FAIL: $name — exit $status, want $want"
	elif [[ "$text" != *"$carries"* ]]; then
		echo "FAIL: $name — $stream does not carry '$carries'"
	elif [[ -n "$lacks" && "$text" == *"$lacks"* ]]; then
		echo "FAIL: $name — $stream carries '$lacks', which it must not"
	else
		echo "ok: $name"
		return
	fi
	sed 's/^/    stdout: /' "$work/stdout"
	sed 's/^/    stderr: /' "$work/stderr"
	failures=$((failures + 1))
}

everything=('@toolchain/exe.linux-x64@1.0.0' 'toolchain@1.0.0' '@beta/gamma@2.0.0' 'alpha@1.0.0' 'alpha@2.0.0' 'delta@3.1.4')

case_is "a missing SBOM is refused" fail stdout \
	"FAIL: no SBOM found" "$work/none" "$lockfile"

case_is "an SBOM with every package of every document passes" pass stderr \
	"OK: the SBOM lists all 6 npm packages resolved by $lockfile" \
	"$(sbom_with "${everything[@]}")" "$lockfile"

# What a reader that stops after the first document writes.
case_is "an SBOM with only the first document is short" fail stderr \
	"FAIL: the SBOM lists 2 of the 6 npm packages resolved by $lockfile" \
	"$(sbom_with '@toolchain/exe.linux-x64@1.0.0' 'toolchain@1.0.0')" "$lockfile"

# The env document is part of the lockfile too, so losing it is as short as
# losing the workspace.
case_is "an SBOM without the first document is short" fail stderr \
	"FAIL: the SBOM lists 4 of the 6 npm packages resolved by $lockfile" \
	"$(sbom_with '@beta/gamma@2.0.0' 'alpha@1.0.0' 'alpha@2.0.0' 'delta@3.1.4')" "$lockfile"

case_is "the census names what grant never judged, and nothing else" fail stderr \
	"  delta@3.1.4" \
	"$(sbom_with '@toolchain/exe.linux-x64@1.0.0' 'toolchain@1.0.0' '@beta/gamma@2.0.0' 'alpha@1.0.0' 'alpha@2.0.0')" "$lockfile" \
	"alpha@"

# As many npm packages as the lockfile resolves, one of them another package:
# a count alone would pass it.
case_is "an SBOM that swaps a resolved package for another is short" fail stderr \
	"FAIL: the SBOM lists 5 of the 6 npm packages resolved by $lockfile" \
	"$(sbom_with '@toolchain/exe.linux-x64@1.0.0' 'toolchain@1.0.0' '@beta/gamma@2.0.0' 'alpha@1.0.0' 'alpha@2.0.0' 'epsilon@9.9.9')" "$lockfile"

# A census of names alone would take one version for the other.
case_is "one version of a package does not stand in for another" fail stderr \
	"  alpha@2.0.0" \
	"$(sbom_with '@toolchain/exe.linux-x64@1.0.0' 'toolchain@1.0.0' '@beta/gamma@2.0.0' 'alpha@1.0.0' 'delta@3.1.4')" "$lockfile"

second="$work/second-lock.yaml"
printf "lockfileVersion: '9.0'\n\npackages:\n\n  zeta@1.0.0:\n    resolution: {integrity: sha512-GGGG}\n" >"$second"
case_is "every lockfile counts" fail stderr \
	"FAIL: the SBOM lists 6 of the 7 npm packages resolved by $lockfile, $second" \
	"$(sbom_with "${everything[@]}")" "$lockfile $second"

# A layout change in one document blinds a scanner that still reads the other,
# and the count of the whole stream stays above zero.
moved="$work/moved-lock.yaml"
printf '%s\n%s\n' "$env_document" "$(workspace_document 9.0 resolutions)" >"$moved"
case_is "a document that resolves no package is refused" fail stderr \
	"FAIL: document 2 of $moved resolves no package" \
	"$(sbom_with "${everything[@]}")" "$moved"

newer="$work/newer-lock.yaml"
printf '%s\n%s\n' "$env_document" "$(workspace_document 10.0 packages)" >"$newer"
case_is "a lockfile version the census was not checked against is refused" fail stderr \
	"FAIL: document 2 of $newer has lockfileVersion 10.0" \
	"$(sbom_with "${everything[@]}")" "$newer"

: >"$work/empty-lock.yaml"
case_is "a lockfile that holds no document is refused" fail stderr \
	"FAIL: $work/empty-lock.yaml holds no YAML document" \
	"$(sbom_with "${everything[@]}")" "$work/empty-lock.yaml"

printf 'packages: [\n' >"$work/broken-lock.yaml"
case_is "a lockfile that does not parse is refused" fail stderr \
	"FAIL: could not read $work/broken-lock.yaml as YAML" \
	"$(sbom_with "${everything[@]}")" "$work/broken-lock.yaml"

if [[ $failures -ne 0 ]]; then
	echo "FAIL: test-sbom-census — $failures case(s) failed"
	exit 1
fi
echo "OK: sbom-census refuses an SBOM that lists fewer npm packages than the lockfiles resolve"
