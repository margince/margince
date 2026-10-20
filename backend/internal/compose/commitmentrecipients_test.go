// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestRecipientAttributionUsesOnlyVerifiedContacts(t *testing.T) {
	first, second, seat := ids.NewV7(), ids.NewV7(), ids.NewV7()
	parties := []commitmentParty{
		{Ref: "contact:" + first.String(), Contact: &first},
		{Ref: "contact:" + second.String(), Contact: &second},
		{Ref: "seat:" + seat.String(), Seat: &seat},
	}
	for _, test := range []struct {
		name    string
		refs    []string
		count   int
		refused bool
	}{
		{name: "legacy single recipient", count: 1},
		{name: "explicit unknown recipient", refs: []string{}, count: 0},
		{name: "two stated recipients", refs: []string{parties[1].Ref, parties[0].Ref}, count: 2},
		{name: "unknown reference", refs: []string{"contact:" + ids.NewV7().String()}, refused: true},
		{name: "colleague is not a contact recipient", refs: []string{parties[2].Ref}, refused: true},
		{name: "duplicate reference", refs: []string{parties[0].Ref, parties[0].Ref}, refused: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			legacy := ids.From[ids.ContactKind](first)
			c := Commitment{PromisedTo: &legacy}
			err := attributeCommitmentRecipients(&c, test.refs, parties)
			if test.refused {
				if !errors.Is(err, apperrors.ErrVersionSkew) {
					t.Fatalf("unverified recipients: %v", err)
				}
				if c.PromisedTo == nil || *c.PromisedTo != legacy {
					t.Fatal("refused attribution changed recipient state")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := len(commitmentRecipients(c)); got != test.count {
				t.Fatalf("recipients=%d, want %d", got, test.count)
			}
		})
	}
}
