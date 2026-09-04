// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// What a merge staging binds itself to.
//
// The approvals engine pins whatever a subject declares; this is the half that
// says merge_tags declares BOTH words. Without it the engine's own cases pass
// over a stager that names only the retired one — which is the state the defect
// was in.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAMergeStagingPinsBothWords(t *testing.T) {
	t.Parallel()
	source, survivor := ids.NewV7(), ids.NewV7()
	r := &mergeTagsResolver{tags: stubTags{names: map[ids.UUID]string{
		source: "SkewSource", survivor: "SkewTarget",
	}}}

	got, err := r.Subject(context.Background(), MergeTagsCommand{
		SourceID: source, TargetID: survivor,
	})
	if err != nil {
		t.Fatalf("Subject: %v", err)
	}

	// The retired word is the target: it is the row the merge destroys.
	if got.TargetID != source || got.TargetType != tagRecordType {
		t.Errorf("target = %s/%s, want the retired word", got.TargetType, got.TargetID)
	}
	// And the survivor is the co-target, so a rename between the card and the
	// release is refused rather than silently folded into.
	if got.CoTargetID != survivor || got.CoTargetType != tagRecordType {
		t.Errorf("co-target = %s/%s, want the surviving word — the human approved "+
			"folding into a NAME, and nothing else pins it", got.CoTargetType, got.CoTargetID)
	}
}
