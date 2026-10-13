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
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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
	outcome, refusal, err := w.predictLanding(ctx, row, typed)
	if refusal != "" {
		return outcome, &migration.SkippedRow{ExternalID: row.ExternalID, Line: row.Line, Reason: refusal}, err
	}
	return outcome, cmp.Or(link.problem, unknownOwner), err
}

// predictLanding is predictRow once the row's cells and links allow it to land.
func (w *csvWriters) predictLanding(ctx context.Context, row migration.Row, typed map[string]any) (predictedOutcome, string, error) {
	// Mirrors Ensure, in the same order: a named record first, then the identity
	// map. The two must answer alike or an approval decides one thing and the
	// commit does another.
	target := w.targetIDOf(ctx, row)
	if target.named {
		if target.reason != "" {
			return predictUnwritable, target.reason, nil
		}
		// Linkable even when the update is refused, as ensureRow binds it.
		w.links.markPending(w.object, row.ExternalID)
		return w.reconcilePrediction(ctx, target.id, row, typed)
	}
	id, found, err := w.lookup(ctx, w.object, row.ExternalID)
	if errors.Is(err, errBoundRecordArchived) {
		return predictUnwritable, boundArchivedReason, nil
	}
	if err != nil {
		return predictCreate, "", err
	}
	if !found {
		return w.predictCreatePath(ctx, row, typed)
	}
	return w.reconcilePrediction(ctx, id, row, typed)
}

// predictCreatePath is what the commit will do with a row no previous run of
// this importer landed.
//
// Two questions, in this order and the order matters. First, createPathRefusal:
// a row naming no record, or a unique key the estate already holds. The store
// refuses a held key whoever holds it, so this is not a disclosure decision.
// The outcome is the same whether or not the caller can see the incumbent,
// which stops the answer being an existence oracle.
//
// Second, whether the estate holds a record this row would duplicate. The CRM
// may hold it from mail, by hand or from a seed, none of which the identity map
// can see, so the create gets the same dedupe read the create path performs.
// That one IS disclosure-filtered, whichever mode asked: passing the mode
// through reported invisible companies as duplicates on a `skip` run, one CSV
// row at a time.
func (w *csvWriters) predictCreatePath(ctx context.Context, row migration.Row, typed map[string]any) (predictedOutcome, string, error) {
	refused, err := w.createPathRefusal(ctx, row, typed)
	if err != nil {
		return predictCreate, "", err
	}
	if refused != "" {
		return predictUnwritable, refused, nil
	}
	collides, err := w.collidesWithExisting(ctx, row)
	if err != nil {
		return predictCreate, "", err
	}
	if !collides {
		return predictCreate, "", nil
	}
	if w.onDuplicate == string(crmcontracts.ImportOnDuplicateSkip) {
		return predictCollidesSkipped, "", nil
	}
	return predictCollides, "", nil
}

// createPathRefusal is why a create is refused before the collision check, or
// "". It refuses a row naming no record, or a unique key the estate already
// holds. The preview and the commit both ask it first, so a refused row reads
// the same whoever holds the key.
func (w *csvWriters) createPathRefusal(ctx context.Context, row migration.Row, typed map[string]any) (string, error) {
	if reason := unnamedReason(w.object, textFields(row.Fields)); reason != "" {
		return reason, nil
	}
	claimed, err := w.contactEmailAlreadyHeld(ctx, row)
	if err != nil {
		return "", err
	}
	if claimed {
		return contactEmailClaimedReason, nil
	}
	takenDomain, err := w.companyDomainAlreadyHeld(ctx, row)
	if err != nil {
		return "", err
	}
	if takenDomain {
		return domainClaimedReason, nil
	}
	leadClaimed, err := w.leadEmailAlreadyHeld(ctx, row)
	if err != nil {
		return "", err
	}
	if leadClaimed {
		return leadEmailClaimedReason, nil
	}
	takenLinkedIn, err := w.linkedInAlreadyHeld(ctx, typed)
	if err != nil || !takenLinkedIn {
		return "", err
	}
	return linkedInClaimedReason, nil
}

// unnamedReason is why a create is refused for naming no record, or "". An
// update is not refused for it: an absent name leaves the stored one alone.
func unnamedReason(object string, fields map[string]string) string {
	switch {
	case object == migration.ObjectCompany && strings.TrimSpace(fields[fieldDisplayName]) == "":
		return "the mapped display_name is empty, so the row names no company"
	case object == migration.ObjectContact && contactFullName(fields) == "":
		return "the row carries neither a name nor an email, so it names no contact"
	}
	return ""
}

// predictReconcile is what reconcile would do to this record, without writing.
//
// Shared by both ways a row reaches an existing record — the identity map, and
// an `id` column naming it — so the preview cannot answer one thing for a row
// the file identified and another for a row the importer remembered.
func (w *csvWriters) reconcilePrediction(
	ctx context.Context, id ids.UUID, row migration.Row, typed map[string]any,
) (predictedOutcome, string, error) {
	current, err := w.read(ctx, id)
	if err != nil {
		return predictCreate, "", err
	}
	changed, err := changedFields(current, textFields(row.Fields), typed)
	if err != nil {
		return predictCreate, "", err
	}
	if len(changed) == 0 {
		return predictUnchanged, "", nil
	}
	if !auth.Allows(ctx, w.object, principal.ActionUpdate) {
		return predictUnwritable, cannotEditReason(w.object), nil
	}
	refused, err := w.updateKeyRefusal(ctx, id, changed, typed)
	if err != nil {
		return predictCreate, "", err
	}
	if refused != "" {
		return predictUnwritable, refused, nil
	}
	return predictUpdate, "", nil
}

// collidesWithExisting asks whether the CRM already holds the company this row
// names, through the SAME ladder the create path runs (PO-F-2). It reads and
// writes nothing.
//
