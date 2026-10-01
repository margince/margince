#!/usr/bin/env bash
# testlog-exec.sh — a `go test -exec` wrapper that runs each test binary with
# Go's own test log switched on, into $TESTLOG_DIR: one <id>.log per package,
# beside an <id>.dir naming the directory it ran in. The log is the list of
# every file, directory and variable the test consulted, which is exactly what
# the test cache keys a replay on; backend/tools/check-test-inputs reads it.
set -euo pipefail
: "${TESTLOG_DIR:?set TESTLOG_DIR to the directory the logs go to}"
mkdir -p "$TESTLOG_DIR"
id=$(printf '%s' "$PWD" | cksum | cut -d' ' -f1)
printf '%s\n' "$PWD" > "$TESTLOG_DIR/$id.dir"
bin=$1
shift
exec "$bin" -test.testlogfile="$TESTLOG_DIR/$id.log" "$@"
