#!/usr/bin/env bash
# print-slow-statements.sh's own test, on log lines a Postgres 16 server wrote
# with the integration lane's settings: a slow plan and a statement cancelled
# by statement_timeout are printed whole, and nothing else in the log is.
#
# Usage: bash scripts/print-slow-statements.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PRINT="$SCRIPT_DIR/print-slow-statements.sh"

FAILURES=0
fail() { echo "FAIL: $*" >&2; FAILURES=$((FAILURES + 1)); }

# Backend 601 writes between the cancellation and its CONTEXT line, as a
# concurrent backend can; 599's error is not a timeout and stays out.
log="$(printf '%s\n' \
	'2026-11-26 05:59:12.301 UTC [578] LOG:  parameter "log_parameter_max_length_on_error" changed to "-1"' \
	'2026-11-26 05:59:13.542 UTC [595] LOG:  duration: 1202.386 ms  plan:' \
	$'\tQuery Text: SELECT pg_sleep(1.2), \'finished\' AS outcome' \
	$'\tResult  (cost=0.00..0.01 rows=1 width=36)' \
	'2026-11-26 05:59:14.072 UTC [597] ERROR:  canceling statement due to statement timeout' \
	'2026-11-26 05:59:14.073 UTC [601] LOG:  checkpoint starting: time' \
	"2026-11-26 05:59:14.072 UTC [597] CONTEXT:  unnamed portal with parameters: \$1 = '0.4', \$2 = 'it''s'" \
	'2026-11-26 05:59:14.072 UTC [597] STATEMENT:  SELECT g, pg_sleep($1::float8)' \
	$'\t  FROM generate_series(1, 3) AS g' \
	$'\t WHERE g::text <> $2 ' \
	'2026-11-26 05:59:14.090 UTC [597] LOG:  disconnection: session time: 0:00:00.020' \
	'2026-11-26 05:59:14.103 UTC [599] ERROR:  division by zero' \
	'2026-11-26 05:59:14.103 UTC [599] STATEMENT:  SELECT 1/0')"

out="$(bash "$PRINT" <<<"$log")"
for wanted in \
	"[595] LOG:  duration: 1202.386 ms  plan:" \
	"Result  (cost=0.00..0.01 rows=1 width=36)" \
	"[597] ERROR:  canceling statement due to statement timeout" \
	"\$1 = '0.4', \$2 = 'it''s'" \
	"[597] STATEMENT:  SELECT g, pg_sleep(\$1::float8)" \
	"FROM generate_series(1, 3) AS g" \
	"WHERE g::text <> \$2"; do
	grep -qF -- "$wanted" <<<"$out" || fail "\"$wanted\" is not printed: $out"
done
for unwanted in "checkpoint starting" "disconnection" "division by zero" "SELECT 1/0" "changed to"; do
	grep -qF -- "$unwanted" <<<"$out" && fail "\"$unwanted\" is printed, though it is neither slow nor cancelled: $out"
done

out="$(bash "$PRINT" <<<"$(grep -F '[599]' <<<"$log")")"
grep -qF "no statement ran past" <<<"$out" || fail "a log with no slow or cancelled statement does not say so: $out"

out="$(SLOW_STATEMENTS_MAX_LINES=2 bash "$PRINT" <<<"$log")"
[[ "$(wc -l <<<"$out")" -eq 3 ]] || fail "the cap of 2 lines printed more than 2 lines and the note: $out"
grep -qF "more lines not shown" <<<"$out" || fail "a capped report does not say lines were cut: $out"

if [[ "$FAILURES" -ne 0 ]]; then
	echo "print-slow-statements.test: $FAILURES failure(s)" >&2
	exit 1
fi
echo "OK: print-slow-statements prints slow plans and timed-out statements with their parameters, and nothing else"
