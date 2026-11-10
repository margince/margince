// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A verdict releases a cleared sender's mail after it has written the contact.
// An ensure holding one of those messages may be waiting for that contact, so
// the release skips the message and a later pass opens it.
func TestAClearedSenderReleaseSkipsAMessageAnEnsureHolds(t *testing.T) {
	e := integration.Setup(t)
	const sender = "chi@fvhospital.example"
	mail := seedCapturedMail(t, e, sender, "FVH | Remaining specialities")
	seedPostureHeldImport(t, e, mail, e.Rep1)
	dispositionID := seedPendingDisposition(t, e, sender, "fvhospital.example", mail)
	e.WsExec(t, `UPDATE capture_pending_counterparty SET status = 'real', kind = 'contact' WHERE id = $1`, dispositionID)
	created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Chi", Source: "manual"})
	if err != nil {
		t.Fatalf("creating the contact: %v", err)
	}
	contact := ids.UUID(created.Id)
	ensure := holdMessageForAnEnsure(t, e, ids.From[ids.ActivityKind](mail), contact)

	var pid int32
	started, released, returned := make(chan error, 1), make(chan error, 1), make(chan struct{})
	go func() {
		released <- database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			err := tx.QueryRow(e.Admin(), `SELECT pg_backend_pid()`).Scan(&pid)
			if err == nil {
				_, err = tx.Exec(e.Admin(), `SELECT id FROM contact WHERE id = $1 FOR UPDATE`, contact)
			}
			started <- err
			if err != nil {
				return err
			}
			defer close(returned)
			_, err = capture.WidenClearedSenderTx(e.Admin(), tx, sender, clearedSenderLiveBound, activities.RecomputeAudienceTx)
			return err
		})
	}()
	if err := <-started; err != nil {
		t.Fatalf("locking the contact before the release: %v", err)
	}
	observer, err := e.Pool.Acquire(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Release()
	testdb.AwaitReturnWithoutWaiting(t, observer.Conn(), pid, returned,
		"the release on a held contact, beside an ensure holding the sender's message,")
	if err := <-released; err != nil {
		t.Fatalf("the release: %v", err)
	}
	if got, _ := audienceAndReason(t, e, mail); got != "participants" {
		t.Fatalf("the release opened the message the ensure held, as %q", got)
	}

	ensure.lockContact()
	ensure.commit()
	if err := ensure.wait(); err != nil {
		t.Fatalf("the ensure: %v", err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := capture.WidenClearedSenderTx(e.Admin(), tx, sender, clearedSenderLiveBound, activities.RecomputeAudienceTx)
		return err
	}); err != nil {
		t.Fatalf("the next release: %v", err)
	}
	if got, _ := audienceAndReason(t, e, mail); got != "workspace" {
		t.Errorf("the next release left the message %q, want workspace", got)
	}
}

// messageHold is an ensure's transaction in capture's order: the message, then
// the contact on lockContact, then commit.
type messageHold struct {
	finished            chan struct{}
	lockContact, commit func()
	err                 error
}

func holdMessageForAnEnsure(t *testing.T, e *integration.Env, activity ids.ActivityID, contact ids.UUID) *messageHold {
	t.Helper()
	start, release := make(chan struct{}), make(chan struct{})
	h := &messageHold{
		finished:    make(chan struct{}),
		lockContact: sync.OnceFunc(func() { close(start) }), commit: sync.OnceFunc(func() { close(release) }),
	}
	locked := make(chan error, 1)
	go func() {
		defer close(h.finished)
		h.err = database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			_, err := tx.Exec(e.Admin(), `SELECT id FROM activity WHERE id = $1 FOR UPDATE`, activity)
			locked <- err
			if err != nil {
				return err
			}
			<-start
			if _, err := tx.Exec(e.Admin(), `SELECT id FROM contact WHERE id = $1 FOR UPDATE`, contact); err != nil {
				return err
			}
			<-release
			return nil
		})
	}()
	if err := <-locked; err != nil {
		t.Fatalf("locking the message for the ensure: %v", err)
	}
	t.Cleanup(func() {
		h.lockContact()
		h.commit()
		<-h.finished
	})
	return h
}

func (h *messageHold) wait() error {
	<-h.finished
	return h.err
}
