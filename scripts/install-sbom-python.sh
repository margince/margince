#!/usr/bin/env bash
set -euo pipefail

requirements="$1"
for attempt in 1 2 3; do
	if pip install --require-hashes --quiet --no-cache-dir --disable-pip-version-check \
		--root-user-action=ignore -r "$requirements"; then
		exit 0
	fi
	echo "pinned Python tooling: installation failed on attempt $attempt of 3" >&2
done
echo "FAIL: could not install hash-pinned Python tooling" >&2
exit 1
