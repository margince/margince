// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A merge is a TWO-row act, and for a long time it had a ONE-row precondition.
//
// The routed id pins the word being retired. The word being folded INTO was
// named in the body and pinned by nothing, so it could be renamed between the
// moment a caller read it and the moment the merge ran, and the merge went
// ahead as though it had not been. The asymmetry is what made it easy to miss:
// renaming the SOURCE before the act is refused, and only the survivor was
// free.
//
// Against a real database rather than a stub, because what is under test is
// the check's placement inside the transaction that already locks both rows —
// a version read outside that lock would pass against a rename that lands
// before the merge's own statements.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// mergePinEnv is two live tags and the store that folds one into the other.
type mergePinEnv struct {
	*Env
	store  *collections.Store
	source ids.TagID
	target ids.TagID
	// targetVersion is what a caller would have read before deciding.
	targetVersion int64
}

func setupMergePin(t *testing.T) *mergePinEnv {
	t.Helper()
	e := Setup(t)
	store := collections.NewStore(e.DB())
	ctx := e.Admin()

	source, err := store.CreateTag(ctx, "SkewSource", nil, nil)
	if err != nil {
		t.Fatalf("coining the tag to retire: %v", err)
	}
	target, err := store.CreateTag(ctx, "SkewTarget", nil, nil)
	if err != nil {
		t.Fatalf("coining the tag to survive: %v", err)
	}
	return &mergePinEnv{
		Env: e, store: store,
		source: source.ID, target: target.ID, targetVersion: target.Version,
	}
}

// rename moves the surviving tag, exactly as an ordinary edit would between a
// caller's read and their merge.
func (e *mergePinEnv) rename(t *testing.T, to string) {
	t.Helper()
	name := to
	if _, err := e.store.UpdateTag(e.Admin(), e.target,
		collections.TagUpdate{Name: &name}, 0); err != nil {
		t.Fatalf("renaming the surviving tag: %v", err)
	}
}

// The reported case: the survivor is renamed after the caller read it, and the
// merge must refuse rather than fold into a word nobody chose.
func TestAMergeRefusesASurvivorRenamedSinceItWasRead(t *testing.T) {
	e := setupMergePin(t)

	e.rename(t, "CompletelyDifferentWord")

	_, err := e.store.MergeTags(e.Admin(), e.source, e.target, e.targetVersion)
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("merging into a renamed survivor answered %v, wanted version skew — "+
			"the caller decided to fold into \"SkewTarget\" and this folds into "+
			"\"CompletelyDifferentWord\" instead", err)
	}

	// And it refused before touching anything: the source is still live, so a
	// caller who re-reads and decides again has something to merge.
	if _, _, err := e.store.GetTag(e.Admin(), e.source); err != nil {
		t.Errorf("the refused merge retired the source anyway: %v", err)
	}
}

// The version a caller actually read still merges, so the pin refuses the
// changed survivor rather than every survivor.
func TestAMergeRunsWhenTheSurvivorIsTheOneThatWasRead(t *testing.T) {
	e := setupMergePin(t)

	if _, err := e.store.MergeTags(e.Admin(), e.source, e.target, e.targetVersion); err != nil {
		t.Fatalf("merging into the survivor as read: %v", err)
	}
}

// Zero is the unpinned merge — what every caller sent before the field existed,
// and what the agent door still sends. It must keep working, or this change is
// a break dressed as a fix.
func TestAnUnpinnedMergeStillRunsAfterARename(t *testing.T) {
	e := setupMergePin(t)

	e.rename(t, "CompletelyDifferentWord")

	if _, err := e.store.MergeTags(e.Admin(), e.source, e.target, 0); err != nil {
		t.Fatalf("an unpinned merge was refused: %v", err)
	}
}

// A merge that already ran cannot run again.
//
// Both rows are locked with IncludeArchived, which a merge REQUIRES — it has
// to archive the source — so an already-retired source reached the statements
// rather than being refused. Every one of them was a no-op: nothing to count,
// nothing to move, nothing left to archive. The write shape still committed an
// audit row and an event, so a replayed request minted a `tag.merged` per
// attempt over a word folded away long ago, and each one read as a real merge.
//
// Counted rather than asserted on the error alone: refusing while still
// writing the audit row would satisfy an error-only test and leave the
// duplicate events that make this worth fixing.
func TestAMergeOfAnAlreadyRetiredTagIsRefusedAndWritesNothing(t *testing.T) {
	e := setupMergePin(t)

	if _, err := e.store.MergeTags(e.Admin(), e.source, e.target, e.targetVersion); err != nil {
		t.Fatalf("the first merge: %v", err)
	}
	before := e.mergeAudits(t)

	_, err := e.store.MergeTags(e.Admin(), e.source, e.target, 0)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("merging a tag that was already folded away answered %v, wanted "+
			"not-found — an archived source stops being addressable rather than "+
			"becoming a different kind of error", err)
	}

	if after := e.mergeAudits(t); after != before {
		t.Errorf("the refused merge wrote %d audit rows, want none — a refusal "+
			"that still records the act tells an auditor a merge happened twice",
			after-before)
	}
}

// mergeAudits counts what the audit log says happened to this source.
func (e *mergePinEnv) mergeAudits(t *testing.T) int {
	t.Helper()
	return e.WsCount(t,
		`SELECT count(*) FROM audit_log WHERE entity_type = 'tag' AND entity_id = $1`,
		e.source.UUID)
}
