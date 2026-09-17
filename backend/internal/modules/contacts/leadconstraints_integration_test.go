// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// What the lead TABLE says about itself, as opposed to what its one writer
// happens to do.
//
// Seven rules were true of every row and enforced only by the Go function that
// writes them, which means they held exactly as long as that function stayed
// the only writer. These prove the database now refuses each one — through raw
// SQL on purpose, because a statement the store would never emit is the case a
// constraint exists for. Driving the store instead would only re-confirm that
// the store is careful.
//
// The cost of the first two is a disagreement rather than a crash: a lead at
// `status = 'promoted'` with no `promoted_contact_id` makes "how many leads did
// we promote" and "show me the contact this lead became" answer about the same
// row and contradict each other.

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// constraintOf names which rule refused a write, so a case cannot pass by
// being caught by a different one than it is about.
func constraintOf(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return fmt.Sprintf("not a postgres error: %v", err)
	}
	return pgErr.ConstraintName
}

func TestTheLeadTableRefusesWhatItsWriterNeverWrites(t *testing.T) {
	e := setupPromoteConsent(t)
	created := time.Now().UTC().Add(-2 * time.Hour)

	for _, refused := range []struct {
		name       string
		constraint string
		set        string
	}{
		{
			name:       "promoted with no contact to show for it",
			constraint: "lead_promoted_names_its_contact",
			set:        `status = 'promoted', archived_at = now()`,
		},
		{
			name:       "promoted but not archived by the promotion that did it",
			constraint: "lead_promoted_names_its_contact",
			set:        `status = 'promoted', promoted_contact_id = NULL, archived_at = NULL`,
		},
		{
			name:       "promoted without the moment it happened",
			constraint: "lead_promoted_names_its_contact",
			set:        `status = 'promoted', archived_at = now(), promoted_at = NULL`,
		},
		{
			name:       "a score off the 0-100 scale its own history refuses",
			constraint: "lead_score_range",
			set:        `score = 900`,
		},
		{
			name:       "a negative score",
			constraint: "lead_score_range",
			set:        `score = -5`,
		},
		{
			name:       "a computed score off the same scale",
			constraint: "lead_score_computed_range",
			set:        `score_computed = 101`,
		},
		{
			name:       "an override with nothing left to override",
			constraint: "lead_override_retains_the_computed_score",
			set:        `score_override_reason = 'the founder knows them', score_computed = NULL`,
		},
		{
			name:       "answered before it arrived",
			constraint: "lead_first_response_follows_creation",
			set:        `first_response_at = created_at - interval '1 second'`,
		},
		{
			name:       "breached before it arrived",
			constraint: "lead_sla_breach_follows_creation",
			set:        `sla_breached_at = created_at - interval '1 second'`,
		},
		{
			name:       "a hand nobody has",
			constraint: "lead_status_set_by_check",
			set:        `status_set_by = 'agent'`,
		},
	} {
		t.Run(refused.name, func(t *testing.T) {
			lead := e.seedLeadCreatedAt(t, fmt.Sprintf("refused-%s@example.test", ids.NewV7()), created)
			_, err := e.owner.Exec(context.Background(),
				fmt.Sprintf(`UPDATE lead SET %s WHERE id = $1`, refused.set), lead.UUID)
			if err == nil {
				t.Fatalf("the table accepted %q, so %s holds nothing", refused.set, refused.constraint)
			}
			if got := constraintOf(err); got != refused.constraint {
				t.Errorf("refused by %q, want %q — the row is wrong for the reason this case is about, "+
					"and a different constraint catching it means this one was never exercised",
					got, refused.constraint)
			}
		})
	}
}

// The admitted shapes, so what the constraints refuse is the violation rather
// than the column. A gate that refuses everything reads as green here too.
func TestTheLeadTableStillAdmitsWhatItsWriterWrites(t *testing.T) {
	e := setupPromoteConsent(t)
	created := time.Now().UTC().Add(-2 * time.Hour)
	lead := e.seedLeadCreatedAt(t, "admitted@example.test", created)

	// A lead nobody has touched: no hand on the ladder, no scores, no clocks.
	// The NULL branch of the status_set_by rule, which used to be admitted by
	// three-valued logic rather than by intent.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE lead SET status_set_by = NULL, score = 0, score_computed = NULL,
		        first_response_at = NULL, sla_breached_at = NULL
		  WHERE id = $1`, lead.UUID); err != nil {
		t.Fatalf("an untouched lead was refused: %v", err)
	}
	// And a fully worked one: scored by the engine, overridden by a human with
	// the machine value retained, answered and breached after it arrived.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE lead SET status_set_by = 'human', score = 100, score_computed = 72,
		        score_override_reason = 'the founder knows them',
		        first_response_at = created_at + interval '1 minute',
		        sla_breached_at = created_at + interval '1 hour'
		  WHERE id = $1`, lead.UUID); err != nil {
		t.Fatalf("a worked lead was refused: %v", err)
	}
}
