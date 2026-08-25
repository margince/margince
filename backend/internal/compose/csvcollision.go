// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Whether a row names a company THIS CALLER CAN SEE that the estate already
// holds.
//
// One question, deliberately — an earlier shape asked two, deciding on the
// estate and disclosing on what may be read, and every version of that leaked.
// Filtering the preview moved the answer into the finished report, which needs
// only the import_run grant. Making the report's wording opaque left the
// OUTCOME saying what the words would not: "your row was not created" answers
// "is this company in your CRM" about a colleague's owner-private capture,
// probed one row at a time.
//
// So an invisible incumbent is answered as no incumbent, and the row creates.
// The cost is a twin the dedupe review queue picks up like any other and a merge
// resolves. The cost of the alternative is a disclosure no merge undoes.
//
// The ladder itself still reads every organization, by design: it is the write
// path's collision check. What is filtered is what this caller is TOLD, never
// what the estate knows.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/modules/migration"
	"github.com/gradionhq/margince/backend/internal/modules/people"
	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// A match the caller MAY NOT SEE is answered as no match at all, so the row goes
// on to create.
//
// Skipping it instead is the intuitive choice and it leaks. Hiding which company
// was hit is not enough: the outcome still says one was, and "your row was not
// created" answers "is this company in your CRM" — for a colleague's
// owner-private capture, probed one CSV row at a time, against a finished report
// readable on the import_run grant alone.
//
// What creating costs is a twin of a record the caller cannot see, which the
// dedupe review queue picks up like any other and a merge resolves. What skipping
// costs is a disclosure no merge undoes.
//
// Not leads: a lead's identity is its email, which the store's own unique key
// already refuses, so there is no silent twin to warn about.
//
// A read-only transaction, and NOT DedupeOrganizationForCreate — that one takes
// a write lock to serialize concurrent creates, which a preview has no business
// holding. The answer can go stale between the preview and the commit; the
// create path runs the locking version itself and its answer is the one that
// decides.
func (w *csvWriters) collidesWithExisting(ctx context.Context, row migration.Row) (bool, error) {
	switch w.object {
	case migration.ObjectOrganization:
		return w.organizationCollides(ctx, row)
	case migration.ObjectPerson:
		return w.personCollides(ctx, row)
	default:
		return false, nil
	}
}

func (w *csvWriters) organizationCollides(ctx context.Context, row migration.Row) (bool, error) {
	fields := textFields(row.Fields)
	candidate := people.OrganizationCandidate{
		DisplayName: strings.TrimSpace(fields[fieldDisplayName]),
		LegalName:   strings.TrimSpace(fields["legal_name"]),
	}
	if candidate.DisplayName == "" && candidate.LegalName == "" {
		return false, nil
	}
	var visible bool
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		match, err := people.DedupeOrganization(ctx, tx, candidate)
		if err != nil || match.Decision == people.DecisionNoMatch {
			return err
		}
		// EVERY candidate at or above the ladder's threshold, not just its
		// winner.
		//
		// The winner is chosen by confidence with the lowest uuid breaking ties,
		// which has nothing to do with who may read it — so an invisible record
		// ranking first MASKS a visible one behind it. Asking only about the
		// winner then made the hidden record change what this caller is told: with
		// it present the row created and reported no duplicate, without it the
		// visible candidate won and the row was skipped. The hidden company was
		// inferable from the disposition either way, which is the whole leak
		// wearing a different hat.
		//
		// A collision this caller can see is a collision. One they cannot see is,
		// as far as they are told, not there — and Ranked carries the set that
		// question has to be asked of.
		//
		// The ladder itself still reads every organization, by design: it is the
		// write path's collision check. What is filtered is what this caller is
		// TOLD, never what the estate knows.
		// EVERY candidate is asked, with no early exit once one answers yes.
		//
		// Returning on the first visible match would make the number of queries
		// depend on where the visible candidate sits among the hidden ones — and
		// a caller who can time the call reads that ordering back. It is a
		// narrow channel and it is the same fact by another route: whether
		// records they cannot see are there.
		//
		// The loop is bounded by the ladder's own threshold, so the cost is the
		// candidate set and not the estate.
		for _, candidate := range candidatesOf(match) {
			seen, err := auth.VisibleTo(ctx, tx, "organization", candidate.UUID)
			if err != nil {
				return err
			}
			visible = visible || seen
		}
		return nil
	}); err != nil {
		return false, fmt.Errorf("import: checking %q against the companies already held: %w",
			candidate.DisplayName, err)
	}
	return visible, nil
}

// candidatesOf answers every organization the ladder matched, best first.
//
// Held by: TestEveryMatchedCandidateIsAskedAboutNotJustTheWinner
// (backend/internal/compose/csvcollisionvisibility_test.go)
//
// Ranked is the full set at or above the review threshold and is what a
// visibility question must be asked of. It is empty for an exact (domain)
// collision, which carries its answer in OrganizationID instead — so that one is
// added when Ranked has nothing, and never twice.
func candidatesOf(match people.OrganizationMatch) []ids.OrganizationID {
	if len(match.Ranked) == 0 {
		return []ids.OrganizationID{match.OrganizationID}
	}
	out := make([]ids.OrganizationID, 0, len(match.Ranked))
	for _, scored := range match.Ranked {
		out = append(out, scored.OrganizationID)
	}
	return out
}
