// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The digest counts the duplicate cards of the reader's Mine page and no
// others, pairs and contact cases alike.
func TestTheDigestCountsTheReadersOwnDuplicateCards(t *testing.T) {
	reader, stranger := readerOf(pageReader()), ids.NewV7()
	mine := DuplicatePair{ID: ids.NewV7(), EntityType: "company", Owners: []ids.UUID{reader}}
	theirs := DuplicatePair{ID: ids.NewV7(), EntityType: "company", Owners: []ids.UUID{stranger}}
	nobodys := DuplicatePair{ID: ids.NewV7(), EntityType: "lead"}
	pairs := stubDuplicates{pairs: []DuplicatePair{mine, theirs, nobodys}, open: 3, byOwner: true}
	cases := &stubContactCases{cases: []ContactCase{aContactCase(reader), aContactCase(stranger), aContactCase()}}

	got, err := CountOwnReview(pageReader(), pairs, cases)
	if err != nil || got != 2 {
		t.Fatalf("the digest counts %d, err %v, want the reader's one pair and one case", got, err)
	}
	if got, err := CountOwnReview(context.Background(), pairs, cases); err != nil || got != 0 {
		t.Fatalf("a read with no reader counts %d, err %v, want 0", got, err)
	}
}
