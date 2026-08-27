// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

// The overnight agent writing what it found onto the run it just read.
//
// WHAT THIS DELIBERATELY CANNOT DO. It takes no user id, no run id, no rank and
// no arbitrary entity reference. The run is resolved from the acting
// principal's own current local day, so a model cannot annotate a colleague's
// morning, an older run, or a run it never read. What it may write is prose,
// and only prose: the ranking stays the deterministic engine's, because a model
// that could reorder the queue could put a deal first by asserting it belongs
// there rather than by evidence.
//
// EVERY CITED EVIDENCE ID IS VERIFIED, never trusted. A model supplies uuids as
// text, and a uuid that parses is not a uuid that means anything — it may name
// a record from another rep's queue, another workspace's deal, or nothing at
// all. So a citation is checked against the evidence the run itself recorded
// for that item, and an id outside it refuses the whole annotation rather than
// being dropped: an annotation whose citations were quietly pruned reads to the
// rep as grounded when it is not.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MaxAnnotationRunes bounds one piece of annotation prose.
//
// It matches the CHECK constraint the migration puts on both columns, and it is
// checked HERE as well so an over-long annotation is a refusal the agent can
// read and shorten rather than a driver error surfacing as a failed run. Two
// spellings of one bound is the cost; a run dying on a constraint violation at
// 2am with no actionable message is what it buys.
const MaxAnnotationRunes = 600

// ItemAnnotation is one finding: what the night learned about one queued deal,
// and the evidence it is drawn from.
type ItemAnnotation struct {
	ItemID ids.UUID
	// Finding is the prose the rep reads beside the rank — why this is here,
	// what changed, what to do next.
	Finding string
	// CitedEvidence is what the finding claims to rest on. Every id is checked
	// against the evidence the RUN recorded for this item; an id outside that
	// set refuses the annotation.
	CitedEvidence []ids.UUID
}

// Annotation is one overnight pass's output for one run.
type Annotation struct {
	// Narrative is the sentence about the night as a whole. Empty is a real
	// answer — a quiet night honestly has no sentence — and is stored as NULL
	// with the stamp still written, so the screen can tell "nothing to say"
	// from "never ran".
	Narrative string
	Items     []ItemAnnotation
}

// AnnotateCurrentRun writes one overnight pass's findings onto the acting
// rep's own run for today.
//
// It is idempotent by replacement: a re-run overwrites, because a pass that ran
// twice has one answer, not two. The stamp moves with it.
func (e *BriefEngine) AnnotateCurrentRun(ctx context.Context, ann Annotation, now time.Time) error {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return err
	}
	userID, err := briefUser(ctx)
	if err != nil {
		return err
	}
	if err := ann.validate(); err != nil {
		return err
	}

	return database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		day, _, err := LocalDayAt(ctx, tx, now)
		if err != nil {
			return err
		}
		var runID ids.UUID
		var previousNarrative *string
		var previousStamp *time.Time
		// FOR UPDATE, because two passes reaching one rep's morning would
		// otherwise interleave their item writes and leave a run carrying half
		// of each night's answer.
		row := tx.QueryRow(ctx, `
			SELECT id, narrative, annotated_at FROM brief_run
			 WHERE user_id = $1 AND local_day = $2
			 FOR UPDATE`, userID, day)
		switch err := row.Scan(&runID, &previousNarrative, &previousStamp); {
		case errors.Is(err, pgx.ErrNoRows):
			// No run for this rep today: the pass has nothing to annotate. Not
			// found rather than a created run — this writes onto what the
			// deterministic engine assembled, and inventing a run here would
			// produce a brief with prose and no ranking behind it.
			return apperrors.ErrNotFound
		case err != nil:
			return err
		}

		// The pass REPLACES, so findings it did not restate are cleared first.
		// Leaving them would carry last night's explanation under tonight's
		// stamp: the rep reads a finding about a reply that has since been
		// answered, dated by a pass that never looked at that deal.
		if err := clearFindings(ctx, tx, runID); err != nil {
			return err
		}
		for _, item := range ann.Items {
			if err := e.annotateItem(ctx, tx, runID, item); err != nil {
				return err
			}
		}
		return writeRunNarrative(ctx, tx, runID, ann.Narrative, now,
			previousNarrative, previousStamp)
	})
}

// clearFindings wipes the run's previous findings so the pass that follows is
// the whole answer rather than a layer over an older one.
//
// It audits the clearing as one row rather than one per item: what happened is
// "a new pass started", and a dozen audit rows saying a finding became null
// would bury the pass that replaced them.
func clearFindings(ctx context.Context, tx pgx.Tx, runID ids.UUID) error {
	tag, err := tx.Exec(ctx,
		`UPDATE brief_item SET finding = NULL WHERE brief_run_id = $1 AND finding IS NOT NULL`,
		runID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// Nothing to clear: the first pass of the night writes no audit row for
		// an erasure that did not happen.
		return nil
	}
	_, err = storekit.Audit(ctx, tx, "update", "brief_run", runID,
		map[string]any{"findings_cleared": tag.RowsAffected()},
		map[string]any{"findings_cleared": 0})
	return err
}

