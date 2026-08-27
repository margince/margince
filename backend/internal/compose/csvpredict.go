// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The dry run's half: what a commit WOULD do with each row, decided the same
// way Ensure decides it.
//
// Split from csvwriters.go on the concept. These two halves must agree — a
// preview that promises something the commit does differently is the defect this
// whole pipeline exists to prevent — and they agree by calling the same
// helpers, not by living in the same file.

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// predictedOutcome is what a commit would do with one row, decided exactly the
// way Ensure decides it — same lookup, same comparison — so the dry run cannot
// promise something the commit then does differently.
type predictedOutcome int

const (
	predictCreate predictedOutcome = iota
	predictUpdate
	predictUnchanged
	// predictUnwritable is a row the commit will refuse. It is counted apart
	// from the three outcomes above because a dry run that folds it into
	// `created` promises work that cannot happen.
	predictUnwritable
	// predictCollides is a row naming a company the CRM ALREADY holds, found
	// by the same PO-F-2 ladder the create path runs. The commit still lands
	// it — and files a review pair — so this is a disclosure, not a refusal:
	// the preview's job is to say a duplicate is coming while a human can
	// still fix the file.
	predictCollides
	// predictCollidesSkipped is a duplicate this run will NOT land, because it
	// asked for on_duplicate: skip. Separate from predictCollides so the
	// preview reports the same outcome the commit will produce.
	predictCollidesSkipped
)

// Predict answers what Ensure would do, without writing.
//
// The unwritable check comes FIRST, before the identity lookup: a row the
// store will refuse is refused whether it would have created or updated, and
// discovering that only on the create branch would let the same bad value pass
// silently on a re-import.
func (w *csvWriters) Predict(ctx context.Context, row migration.Row) (predictedOutcome, error) {
	outcome, _, err := w.predictRow(ctx, row)
	return outcome, err
}

// predict answers what the commit will do AND, when it will refuse, the sentence
// the report shows for it.
//
// The reason travels with the outcome rather than being recomputed by the
// caller. It used to be recomputed — from unwritableReason, which knows about
// the size_band vocabulary and nothing else — so a refusal from any other source
// reached the report as a skip with an EMPTY reason, and the person reading it
// was told a row would not land without being told why.
func (w *csvWriters) predictRow(ctx context.Context, row migration.Row) (predictedOutcome, string, error) {
	if reason := unwritableReason(w.object, textFields(row.Fields)); reason != "" {
		return predictUnwritable, reason, nil
	}
	// Mirrors Ensure, in the same order: a named record first, then the identity
	// map. The two must answer alike or an approval decides one thing and the
	// commit does another.
	target := w.targetIDOf(ctx, row)
	if target.named {
		if target.reason != "" {
			return predictUnwritable, target.reason, nil
		}
		return w.reconcilePrediction(ctx, target.id, row)
	}
	id, found, err := w.lookup(ctx, w.object, row.ExternalID)
	if err != nil {
		return predictCreate, "", err
	}
	if !found {
		return w.predictCreatePath(ctx, row)
	}
	return w.reconcilePrediction(ctx, id, row)
}

// predictCreatePath is what the commit will do with a row no previous run of
// this importer landed.
//
// Two questions, in this order and the order matters. First, whether a person's
// address is already spoken for: uq_person_email_dedupe is estate-wide, so the
// store refuses that row whoever holds the address, and a preview promising a
// create would simply be wrong. It runs BEFORE the collision check and is not a
// disclosure decision — the outcome is identical for an incumbent the caller can
// see and one they cannot, which is what stops the commit's answer being an
// existence oracle.
//
// Second, whether the estate holds a record this row would duplicate. The CRM
// may hold it from mail, by hand or from a seed, none of which the identity map
// can see, so the create gets the same dedupe read the create path performs.
// That one IS disclosure-filtered, whichever mode asked: passing the mode
// through reported invisible companies as duplicates on a `skip` run, one CSV
// row at a time.
func (w *csvWriters) predictCreatePath(ctx context.Context, row migration.Row) (predictedOutcome, string, error) {
	claimed, err := w.personEmailAlreadyHeld(ctx, row)
	if err != nil {
		return predictCreate, "", err
	}
	if claimed {
		return predictUnwritable, personEmailClaimedReason, nil
	}
	takenDomain, err := w.orgDomainAlreadyHeld(ctx, row)
	if err != nil {
		return predictCreate, "", err
	}
	if takenDomain {
		return predictUnwritable, domainClaimedReason, nil
	}
	collides, err := w.collidesWithExisting(ctx, row)
	if err != nil {
		return predictCreate, "", err
	}
	if !collides {
		return predictCreate, "", nil
	}
	if w.onDuplicate == string(crmcontracts.Skip) {
		return predictCollidesSkipped, "", nil
	}
	return predictCollides, "", nil
}

// predictReconcile is what reconcile would do to this record, without writing.
//
// Shared by both ways a row reaches an existing record — the identity map, and
// an `id` column naming it — so the preview cannot answer one thing for a row
// the file identified and another for a row the importer remembered.
func (w *csvWriters) reconcilePrediction(
	ctx context.Context, id ids.UUID, row migration.Row,
) (predictedOutcome, string, error) {
	current, err := w.read(ctx, id)
	if err != nil {
		return predictCreate, "", err
	}
	changed, err := changedFields(current, textFields(row.Fields))
	if err != nil {
		return predictCreate, "", err
	}
	if len(changed) == 0 {
		return predictUnchanged, "", nil
	}
	return predictUpdate, "", nil
}

// collidesWithExisting asks whether the CRM already holds the company this row
// names, through the SAME ladder the create path runs (PO-F-2). It reads and
// writes nothing.
//
