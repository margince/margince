#!/usr/bin/env bash
# Prints, from a Postgres server log on stdin, the statements that made an
# integration shard slow: each plan auto_explain logged, and each statement
# that statement_timeout cancelled (SQLSTATE 57014).
#
# auto_explain logs a plan only when a statement finishes, so a cancelled
# statement never has one. Its ERROR entry is followed by a CONTEXT line with
# the bound parameters (log_parameter_max_length_on_error) and a STATEMENT line
# with the SQL, which together are enough to run EXPLAIN on it again.
#
# Usage: docker logs "$container" 2>&1 | bash scripts/print-slow-statements.sh
# SLOW_STATEMENTS_MAX_LINES (default 2000) caps the output.
set -euo pipefail

awk -v max_lines="${SLOW_STATEMENTS_MAX_LINES:-2000}" '
# Postgres writes one entry per message, under a prefix ending in "[pid] LEVEL:  ";
# the lines that carry on a multi-line entry start with a tab.
function emit(line) {
	shown++
	if (shown <= max_lines) print line
}
function start_entry() {
	if (entries++ > 0) emit("")
}
/^\t/ {
	if (printing) emit($0)
	next
}
{
	printing = 0
	if (!match($0, /\[[0-9]+\] [A-Z]+:  /)) next
	head = substr($0, RSTART, RLENGTH)
	message = substr($0, RSTART + RLENGTH)
	pid = head
	sub(/\].*/, "", pid)
	sub(/^\[/, "", pid)
	level = head
	sub(/^\[[0-9]+\] /, "", level)
	sub(/:  $/, "", level)

	if (level == "LOG" && message ~ /^duration: .* plan:/) {
		start_entry()
		printing = 1
	} else if (level == "ERROR" && message ~ /^canceling statement due to statement timeout/) {
		start_entry()
		printing = 1
		cancelled[pid] = 1
	} else if ((pid in cancelled) && level ~ /^(DETAIL|HINT|CONTEXT|STATEMENT|QUERY|LOCATION)$/) {
		printing = 1
	} else {
		delete cancelled[pid]
	}
	if (printing) emit($0)
}
END {
	if (entries == 0) {
		print "no statement ran past the auto_explain threshold or hit the statement timeout"
	} else if (shown > max_lines) {
		print "... " (shown - max_lines) " more lines not shown (SLOW_STATEMENTS_MAX_LINES=" max_lines ")"
	}
}
'
