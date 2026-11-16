// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commitmentparty

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAShortRefReadsBackToItsParticipant(t *testing.T) {
	t.Parallel()
	seat, contact := ids.MustParse("0198f3a1-7c42-7e0b-9d51-2a6f4b8c1e07"), ids.MustParse("0198f3a1-7c42-7e0b-9d51-2a6f4b8c1e08")
	parties := []Party{
		{Ref: "seat:" + seat.String(), Seat: &seat},
		{Name: "Unverified Sender"},
		{Ref: "contact:" + contact.String(), Contact: &contact},
	}
	refs := Refs(parties)
	if got := Key(refs, "p2"); got != parties[2].Ref {
		t.Errorf("p2 reads back to %q, want %q", got, parties[2].Ref)
	}
	if got := Key(refs, "p3"); got != "p3" {
		t.Errorf("an unminted ref reads back to %q; it must stay as written for the validator to refuse", got)
	}
	if got := Keys(refs, nil); got != nil {
		t.Errorf("an omitted recipient list became %v; omitted and empty mean different things", got)
	}
	if got, ok := Outside(refs, "p2", parties); !ok || got != contact {
		t.Errorf("p2 is the outside contact %s, got %s (%v)", contact, got, ok)
	}
	if _, ok := Outside(refs, "p1", parties); ok {
		t.Error("a colleague counts as an outside speaker")
	}
}
