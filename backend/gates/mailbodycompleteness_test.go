// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// TestAutomaticWorkReadsOnlyMailReadInFull holds the clauses that keep
// automatic work current to provenance.MailBodyCompleteSQL. The task census
// sends every open-task reader through storekit.CurrentTaskClause, and the
// request writer composes storekit.AutomaticRequestIntentClause. Holding the
// clauses holds the readers and writers behind them. The source-mail arm keeps
// metadata-only work current, so it must not read the body's completeness.
func TestAutomaticWorkReadsOnlyMailReadInFull(t *testing.T) {
	t.Parallel()
	arg := func(any) int { return 1 }
	asOf := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		clause  string
		aliases []string
	}{
		"AutomaticRequestIntentClause": {storekit.AutomaticRequestIntentClause("source"), []string{"source"}},
		"CurrentCommitmentClause":      {provenance.CurrentCommitmentClause("a.source_activity_id", "a.source_id", "a.due_at", asOf, arg), []string{"evidence", "origin"}},
		"CurrentTaskClause":            {storekit.CurrentTaskClause("a", asOf, "'{}'::jsonb", arg), []string{"request_source", "evidence", "origin"}},
	} {
		for _, alias := range tc.aliases {
			if !strings.Contains(tc.clause, provenance.MailBodyCompleteSQL(alias)) {
				t.Errorf("%s does not require %s to have been read in full", name, alias)
			}
		}
	}
	if source := storekit.CurrentSourceMailClause("a.source_activity_id", asOf, arg); strings.Contains(source, "body_completeness") {
		t.Error("CurrentSourceMailClause reads body completeness; it keeps metadata-only work current and must not")
	}
}
