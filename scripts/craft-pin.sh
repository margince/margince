#!/usr/bin/env bash
# craft-pin.sh — the pinned craftsmanship gate, resolved the same way everywhere.
#
# Run it to print the path to ONE known binary; source it for craft_bin(). It
# gates nothing itself.
#
# The gate is fetched and checksum-verified rather than taken from PATH, for the
# reason the secret scanner is (scripts/gitleaks-pin.sh): a craftsmanship verdict
# is a function of the rubric the binary carries, so "some craft" on a laptop and
# a pinned one in CI are not the same gate. The difference surfaces as a finding
# that appears only after you push — or worse, only in CI's absence. Pinning is
# what lets `make craft-static` promise the answer the pull request will get, and
# it means no engineer installs anything.
#
# The gate's source is not public; the binary is. That is deliberate, and it is
# what keeps this working where it has to: offline on a laptop, and in CI on a
# pull request from a fork, which carries no credentials at all. A gate that
# needed a token would silently stop covering outside contributions.
#
# Bumping the version: change CRAFT_VERSION and replace all four digests from the
# release's checksums.txt. All four, not just yours — the others are what CI and
# your colleagues run. Every release has its own digests even when the gate's
# behaviour is unchanged, because the build stamps the source revision it came
# from; never carry a digest forward.

CRAFT_VERSION="v1.0.0"

# sha256 of each released binary, from
# https://github.com/margince/craft-dist/releases/download/${CRAFT_VERSION}/checksums.txt
CRAFT_SHA256_darwin_arm64="f0caaf099cafa837311dfcde376229b10ed710e7b3751a4774b4e6c8564044f9"
CRAFT_SHA256_darwin_amd64="e5c35e551f9470b39d8270446af3bc46e9a587a4ec882dcc616813a444625503"
CRAFT_SHA256_linux_arm64="7b7abda53a7db9ffc0ed451905455fe1f541146b76ebec02ee626f31846c4344"
CRAFT_SHA256_linux_amd64="9dba08e7c058c83b8ec226adf5d61e8578cac011fe4d2da78dfbbae488cd5754"

# craft_platform — the release's name for this host, e.g. darwin_arm64.
craft_platform() {
	local os arch
	case "$(uname -s)" in
	Darwin) os=darwin ;;
	Linux) os=linux ;;
	*)
		echo "craft-pin: unsupported OS $(uname -s)" >&2
		return 1
		;;
	esac
	case "$(uname -m)" in
	arm64 | aarch64) arch=arm64 ;;
	x86_64 | amd64) arch=amd64 ;;
	*)
		echo "craft-pin: unsupported architecture $(uname -m)" >&2
		return 1
		;;
	esac
	printf '%s_%s' "$os" "$arch"
}

# craft_bin — print the path to the pinned binary, downloading it once into
# .tmp/ (gitignored) if it is not already there. Every later run is a cache hit.
craft_bin() {
	local root platform digest dest url staged
	root="$(git rev-parse --show-toplevel)"
	platform="$(craft_platform)" || return 1

	# Indirect expansion: pick this platform's digest out of the four above.
	local digest_var="CRAFT_SHA256_${platform}"
	digest="${!digest_var:-}"
	if [[ -z "$digest" ]]; then
		echo "craft-pin: no pinned digest for $platform" >&2
		return 1
	fi

	dest="$root/.tmp/craft/$CRAFT_VERSION/craft"
	if [[ -x "$dest" ]]; then
		printf '%s' "$dest"
		return 0
	fi

	mkdir -p "$(dirname "$dest")"
	url="https://github.com/margince/craft-dist/releases/download/${CRAFT_VERSION}/craft_${CRAFT_VERSION}_${platform}"
	staged="$(mktemp)"
	echo "craft-pin: fetching craft $CRAFT_VERSION ($platform)" >&2
	# --proto/--proto-redir '=https': the release URL redirects to a CDN, and a
	# redirect to plain http would fetch the gate over a tamperable channel. The
	# digest below would still catch a swap; the download refuses to leave TLS in
	# the first place.
	if ! curl -fsSL --proto '=https' --proto-redir '=https' -o "$staged" "$url"; then
		rm -f "$staged"
		echo "craft-pin: could not download $url" >&2
		return 1
	fi

	if ! printf '%s  %s\n' "$digest" "$staged" | shasum -a 256 -c - >/dev/null 2>&1; then
		rm -f "$staged"
		echo "craft-pin: checksum mismatch for craft $CRAFT_VERSION ($platform)." >&2
		echo "  The download did not match the digest pinned in scripts/craft-pin.sh." >&2
		echo "  Do not bypass this: it means the artifact changed, not that the pin is stale." >&2
		return 1
	fi

	# Staged then moved, so a killed run leaves no half-written binary that the
	# `-x` cache check above would hand to the next one as if it were the gate.
	chmod +x "$staged"
	mv "$staged" "$dest"
	printf '%s' "$dest"
}

# Executed rather than sourced: print the path. `$(scripts/craft-pin.sh)` is a
# command, which is how the Makefile and the pre-push hook call the gate.
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
	set -euo pipefail
	craft_bin
fi
