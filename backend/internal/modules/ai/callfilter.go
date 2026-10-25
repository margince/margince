// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// CallListFilter narrows the call trace; a zero field does not narrow, and the
// set fields combine with `AND`. Outcome is one of the AiCallOutcome values the
// handler has already validated.
type CallListFilter struct {
	Task, Provider, ServedProvider, Tier string
	From, To                             *time.Time
	Outcome                              string
	Model, ServedModel                   string
	Query                                string
}

// callModelSQL is the model a reader sees for a call, the served identity or the
// configured binding when the provider reported none. The dropdown's option set
// reads it. The model filter matches it or the configured id, so an option
// matches its own calls and a figures link by configured model still lands.
const callModelSQL = `COALESCE(NULLIF(c.served_model, ''), c.model_id)`

// callSearchSQL matches one ILIKE pattern against the codes a row shows. The
// task also matches with spaces, as the screen spells it in words.
const callSearchSQL = `(c.task ILIKE $%[1]d OR replace(c.task, '_', ' ') ILIKE $%[1]d
	OR c.provider ILIKE $%[1]d OR c.model_id ILIKE $%[1]d OR c.served_model ILIKE $%[1]d
	OR COALESCE(c.error_sentinel, '') ILIKE $%[1]d
	OR replace(COALESCE(c.error_sentinel, ''), '_', ' ') ILIKE $%[1]d)`

// declinedSentinels are outcomes, not failures: the model was reached and
// decided not to answer.
var declinedSentinels = []string{sentinelOutputWithheld, sentinelRequestRejected}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// callQuery is one statement's argument list. Every placeholder is derived from
// it. The outcome expression binds its sentinel sets here, so the column a
// row reports and the predicate the filter applies are the same text.
type callQuery struct {
	args    []any
	outcome string
}

func newCallQuery() *callQuery {
	query := &callQuery{}
	query.outcome = callOutcomeSQL(query.arg(declinedSentinels), query.arg(servedSentinels))
	return query
}

//craft:ignore naked-any one list holds string slices and strings, each bound as a pgx argument
func (query *callQuery) arg(value any) int {
	query.args = append(query.args, value)
	return len(query.args)
}

// callOutcomeSQL buckets a terminal row `c` joined to its logical call `lc`.
// A declined call is a model that decided not to answer; any other sentinel
// whose answer never reached the caller is failed. A decision model that fell
// back is the first attempt, so it is not counted as a retry.
func callOutcomeSQL(declined, served int) string {
	return storekit.SQLf(`CASE
	WHEN NULLIF(c.error_sentinel, '') = ANY($%[1]d) THEN 'declined'
	WHEN NULLIF(c.error_sentinel, '') IS NOT NULL AND NOT (c.error_sentinel = ANY($%[2]d)) THEN 'failed'
	WHEN c.degraded OR (lc.decision_attempted AND c.kind <> 'decision')
	  OR c.attempt - CASE WHEN lc.decision_attempted THEN 1 ELSE 0 END > 1 THEN 'fallback'
	ELSE 'answered' END`, declined, served)
}

// where is the filter's `WHERE` clause over the terminal rows.
func (f CallListFilter) where(query *callQuery) string {
	clauses := []string{"c.is_terminal"}
	for _, equal := range []struct{ column, value string }{
		{"c.task", f.Task},
		{"c.provider", f.Provider},
		{"c.served_provider", f.ServedProvider},
		{"c.tier", f.Tier},
	} {
		if equal.value != "" {
			clauses = append(clauses, storekit.SQLf("%s = $%d", equal.column, query.arg(equal.value)))
		}
	}
	if f.From != nil {
		clauses = append(clauses, storekit.SQLf("c.occurred_at >= $%d", query.arg(*f.From)))
	}
	if f.To != nil {
		clauses = append(clauses, storekit.SQLf("c.occurred_at < $%d -- occurred_at raw: an AI call's own time, not an activity's\n", query.arg(*f.To)))
	}
	if f.Outcome != "" {
		clauses = append(clauses, storekit.SQLf("(%s) = $%d", query.outcome, query.arg(f.Outcome)))
	}
	if f.Model != "" {
		clauses = append(clauses, storekit.SQLf("(c.model_id = $%[2]d OR %[1]s = $%[2]d)", callModelSQL, query.arg(f.Model)))
	}
	if f.ServedModel != "" {
		clauses = append(clauses, storekit.SQLf("%s = $%d", answeredModelSQL("c"), query.arg(f.ServedModel)))
	}
	if needle := strings.TrimSpace(f.Query); needle != "" {
		pattern := "%" + likeEscaper.Replace(needle) + "%"
		clauses = append(clauses, storekit.SQLf(callSearchSQL, query.arg(pattern)))
	}
	return strings.Join(clauses, " AND ")
}
