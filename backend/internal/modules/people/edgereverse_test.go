// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Reading an audited edge image back as the patch that restates it.
//
// The keys are relationshipFieldImage's and the decode is here for that reason:
// one writer, one reader. What the cases below hold is that a value the image
// carries reaches the patch and a value it does not carry does NOT — an image
// arriving as jsonb has no types of its own, and a silently dropped key would
// restore a state the entry never held.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnEdgeImageIsReadBackAsThePatchThatRestatesIt(t *testing.T) {
	t.Parallel()
	patch, err := relationshipPatchFromImage(map[string]any{
		relationshipKindField: "employment",
		relationshipRoleField: "cto",
		"is_current_primary":  true,
		"started_at":          "2024-03-01T00:00:00Z",
		"ended_at":            "2026-01-31T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("reading the image: %v", err)
	}
	if patch.Role == nil || *patch.Role != "cto" {
		t.Errorf("role = %v, want the value the image carries", patch.Role)
	}
	if patch.IsCurrentPrimary == nil || !*patch.IsCurrentPrimary {
		t.Errorf("is_current_primary = %v, want the flag the image carries", patch.IsCurrentPrimary)
	}
	want := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	if patch.StartedAt == nil || !patch.StartedAt.Equal(want) {
		t.Errorf("started_at = %v, want %v", patch.StartedAt, want)
	}
	if patch.EndedAt == nil {
		t.Error("ended_at was dropped; the patch would leave the date the entry changed")
	}
}

// A key the image does not carry is left UNSUPPLIED, which the patch reads as
// "leave it". The audited pair is already narrowed to what moved, so an image
// missing a key asserts this entry did not change it — and supplying a zero
// value would reverse more than the person asked to reverse.
func TestAKeyTheEdgeImageDoesNotCarryIsLeftUnsupplied(t *testing.T) {
	t.Parallel()
	patch, err := relationshipPatchFromImage(map[string]any{relationshipRoleField: "coo"})
	if err != nil {
		t.Fatalf("reading the image: %v", err)
	}
	if patch.IsCurrentPrimary != nil || patch.StartedAt != nil || patch.EndedAt != nil {
		t.Errorf("an absent key reached the patch: %+v", patch)
	}
}

// A value of the wrong shape is a FAULT, not a skipped field. Dropping it would
// write a patch that restores a state the entry never recorded, and report
// success for it.
func TestAnEdgeImageValueOfTheWrongShapeIsAFault(t *testing.T) {
	t.Parallel()
	for key, image := range map[string]map[string]any{
		"role":               {relationshipRoleField: 7},
		"is_current_primary": {"is_current_primary": "yes"},
		"started_at":         {"started_at": "the first of March"},
	} {
		if _, err := relationshipPatchFromImage(image); err == nil {
			t.Errorf("a malformed %s was read as a patch rather than refused", key)
		}
	}
}

// An edge names one anchor, and every kind's anchor is a column the row must
// hold. A row holding none is a shape no kind admits, and answering "writable"
// about a record that is not there would light a button on a write that cannot
// happen.
//
// The object comes back with the id rather than being handed in: the caller
// that names the anchor and the switch that reads it off the row are one
// answer now, so a caller cannot ask for one kind's anchor and be given
// another kind's column.
func TestAnEdgeWithNoAnchorIsAFaultRatherThanAnEmptyAnswer(t *testing.T) {
	t.Parallel()
	if _, _, err := anchorIDOf(relationshipRow{ID: ids.NewV7(), Kind: "employment"}); err == nil {
		t.Error("an employment with no person was given an anchor")
	}
	person := ids.From[ids.PersonKind](ids.NewV7())
	object, got, err := anchorIDOf(relationshipRow{Kind: "employment", PersonID: &person})
	if err != nil {
		t.Fatalf("anchoring an employment on its person: %v", err)
	}
	if object != anchorPerson {
		t.Errorf("anchor object = %q, want %q — an employment annotates its person", object, anchorPerson)
	}
	if got != person.UUID {
		t.Errorf("anchor = %s, want the person the employment names", got)
	}
}

// The inverse is chosen from what the reversed entry DID, and there are two.
// Anything else is a fault rather than a silent no-op: an unlink is refused by
// the caller before it reaches here, so an action arriving here is a bug.
func TestOnlyTwoEdgeActionsHaveAnInverseHere(t *testing.T) {
	t.Parallel()
	var store *Store
	if err := store.ReverseEdge(t.Context(), ReverseEdgeInput{
		EdgeID: ids.NewV7(), Action: "archive",
	}); err == nil {
		t.Error("an unlink was accepted here; putting one back is an un-archive")
	}
}
