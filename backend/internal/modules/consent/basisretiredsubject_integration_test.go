// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// A send authorized while its subject is merged away records nothing on the
// retired record.
//
// The gate resolves a recipient to a live record, then decides, then writes
// the ground it relied on. A merge can commit between the resolution and the
// write. It carries the retired record's consent rows to the survivor and
// archives it.
//
// A basis written after that carry has passed sits where no read returns it.
// The installation could then not say what it relied on for a message it sent,
// which is the gap `communication_basis` exists to close.

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// retireContact does to a contact what a merge's last step does: archives it
// and points it at the survivor, so it stays fetchable by id.
func retireContact(t *testing.T, e *channelConsentEnv, retired, survivor ids.ContactID) {
	t.Helper()
	if _, err := e.owner.Exec(e.ctx,
		`UPDATE contact SET archived_at = now(), merged_into_id = $2 WHERE id = $1`,
		retired, survivor); err != nil {
		t.Fatalf("retiring the merged contact: %v", err)
	}
}

func TestABasisIsRefusedForASubjectAMergeRetired(t *testing.T) {
	e := setupChannelConsent(t)
	survivor := seedContactWithEmail(t, e, "survivor@example.test", false)
	ctx := principal.WithActor(e.ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{
			"contact": {Read: true, Create: true, Update: true},
		}},
	})
	// The merge has run and the carry has passed. Its resolution happened while
	// the record was live, which is why the write is the moment that has to
	// check.
	retireContact(t, e, e.contact, survivor)

	err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		return recordBasis(ctx, tx, subjectRef{Kind: entityContact, ID: e.contact.String()},
			resolution{Supported: true, Basis: commsauthz.BasisSubjectInitiatedCorrespondence},
			commsauthz.Request{}, packRules{reply: 24 * time.Hour})
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("recording a basis on a retired subject answered %v, want ErrNotFound so the "+
			"send is refused and retried against the survivor", err)
	}
	var rows int
	if err := e.owner.QueryRow(e.ctx,
		`SELECT count(*) FROM communication_basis WHERE contact_id = $1`, e.contact).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("communication_basis rows on the retired contact = %d, want 0", rows)
	}
}

// The live subject still records, so the lock refuses the retired case rather
// than the write itself.
func TestABasisStillRecordsForALiveSubject(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := principal.WithActor(e.ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{
			"contact": {Read: true, Create: true, Update: true},
		}},
	})
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		return recordBasis(ctx, tx, subjectRef{Kind: entityContact, ID: e.contact.String()},
			resolution{Supported: true, Basis: commsauthz.BasisSubjectInitiatedCorrespondence},
			commsauthz.Request{}, packRules{reply: 24 * time.Hour})
	}); err != nil {
		t.Fatalf("recording a basis on a live subject: %v", err)
	}
	var rows int
	if err := e.owner.QueryRow(e.ctx,
		`SELECT count(*) FROM communication_basis WHERE contact_id = $1`, e.contact).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("communication_basis rows on the live contact = %d, want 1", rows)
	}
}
