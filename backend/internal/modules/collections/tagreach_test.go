// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// `carried_by` on a search hit is this counter, so what it refuses is what a
// searcher is not told. The count is per-caller by construction — it totals
// tagUsage, whose per-type queries each carry that type's row-scope clause —
// and the door in front of it is the vocabulary read: a caller who may not
// read tags learns nothing about them, including how many records carry one.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A count is a fact about the vocabulary, so it asks the same grant reading the
// vocabulary asks. The refusal lands before any query, which is why a nil
// transaction is enough to prove it: reaching the database would panic.
func TestCountingATagsReachNeedsTheVocabularyRead(t *testing.T) {
	t.Parallel()
	ctx := taggerWith(map[string]principal.ObjectGrant{
		"person":       {Read: true},
		"organization": {Read: true},
		"deal":         {Read: true},
	})

	_, err := CountTagReachBatch(ctx, nil, []ids.TagID{ids.New[ids.TagKind]()})

	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a seat without tag.read counted a tag's reach: %v", err)
	}
}

// The disclosure case Codex named, and the reason the object grant is asked
// per counted type: ScopeClauseFor renders a ROW predicate and is not a gate,
// so a seat holding tag.read and no person.read would be counted the people it
// may not list — and a count of rows a caller cannot open tells them those rows
// exist. The refusal lands before any query here too, so a nil transaction is
// the proof: reaching the database would panic instead.
func TestATypeTheCallerMayNotReadIsNotCounted(t *testing.T) {
	t.Parallel()
	ctx := taggerWith(map[string]principal.ObjectGrant{
		"tag": {Read: true},
	})

	if _, err := countVisibleTaggedBatch(ctx, nil, []ids.TagID{ids.New[ids.TagKind]()}, "person"); !errors.Is(
		err, apperrors.ErrPermissionDenied,
	) {
		t.Fatalf("counted a type the seat may not read: %v", err)
	}
}

// The admit case, and it is not decoration: without it a counter that refused
// EVERY caller would pass the refusal above and report nothing to anybody.
// Holding the grant must get PAST the door — it then fails on the nil
// transaction, which is the proof it got that far.
func TestTheVocabularyReadGetsPastTheDoor(t *testing.T) {
	t.Parallel()
	ctx := taggerWith(map[string]principal.ObjectGrant{
		"tag":          {Read: true},
		"person":       {Read: true},
		"organization": {Read: true},
		"deal":         {Read: true},
	})

	defer func() {
		if recover() == nil {
			t.Fatal("expected the nil transaction to panic once the door admitted")
		}
	}()
	if _, err := CountTagReachBatch(ctx, nil, []ids.TagID{ids.New[ids.TagKind]()}); err != nil {
		t.Fatalf("the door refused a seat holding tag.read: %v", err)
	}
}
