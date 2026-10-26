// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A create that runs while an erasure of the same address is in flight. A row
// lock on the subject's note holds the erasure mid-transaction. The create's
// check then falls between the erasure's purge and its commit.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestALeadCreatedDuringAnErasureNeverKeepsTheErasedAddress(t *testing.T) {
	e := integration.Setup(t)
	ctx := context.Background()
	subject, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Die Betroffene",
		Emails:   []contacts.ContactEmailInput{{Email: erasedAddress, EmailType: "work", IsPrimary: true}},
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("creating the subject: %v", err)
	}
	note, _, err := e.Activities.LogActivity(machineCtx(e), activities.LogActivityInput{
		Kind: "note", Source: "import", SourceSystem: new("erasure-race"), SourceID: new("note-1"),
		Subject: new("Notiz"), Body: new("Nur über die Betroffene"),
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.UUID(subject.Id)}},
	})
	if err != nil {
		t.Fatalf("logging the subject's note: %v", err)
	}

	owner := integration.OwnerConn(t)
	gate, err := owner.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the holding transaction: %v", err)
	}
	defer func() {
		if rollbackErr := gate.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			t.Errorf("closing the holding transaction: %v", rollbackErr)
		}
	}()
	if _, err := gate.Exec(ctx, `SELECT id FROM activity WHERE id = $1 FOR UPDATE`, note.Id); err != nil {
		t.Fatalf("holding the note: %v", err)
	}

	erasureErr := make(chan error, 1)
	erasureDone := make(chan struct{})
	go func() {
		erasureErr <- privacy.NewEraser(e.DB()).EraseContact(e.Admin(), ids.UUID(subject.Id), "Art. 17 request")
		close(erasureDone)
	}()
	waitForLockWaiter(t, erasureDone, `'tuple', 'transactionid'`,
		"the erasure finished without reaching the held note")

	name, address := "Wieder Da", erasedAddress
	var leadErr error
	leadDone := make(chan struct{})
	go func() {
		_, _, leadErr = e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: &name, Email: &address})
		close(leadDone)
	}()
	waitForLockWaiter(t, leadDone, `'advisory'`,
		"the lead create finished without waiting for the erasure in flight, so it can store the erased address")

	if err := gate.Commit(ctx); err != nil {
		t.Fatalf("releasing the note: %v", err)
	}
	<-erasureDone
	if err := <-erasureErr; err != nil {
		t.Fatalf("the erasure failed: %v", err)
	}
	<-leadDone
	if _, refused := errors.AsType[*storekit.ErasedIdentifierError](leadErr); !refused {
		t.Errorf("the lead create answered %v, want the erasure refusal", leadErr)
	}
	if live := e.WsCount(t, `SELECT count(*) FROM lead WHERE email = $1 AND archived_at IS NULL`, erasedAddress); live != 0 {
		t.Fatalf("%d live lead(s) hold the erased address", live)
	}
}

// waitForLockWaiter returns once a backend in this database waits on a lock
// of the given kinds.
func waitForLockWaiter(t *testing.T, done <-chan struct{}, kinds, finishedEarly string) {
	t.Helper()
	owner := integration.OwnerConn(t)
	testdb.WaitForContention(t, done, finishedEarly, "no backend waited on the lock in time",
		func(ctx context.Context) (bool, error) {
			var waiting bool
			err := owner.QueryRow(ctx, `
				SELECT EXISTS (SELECT 1 FROM pg_locks l
				  JOIN pg_stat_activity a ON a.pid = l.pid
				 WHERE NOT l.granted AND l.locktype IN (`+kinds+`)
				   AND a.datname = current_database())`).Scan(&waiting)
			return waiting, err
		})
}
