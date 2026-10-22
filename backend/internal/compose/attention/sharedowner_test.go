// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestDirectRecipientsShareResponsibilityWithoutBecomingUnassigned(t *testing.T) {
	t.Parallel()
	alice, bob := ids.NewV7(), ids.NewV7()
	row := ranked{ownerRef: waitingOwner(ids.Nil, true, []ids.UUID{alice, bob})}
	owner := ownerOnTheWire(row, alice)
	if owner == nil || owner.Kind != crmcontracts.WorklistOwnerKindWorklistOwnerShared || owner.Users == nil || len(*owner.Users) != 2 {
		t.Fatalf("shared responsibility = %#v", owner)
	}
	nameAnOwner(owner, map[ids.UUID]string{alice: "Alice"})
	if (*owner.Users)[0].Label == nil || *(*owner.Users)[0].Label != "Alice" || (*owner.Users)[1].Label != nil {
		t.Fatalf("roster naming = %#v", owner.Users)
	}
	selected := ownerOnTheWire(ranked{ownerRef: waitingOwner(bob, true, []ids.UUID{alice, bob})}, alice)
	if selected == nil || selected.Id == nil || ids.UUID(*selected.Id) != bob {
		t.Fatal("selected recipient lost personal attribution")
	}
	if ownerOfTheGroup([]ranked{row, row}).kind != ownerShared {
		t.Fatal("same shared group lost ownership")
	}
	other := ranked{ownerRef: waitingOwner(ids.Nil, true, []ids.UUID{alice, ids.NewV7()})}
	if ownerOfTheGroup([]ranked{row, other}).kind != ownerUnstated {
		t.Fatal("different shared groups were combined")
	}
}
