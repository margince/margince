// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// The batched merge-card read, against a real database.
//
// Two things have to hold, and neither can be proven against a mock. The row
// scope must still decide every id — a set-wise predicate is a different
// statement from twenty single-row checks, and a set-wise one that leaks is a
// dedupe queue handing out records the reader may not see. And the detail line
// must name what the RECORD PAGE names: it comes from a narrow query written
// here rather than from the composite read, so the two orderings are free to
// drift, and a merge card that names a person by an address they are not
// otherwise known by is worse than one that names nothing.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// addEmail files one address on a person at the given position.
func (e *privacyEnv) addEmail(t *testing.T, id ids.PersonID, email string, position int) {
	t.Helper()
	ctx := e.as(e.owner, principal.RowScopeOwn)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO person_email (id, person_id, email, position, source, captured_by)
			VALUES ($1, $2, $3, $4, 'gmail:seed', 'connector:gmail')`,
			ids.NewV7(), id, email, position)
		return err
	}); err != nil {
		t.Fatalf("seeding an email: %v", err)
	}
}

// A ROW THE READER MAY NOT SEE IS ABSENT, not an error and not a name.
//
// The dedupe queue row proves a pair was detected, never that this reader may
// see what it points at. A batched read that took the queue's word for it would
// hand a rep the name and address of a colleague's private contact.
func TestABatchedMergeFaceStillObeysTheRowScope(t *testing.T) {
	e := setupCapturePrivacy(t)
	mine := e.capturePerson(t, "workspace")
	theirs := e.capturePerson(t, "owner")

	// The teammate shares a team with the owner but reads at team scope, which
	// is what makes the owner-private row invisible rather than out of reach.
	ctx := e.as(e.teammate, principal.RowScopeTeam)
	if e.canRead(ctx, t, theirs) {
		t.Fatal("precondition: the owner-private person should be invisible to the teammate")
	}

	faces, err := e.store.DescribeForMerge(ctx, "person", []ids.UUID{mine.UUID, theirs.UUID})
	if err != nil {
		t.Fatalf("DescribeForMerge: %v", err)
	}
	if _, named := faces[theirs.UUID]; named {
		t.Errorf("named a person this reader cannot see: %+v — the queue row is not permission "+
			"to read what it points at", faces[theirs.UUID])
	}
	face, named := faces[mine.UUID]
	if !named {
		t.Fatal("did not name the person this reader CAN see, so the check above proves nothing")
	}
	if face.Label != "Captured Contact" || face.CreatedAt.IsZero() {
		t.Errorf("face = %+v, want the display name and the day it arrived", face)
	}
}

// AND THE DETAIL LINE IS THE ONE THE RECORD PAGE SHOWS.
//
// Both reads order by (position, created_at); this seeds them out of that order
// so a query that took whatever the database offered would pick the other one.
func TestAMergeFaceNamesTheSameAddressTheRecordPageDoes(t *testing.T) {
	e := setupCapturePrivacy(t)
	id := e.capturePerson(t, "workspace")
	e.addEmail(t, id, "second@example.test", 2)
	e.addEmail(t, id, "first@example.test", 1)

	ctx := e.as(e.owner, principal.RowScopeOwn)
	record, err := e.store.GetPerson(ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("GetPerson: %v", err)
	}
	if record.Emails == nil || len(*record.Emails) == 0 {
		t.Fatal("precondition: the record page should carry the seeded addresses")
	}
	shown := string((*record.Emails)[0].Email)

	faces, err := e.store.DescribeForMerge(ctx, "person", []ids.UUID{id.UUID})
	if err != nil {
		t.Fatalf("DescribeForMerge: %v", err)
	}
	if got := faces[id.UUID].Detail; got != shown {
		t.Errorf("the card names %q and the record page names %q — a merge screen that calls "+
			"somebody by an address they are not otherwise known by is worse than one that "+
			"names nothing", got, shown)
	}
}

// AND AN OBJECT GRANT REFUSED IS AN ERROR, because it is not about these ids:
// the caller renders it as withheld, and it must be able to tell that from a
// database that would not answer.
func TestABatchedMergeFaceRefusesWithoutTheObjectGrant(t *testing.T) {
	e := setupCapturePrivacy(t)
	id := e.capturePerson(t, "workspace")

	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.owner.String(), UserID: e.owner,
		Permissions: principal.Permissions{RoleKeys: []string{"rep"}, RowScope: principal.RowScopeAll},
	})

	if _, err := e.store.DescribeForMerge(ctx, "person", []ids.UUID{id.UUID}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("err = %v, want ErrPermissionDenied for a caller with no person.read", err)
	}
}