// annotateItem writes one finding, having proved the item belongs to this run
// and every citation to that item.
func (e *BriefEngine) annotateItem(ctx context.Context, tx pgx.Tx, runID ids.UUID, ann ItemAnnotation) error {
	var dealID ids.UUID
	var evidence []ids.UUID
	var before *string
	row := tx.QueryRow(ctx, `
		SELECT deal_id, evidence_ids, finding FROM brief_item
		 WHERE id = $1 AND brief_run_id = $2
		 FOR UPDATE`, ann.ItemID, runID)
	switch err := row.Scan(&dealID, &evidence, &before); {
	case errors.Is(err, pgx.ErrNoRows):
		// The item is not in THIS run — another rep's, another day's, or
		// invented. Existence-hiding, like every row-scope miss.
		return apperrors.ErrNotFound
	case err != nil:
		return err
	}
	// The deal may have moved out of this rep's scope since the run was
	// assembled. A finding is a read-back as much as a write, so it carries the
	// same check the mark path carries.
	if err := auth.EnsureVisible(ctx, tx, "deal", dealID); err != nil {
		return err
	}
	if err := citationsWithin(ann, evidence); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE brief_item SET finding = $2 WHERE id = $1`, ann.ItemID, ann.Finding); err != nil {
		return err
	}
	_, err := storekit.Audit(ctx, tx, "update", "brief_item", ann.ItemID,
		map[string]any{"finding": before},
		map[string]any{"finding": ann.Finding})
	return err
}

// citationsWithin refuses a finding that is not grounded in what the run
// actually recorded for that item.
//
// A FINDING MUST CITE SOMETHING. An empty citation list is not "a claim with no
// sources" — it is the verification being skipped, and it is the easy path: a
// model that omits the field gets its prose onto the rep's screen unchecked,
// under the same agent tag a grounded finding carries. The agent's own goal
// says every claim is grounded in a record it read, so a finding citing nothing
// has not done what it was asked, and the rep cannot tell by reading it.
//
// The whole annotation fails rather than the citation being dropped. A finding
// whose unverifiable citations were silently pruned still reads to the rep as
// grounded — the prose still says "he wrote yesterday" — and the pruning is
// invisible to everyone including the agent that would need to correct it.
func citationsWithin(ann ItemAnnotation, evidence []ids.UUID) error {
	if len(ann.CitedEvidence) == 0 {
		return httperr.Validation("cited_evidence", "required",
			fmt.Sprintf("item %s carries a finding that cites nothing; "+
				"cite the evidence ids the brief gave you for it", ann.ItemID))
	}
	// Bounded: an item carries a handful of evidence rows, so a list longer
	// than the evidence itself is a model repeating itself rather than
	// grounding anything — and an unbounded one is a loop this code would walk
	// on its behalf.
	if len(ann.CitedEvidence) > len(evidence) {
		return httperr.Validation("cited_evidence", "too_many",
			fmt.Sprintf("item %s cites %d ids but the run recorded %d for it",
				ann.ItemID, len(ann.CitedEvidence), len(evidence)))
	}
	held := make(map[ids.UUID]bool, len(evidence))
	for _, id := range evidence {
		held[id] = true
	}
	for _, cited := range ann.CitedEvidence {
		if !held[cited] {
			return httperr.Validation("cited_evidence", "not_this_items_evidence",
				fmt.Sprintf("item %s cites %s, which is not evidence this run recorded for it",
					ann.ItemID, cited))
		}
	}
	return nil
}

// writeRunNarrative stores the run-level sentence and the stamp that says a
// pass happened at all.
func writeRunNarrative(
	ctx context.Context, tx pgx.Tx, runID ids.UUID, narrative string, now time.Time,
	before *string, beforeStamp *time.Time,
) error {
	stamp := now.UTC()
	// Empty prose is stored as NULL, not as "". The CHECK allows a stamp with
	// no sentence precisely so a pass that ran and found nothing worth saying
	// is distinguishable from one that never ran — collapsing them would make
	// an honest quiet morning look like a broken one.
	var stored *string
	if narrative != "" {
		stored = &narrative
	}
	if _, err := tx.Exec(ctx,
		`UPDATE brief_run SET narrative = $2, annotated_at = $3 WHERE id = $1`,
		runID, stored, stamp); err != nil {
		return err
	}
	_, err := storekit.Audit(ctx, tx, "update", "brief_run", runID,
		map[string]any{"narrative": before, "annotated_at": beforeStamp},
		map[string]any{"narrative": stored, "annotated_at": stamp})
	return err
}

// validate bounds what a model may write before any of it reaches a statement.
func (a Annotation) validate() error {
	if err := boundProse("narrative", a.Narrative); err != nil {
		return err
	}
	seen := make(map[ids.UUID]bool, len(a.Items))
	for _, item := range a.Items {
		if item.ItemID.IsZero() {
			return httperr.Validation("item_id", "required", "an annotation names no item")
		}
		// One answer per item. Two findings for one deal is a model
		// contradicting itself, and picking either would be this code choosing
		// which of them the rep reads.
		if seen[item.ItemID] {
			return httperr.Validation("item_id", "duplicate",
				fmt.Sprintf("item %s is annotated twice", item.ItemID))
		}
		seen[item.ItemID] = true
		if err := boundProse("finding", item.Finding); err != nil {
			return err
		}
	}
	return nil
}

// boundProse refuses text past the column's own ceiling, in runes rather than
// bytes: the constraint counts characters, and a German sentence full of
// umlauts would otherwise be refused by the database after passing here.
func boundProse(field, text string) error {
	if n := len([]rune(text)); n > MaxAnnotationRunes {
		return httperr.Validation(field, "too_long",
			fmt.Sprintf("%s is %d characters, over the %d allowed", field, n, MaxAnnotationRunes))
	}
	return nil
}
