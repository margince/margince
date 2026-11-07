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
	"cmp"
	"context"
	"errors"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

// predictRow answers what the commit will do, and what the report says about
// the row: why it is refused, or the warning it lands with.
//
// Checks run in Ensure's order: create grant, then cells, then a contact's
// company link under the run's policy. A dropped link rides on the outcome.
func (w *csvWriters) predictRow(ctx context.Context, row migration.Row) (predictedOutcome, *migration.SkippedRow, error) {
	if refused := w.createRefusal(ctx, row); refused != nil {
		return predictUnwritable, refused, nil
	}
	typed, invalid, unknownOwner := w.landingCells(row)
	if invalid != nil {
		return predictUnwritable, invalid, nil
	}
	link, err := w.linkProblemOf(ctx, row)
	if err != nil {
		return predictCreate, nil, err
	}
	if link.skip {
		return predictUnwritable, link.problem, nil
	}
	outcome, refusal, err := w.predictLanding(ctx, row, link.into(typed))
	if refusal.Code != "" {
		why := migration.SkippedRow{ExternalID: row.ExternalID, Line: row.Line}.Because(refusal)
		return outcome, &why, err
	}
	if err == nil && w.object == migration.ObjectContact && outcome != predictUnwritable && outcome != predictCollidesSkipped && link.problem == nil {
		employer, previewErr := w.previewImportedEmployer(ctx, row, typed)
		if why, refused := employerRefusalWhy(previewErr); refused {
			problem := migration.SkippedRow{ExternalID: row.ExternalID, Line: row.Line, Kind: migration.ProblemBrokenLink}.Because(why)
			return predictUnwritable, &problem, nil
		}
		if previewErr != nil {
			return outcome, nil, previewErr
		}
		w.rememberPredictedEmployer(row.ExternalID, employer)
		if employer.Effect == contacts.ContactEmployerCreateEffect {
			problem := migration.SkippedRow{ExternalID: row.ExternalID, Line: row.Line}.Because(migration.Why(migration.ReasonEmployerWillCreate, "name", textFields(row.Fields)[csvEmployerName]))
			return outcome, cmp.Or(unknownOwner, &problem), nil
		}
	}
	return outcome, cmp.Or(link.problem, unknownOwner), err
}

// predictLanding is predictRow once the row's cells and links allow it to land.
func (w *csvWriters) predictLanding(ctx context.Context, row migration.Row, typed map[string]any) (predictedOutcome, migration.Reason, error) {
	// Mirrors Ensure, in the same order: a named record first, then the identity
	// map. The two must answer alike or an approval decides one thing and the
	// commit does another.
	target := w.targetIDOf(ctx, row)
	if target.named {
		if target.why.Code != "" {
			return predictUnwritable, target.why, nil
		}
		return w.reconcilePrediction(ctx, target.id, row, typed)
	}
	id, found, err := w.lookup(ctx, w.object, row.ExternalID)
	if errors.Is(err, errBoundRecordArchived) {
		return predictUnwritable, boundArchivedWhy, nil
	}
	if err != nil {
		return predictCreate, migration.Reason{}, err
	}
	if !found {
		return w.predictCreatePath(ctx, row, typed)
	}
	return w.reconcilePrediction(ctx, id, row, typed)
}

// predictCreatePath is what the commit will do with a row no previous run of
// this importer landed.
//
// Two kinds of question, in this order and the order matters. First, whether a
// key the store refuses is already spoken for: a contact's or lead's address, a
// company's domain, a lead's or company's LinkedIn URL. Each key is estate-wide,
// so the store refuses that row whoever holds it, and a preview promising a
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
func (w *csvWriters) predictCreatePath(ctx context.Context, row migration.Row, typed map[string]any) (predictedOutcome, migration.Reason, error) {
	switch w.object {
	case migration.ObjectDeal:
		return w.predictDealCreate(ctx, row, typed)
	case migration.ObjectActivity:
		return w.predictActivityCreate(ctx, row, typed)
	}
	claimed, err := w.contactEmailAlreadyHeld(ctx, row)
	if err != nil {
		return predictCreate, migration.Reason{}, err
	}
	if claimed {
		return predictUnwritable, contactEmailClaimedWhy, nil
	}
	takenLead, err := w.leadEmailAlreadyHeld(ctx, row)
	if err != nil {
		return predictCreate, migration.Reason{}, err
	}
	if takenLead {
		return predictUnwritable, naturalKeyTakenWhy, nil
	}
	takenDomain, err := w.companyDomainAlreadyHeld(ctx, row)
	if err != nil {
		return predictCreate, migration.Reason{}, err
	}
	if takenDomain {
		return predictUnwritable, domainClaimedWhy, nil
	}
	takenLinkedIn, err := w.linkedInAlreadyHeld(ctx, typed)
	if err != nil {
		return predictCreate, migration.Reason{}, err
	}
	if takenLinkedIn {
		return predictUnwritable, linkedInClaimedWhy, nil
	}
	contactHeld, err := w.leadRowContactHeld(ctx, row, typed)
	if err != nil {
		return predictCreate, migration.Reason{}, err
	}
	if contactHeld {
		return predictUnwritable, contactHoldsLeadWhy, nil
	}
	collides, err := w.collidesWithExisting(ctx, row)
	if err != nil {
		return predictCreate, migration.Reason{}, err
	}
	if !collides {
		return predictCreate, migration.Reason{}, nil
	}
	if w.onDuplicate == string(crmcontracts.ImportOnDuplicateSkip) {
		return predictCollidesSkipped, migration.Reason{}, nil
	}
	return predictCollides, migration.Reason{}, nil
}

// predictReconcile is what reconcile would do to this record, without writing.
//
// Shared by both ways a row reaches an existing record — the identity map, and
// an `id` column naming it — so the preview cannot answer one thing for a row
// the file identified and another for a row the importer remembered.
func (w *csvWriters) reconcilePrediction(
	ctx context.Context, id ids.UUID, row migration.Row, typed map[string]any,
) (predictedOutcome, migration.Reason, error) {
	switch w.object {
	case migration.ObjectDeal:
		return w.predictDealUpdate(ctx, id, row, typed)
	case migration.ObjectActivity:
		return w.predictActivityUpdate(ctx, id, row, typed)
	}
	current, err := w.read(ctx, id)
	if err != nil {
		return predictCreate, migration.Reason{}, err
	}
	changed, err := changedFields(current, textFields(row.Fields), typed)
	if err != nil {
		return predictCreate, migration.Reason{}, err
	}
	if len(changed) == 0 {
		return predictUnchanged, migration.Reason{}, nil
	}
	if !auth.Allows(ctx, w.object, principal.ActionUpdate) {
		return predictUnwritable, cannotEditWhy(w.object), nil
	}
	return predictUpdate, migration.Reason{}, nil
}

// collidesWithExisting asks whether the CRM already holds the company this row
// names, through the SAME ladder the create path runs (PO-F-2). It reads and
// writes nothing.
//
