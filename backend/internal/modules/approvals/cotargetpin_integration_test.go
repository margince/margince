// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// The SECOND row an approval's meaning can rest on.
//
// A tag merge retires one word into another, and the card a human reads names
// both. The retired side has been pinned since pins existed; the survivor was
// pinned by nothing, so it could be renamed while the card was pending and the
// merge still ran as though it had not been. The human approved folding into
// one word; it folded into whatever that row was called by then.
//
// Both directions run here, and the second is not ceremony: a check that
// refuses every redemption passes the first half perfectly.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedTag mints one word. The name carries part of the id because uq_tag_name
// is unique on lower(name) across the whole database, and these cases share
// one — two of them seeding "SkewSource" collide on the second, which says
// nothing about pins.
//
// The TAIL of the id, not the head: a uuidv7 opens with its timestamp, so two
// minted in the same millisecond agree on their first eight characters and the
// suffix distinguishes nothing.
func (e *stagingEnv) seedTag(t *testing.T, name string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO tag (id, name) VALUES ($1, $2)`,
		id, name+"-"+id.String()[24:]); err != nil {
		t.Fatalf("seeding tag %q: %v", name, err)
	}
	return id
}

// mergeCall is the staging merge_tags makes: the retired word is the target,
// the surviving word is the co-target.
func (e *stagingEnv) mergeCall(source, survivor ids.UUID) StageInput {
	return StageInput{
		Kind:           "merge_tags",
		ProposedChange: []byte(`{"tag_id":"` + source.String() + `","into_tag_id":"` + survivor.String() + `"}`),
		DiffHash:       "b91a2f0c4d6e8a13-one-merge",
		TargetType:     "tag",
		TargetID:       source,
		CoTargetType:   "tag",
		CoTargetID:     survivor,
		Summary:        `Fold tag "SkewSource" into "SkewTarget", releasing the name "SkewSource"`,
	}
}

// The reported reproduction, end to end: rename the SURVIVOR while the card is
// pending and the redemption must refuse.
func TestRenamingTheWordAMergeFoldsIntoRefusesTheRedemption(t *testing.T) {
	e := setupStaging(t)
	passport := e.seedPassport(t)
	source := e.seedTag(t, "SkewSource")
	survivor := e.seedTag(t, "SkewTarget")

	in := e.mergeCall(source, survivor)
	id, err := e.svc.Stage(e.asPassport(passport), in)
	if err != nil {
		t.Fatalf("staging the merge: %v", err)
	}
	e.approve(t, id)

	// The human has approved "fold SkewSource into SkewTarget". Somebody now
	// renames the survivor — HTTP 200, nothing refuses it, and the card still
	// reads the old sentence.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE tag SET name = 'CompletelyDifferentWord', version = version + 1 WHERE id = $1`,
		survivor); err != nil {
		t.Fatalf("renaming the survivor: %v", err)
	}

	_, _, err = e.svc.Redeem(e.asPassport(passport), id, in.Kind, in.DiffHash)
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("redeeming after the survivor was renamed = %v, want version skew — "+
			"the human approved folding into one word and this would fold into another", err)
	}
}

// The mirror: an untouched survivor redeems. Without this the case above is
// satisfied by a check that refuses everything.
func TestAMergeWhoseBothWordsHeldStillRedeems(t *testing.T) {
	e := setupStaging(t)
	passport := e.seedPassport(t)
	source := e.seedTag(t, "SkewSource")
	survivor := e.seedTag(t, "SkewTarget")

	in := e.mergeCall(source, survivor)
	id, err := e.svc.Stage(e.asPassport(passport), in)
	if err != nil {
		t.Fatalf("staging the merge: %v", err)
	}
	e.approve(t, id)

	if _, _, err := e.svc.Redeem(e.asPassport(passport), id, in.Kind, in.DiffHash); err != nil {
		t.Fatalf("redeeming a merge whose two words both held: %v", err)
	}
}

// A co-target of a type that carries no version column cannot be pinned, and
// minting a pin redemption could never verify is worse than refusing to stage.
func TestACoTargetThatCannotBePinnedIsRefusedAtStaging(t *testing.T) {
	e := setupStaging(t)
	passport := e.seedPassport(t)
	source := e.seedTag(t, "SkewSource")

	in := e.mergeCall(source, ids.NewV7())
	in.CoTargetType = "workspace"

	if _, err := e.svc.Stage(e.asPassport(passport), in); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("staging a co-target with no version to read = %v, want invalid argument", err)
	}
}
