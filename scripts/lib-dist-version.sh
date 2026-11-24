#!/usr/bin/env bash
# The dist release version grammar, shared by every script that checks one. Source this; don't execute it.

# Plain x.y.z: no prefix, no suffix, and no part with a leading zero.
# shellcheck disable=SC2034 # read by the scripts that source this file
DIST_VERSION_RE='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
