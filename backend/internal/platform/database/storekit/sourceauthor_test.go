// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// Whether the row already carries exactly the answer being offered.
//
// This decides whether the repair WRITES, so a wrong `true` skips a correction
// and a wrong `false` restamps a record for nothing. The asymmetric cases are
// the ones worth pinning: an offer that drops a seat id, or adds one, is a
// different answer even when the name is identical, and a nil is not an empty
// string.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOnlyAnIdenticalAnswerCountsAsUnchanged(t *testing.T) {
	seat := ids.NewV7()
	other := ids.NewV7()
	name := "Mutaz Suleiman"
	otherName := "Shayne Ahsan"
	empty := ""

	ptr := func(s string) *string { return &s }
	id := func(u ids.UUID) *ids.UUID { return &u }

	for _, c := range []struct {
		what      string
		beforeID  *ids.UUID
		beforeNam *string
		in        SourceAuthorInput
		same      bool
	}{
		{what: "both halves match", beforeID: id(seat), beforeNam: &name, same: true, in: SourceAuthorInput{AuthorID: id(seat), AuthorName: &name}},
		{what: "name only, on a row carrying name only", beforeNam: &name, same: true, in: SourceAuthorInput{AuthorName: &name}},
		{what: "seat only, on a row carrying seat only", beforeID: id(seat), same: true, in: SourceAuthorInput{AuthorID: id(seat)}},
		{what: "a different name under the same seat", beforeID: id(seat), beforeNam: &name, in: SourceAuthorInput{AuthorID: id(seat), AuthorName: &otherName}},
		{what: "a different seat under the same name", beforeID: id(seat), beforeNam: &name, in: SourceAuthorInput{AuthorID: id(other), AuthorName: &name}},
		{what: "the offer DROPS the seat the row carries", beforeID: id(seat), beforeNam: &name, in: SourceAuthorInput{AuthorName: &name}},
		{what: "the offer ADDS a seat the row lacks", beforeNam: &name, in: SourceAuthorInput{AuthorID: id(seat), AuthorName: &name}},
		{what: "nothing stored yet", in: SourceAuthorInput{AuthorName: &name}},
		{what: "an empty name is not an absent one", beforeNam: ptr(empty), in: SourceAuthorInput{AuthorName: &name}},
		{what: "a stored name against an EMPTY offered one", beforeNam: &name, in: SourceAuthorInput{AuthorName: ptr(empty)}},
		{what: "an absent stored name against an empty offered one", in: SourceAuthorInput{AuthorName: ptr(empty)}},
		{what: "an empty stored name against an empty offered one", beforeNam: ptr(empty), same: true, in: SourceAuthorInput{AuthorName: ptr(empty)}},
	} {
		before := SourceAuthorBefore{ID: c.beforeID, Name: c.beforeNam}
		if got := before.Same(c.in); got != c.same {
			t.Errorf("%s: Same = %v, want %v", c.what, got, c.same)
		}
	}
}
