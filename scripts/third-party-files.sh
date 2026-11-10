#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
python3 "$repo_root/scripts/third_party_files.py" check "${1:-$repo_root}" "${2:-$repo_root/third-party-files.txt}"
