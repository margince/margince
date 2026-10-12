#!/usr/bin/env bash
# Prove `make sbom-census` fails on a short census: an SBOM that lists fewer of
# the npm packages pnpm-lock.yaml resolves than the lockfile holds.
#
# grant judges only what an SBOM lists, so a scanner that reads part of the
# lockfile still yields a valid, non-empty SBOM and a green license gate over
# the part it read. pnpm writes the lockfile as a stream of YAML documents, the
# package manager's own binaries first and the workspace after, and a reader
# that stops after the first one loses every package the web app uses. Nothing
# else in the tree fails when that happens, so each case plants a lockfile and
# an SBOM and asserts the verdict sentence, not only the exit status.
#
# The YAML reader is the real one, in the pinned Python image: reading every
# document is the behaviour under test, and a stub would prove nothing about
# it. So this needs Docker and network, which the license-gate job has.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0

# The shape pnpm writes: an env document with the package manager's own
# binaries, then the workspace, whose snapshots carry peer suffixes that are
# not packages of their own.
lockfile="$work/pnpm-lock.yaml"
cat >"$lockfile" <<'YAML'
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

---
lockfileVersion: '9.0'

settings:
  autoInstallPeers: true
  excludeLinksFromLockfile: false

importers:

  .: {}

  frontend:
    dependencies:
      alpha:
        specifier: 1.0.0
        version: 1.0.0
      delta:
        specifier: 3.1.4
        version: 3.1.4(alpha@1.0.0)
    devDependencies:
      '@beta/gamma':
        specifier: 2.0.0
        version: 2.0.0

packages:

  '@beta/gamma@2.0.0':
    resolution: {integrity: sha512-CCCC}

  alpha@1.0.0:
    resolution: {integrity: sha512-DDDD}

  delta@3.1.4:
    resolution: {integrity: sha512-EEEE}
    peerDependencies:
      alpha: ^1.0.0

snapshots:

  '@beta/gamma@2.0.0': {}

  alpha@1.0.0: {}

  delta@3.1.4(alpha@1.0.0):
    dependencies:
      alpha: 1.0.0
YAML

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

# case_is <name> <pass|fail> <substring the output must carry> <sbom dir> <lockfile>
case_is() {
	local name="$1" want="$2" carries="$3" sboms="$4" lock="$5" out status=0
	out="$(make -s --no-print-directory sbom-census SBOM_DIR="$sboms" SBOM_LOCKFILE="$lock" 2>&1)" || status=$?
	if [[ "$want" == pass && $status -ne 0 ]] || [[ "$want" == fail && $status -eq 0 ]]; then
		echo "FAIL: $name — exit $status, want $want"
		echo "$out" | sed 's/^/    /'
		failures=$((failures + 1))
		return
	fi
	if [[ "$out" != *"$carries"* ]]; then
		echo "FAIL: $name — output does not carry '$carries'"
		echo "$out" | sed 's/^/    /'
		failures=$((failures + 1))
		return
	fi
	echo "ok: $name"
}

everything=('@toolchain/exe.linux-x64@1.0.0' 'toolchain@1.0.0' '@beta/gamma@2.0.0' 'alpha@1.0.0' 'delta@3.1.4')

case_is "a missing SBOM is refused before anything is read" fail \
	"FAIL: no SBOM found" "$work/none" "$lockfile"

case_is "an SBOM with every package of every document passes" pass \
	"OK: the SBOM lists all 5 npm packages $lockfile resolves" \
	"$(sbom_with "${everything[@]}")" "$lockfile"

# The shape syft 1.50 wrote: the env document read, the workspace lost.
case_is "an SBOM with only the first document is short" fail \
	"FAIL: the SBOM lists 2 of the 5 npm packages $lockfile resolves" \
	"$(sbom_with '@toolchain/exe.linux-x64@1.0.0' 'toolchain@1.0.0')" "$lockfile"

case_is "the census names what grant never judged" fail \
	"delta@3.1.4" \
	"$(sbom_with '@toolchain/exe.linux-x64@1.0.0' 'toolchain@1.0.0' '@beta/gamma@2.0.0' 'alpha@1.0.0')" "$lockfile"

# As many npm packages as the lockfile resolves, one of them another package:
# a count alone would pass it.
case_is "an SBOM that swaps a resolved package for another is short" fail \
	"FAIL: the SBOM lists 4 of the 5 npm packages $lockfile resolves" \
	"$(sbom_with '@toolchain/exe.linux-x64@1.0.0' 'toolchain@1.0.0' '@beta/gamma@2.0.0' 'alpha@1.0.0' 'epsilon@9.9.9')" "$lockfile"

# The env document is part of the lockfile too, so losing it is as short as
# losing the workspace.
case_is "an SBOM without the first document is short" fail \
	"FAIL: the SBOM lists 3 of the 5 npm packages $lockfile resolves" \
	"$(sbom_with '@beta/gamma@2.0.0' 'alpha@1.0.0' 'delta@3.1.4')" "$lockfile"

# An empty read is what a lockfile layout the reader no longer understands
# looks like, and against it every SBOM is complete.
cat >"$work/empty-lock.yaml" <<'YAML'
lockfileVersion: '9.0'

importers:

  .: {}
YAML
case_is "a lockfile that resolves nothing is refused" fail \
	"resolves no package" \
	"$(sbom_with "${everything[@]}")" "$work/empty-lock.yaml"

printf 'packages: [\n' >"$work/broken-lock.yaml"
case_is "a lockfile the reader cannot parse is refused" fail \
	"FAIL: could not read $work/broken-lock.yaml" \
	"$(sbom_with "${everything[@]}")" "$work/broken-lock.yaml"

if [[ $failures -ne 0 ]]; then
	echo "FAIL: test-sbom-census — $failures case(s) failed"
	exit 1
fi
echo "OK: sbom-census refuses an SBOM that lists fewer npm packages than the lockfile resolves"
