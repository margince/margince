// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package overlay

// What `writable` answers on a mirrored record.
//
// Absent means NOT writable, which is the right default for a client that
// cannot tell and the wrong permanent answer for a server that can: an overlay
// user entitled to edit a record saw no way to, on every row, and concluded the
// product cannot edit the incumbent's records at all.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// seatWith binds an actor holding exactly the named actions on every object.
func seatWith(actions ...principal.Action) context.Context {
	grant := principal.ObjectGrant{}
	for _, a := range actions {
		switch a {
		case principal.ActionRead:
			grant.Read = true
		case principal.ActionUpdate:
			grant.Update = true
		case principal.ActionCreate:
			grant.Create = true
		}
	}
	objects := map[string]principal.ObjectGrant{}
	for _, object := range []string{"contact", "company", "deal", "lead", "project", "activity"} {
		objects[object] = grant
	}
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:u-1",
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"}, Objects: objects, RowScope: principal.RowScopeAll,
		},
	})
}

func TestASupportedTypeWithTheGrantIsWritable(t *testing.T) {
	t.Parallel()
	ctx := seatWith(principal.ActionRead, principal.ActionUpdate)

	for _, et := range []datasource.EntityType{
		datasource.EntityContact, datasource.EntityCompany,
		datasource.EntityDeal, datasource.EntityLead,
	} {
		if !SupportsWrite(WriteUpdate, et) {
			t.Fatalf("%s is no longer an updatable overlay type, so this case is asserting about "+
				"a type the write-back contract has dropped", et)
		}
		if !Writable(ctx, et) {
			t.Errorf("%s reads as not writable to a seat that holds the update grant — the edit "+
				"affordances disappear on a record the overlay would have accepted", et)
		}
	}
}

func TestASupportedTypeWithoutTheGrantIsNotWritable(t *testing.T) {
	t.Parallel()
	ctx := seatWith(principal.ActionRead)

	if Writable(ctx, datasource.EntityContact) {
		t.Error("a read-only seat reads as writable: the flag would offer an edit the write refuses")
	}
}

// An unsupported type is not writable however entitled the seat. The two terms
// are AND, and the contract half is the one a grant cannot argue with.
//
// The type is FOUND rather than named: which types the write-back contract
// supports is its business and changes, and a case anchored on one that gains
// support would skip — proving nothing while reading as a pass.
func TestAnUnsupportedTypeIsNotWritableEvenWithTheGrant(t *testing.T) {
	t.Parallel()
	ctx := seatWith(principal.ActionRead, principal.ActionUpdate, principal.ActionCreate)

	var unsupported datasource.EntityType
	for _, et := range datasource.EntityTypes() {
		if !SupportsWrite(WriteUpdate, et) {
			unsupported = et
			break
		}
	}
	if unsupported == "" {
		t.Fatal("every entity type is updatable through the overlay, so the contract half of this " +
			"answer can no longer be observed — if that is deliberate, this case has to say so " +
			"rather than quietly stop checking")
	}
	if Writable(ctx, unsupported) {
		t.Errorf("%s reads as writable though the write-back contract does not support updating it",
			unsupported)
	}
}

// The flag says nothing about CREATE, and a client reading it that way would
// offer a button for a write the overlay refuses on every type.
func TestNoOverlayTypeSupportsCreate(t *testing.T) {
	t.Parallel()

	for _, et := range datasource.EntityTypes() {
		if SupportsWrite(WriteCreate, et) {
			t.Errorf("%s reports create support: `writable` answers the UPDATE question, so a type "+
				"that gained create needs the flag's meaning revisited rather than inherited", et)
		}
	}
}

// An actor with no seat at all is not writable, and does not panic answering.
// The assemblers run on read paths that a system principal also reaches.
func TestAnUnseatedCallerIsNotWritable(t *testing.T) {
	t.Parallel()

	if Writable(context.Background(), datasource.EntityContact) {
		t.Error("a context with no actor read as writable")
	}
}
