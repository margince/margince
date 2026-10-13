// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// CallFilter narrows the call trace; a zero field does not narrow, and the set
// fields combine with AND. Outcome is one of the AiCallOutcome values the
// handler has already validated.
type CallFilter struct {
	Task     string
	From, To *time.Time
	Outcome  string
	Model    string
	Query    string
}

// callModelSQL is the model a reader sees for a call, the served identity or the
// configured binding when the provider reported none. The filter and the
// dropdown's option set both read it, so every option matches its own calls.
const callModelSQL = `COALESCE(NULLIF(c.served_model, ''), c.model_id)`

// callSearchSQL matches one ILIKE pattern against the codes a row shows. The
// task also matches with spaces, as the screen spells it in words.
const callSearchSQL = `(c.task ILIKE $%[1]d OR replace(c.task, '_', ' ') ILIKE $%[1]d
	OR c.provider ILIKE $%[1]d OR c.model_id ILIKE $%[1]d OR c.served_model ILIKE $%[1]d
	OR COALESCE(c.error_sentinel, '') ILIKE $%[1]d
	OR replace(COALESCE(c.error_sentinel, ''), '_', ' ') ILIKE $%[1]d)`

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// callQuery is one statement's argument list. Every placeholder is derived from
// it, and the outcome expression binds its sentinel sets here so the column a
// row reports and the predicate the filter applies are the same text.
type callQuery struct {
	args    []any
	outcome string
}

func newCallQuery() *callQuery {
	query := &callQuery{}
	query.outcome = callOutcomeSQL(query.arg(declinedSentinels), query.arg(answeredSentinels))
	return query
}

func (query *callQuery) arg(value any) int {
	query.args = append(query.args, value)
	return len(query.args)
}

// callOutcomeSQL buckets a terminal row `c` joined to its logical call `lc`.
// A declined call is a model that decided not to answer; any other sentinel the
// health read counts as a failure is failed. A decision model that fell back is
// the first attempt, so it is not counted as a retry.
func callOutcomeSQL(declined, answered int) string {
	return storekit.SQLf(`CASE
	WHEN NULLIF(c.error_sentinel, '') = ANY($%[1]d) THEN 'declined'
	WHEN NULLIF(c.error_sentinel, '') IS NOT NULL AND NOT (c.error_sentinel = ANY($%[2]d)) THEN 'failed'
	WHEN c.degraded OR (lc.decision_attempted AND c.kind <> 'decision')
	  OR c.attempt - CASE WHEN lc.decision_attempted THEN 1 ELSE 0 END > 1 THEN 'fallback'
	ELSE 'answered' END`, declined, answered)
}

// where is the filter's WHERE clause over the terminal rows.
func (f CallFilter) where(query *callQuery) string {
	clauses := []string{"c.is_terminal"}
	if f.Task != "" {
		clauses = append(clauses, storekit.SQLf("c.task = $%d", query.arg(f.Task)))
	}
	if f.From != nil {
		clauses = append(clauses, storekit.SQLf("c.occurred_at >= $%d", query.arg(*f.From)))
	}
	if f.To != nil {
		clauses = append(clauses, storekit.SQLf("c.occurred_at < $%d", query.arg(*f.To)))
	}
	if f.Outcome != "" {
		clauses = append(clauses, storekit.SQLf("(%s) = $%d", query.outcome, query.arg(f.Outcome)))
	}
	if f.Model != "" {
		clauses = append(clauses, storekit.SQLf("%s = $%d", callModelSQL, query.arg(f.Model)))
	}
	if needle := strings.TrimSpace(f.Query); needle != "" {
		pattern := "%" + likeEscaper.Replace(needle) + "%"
		clauses = append(clauses, storekit.SQLf(callSearchSQL, query.arg(pattern)))
	}
	return strings.Join(clauses, " AND ")
}
