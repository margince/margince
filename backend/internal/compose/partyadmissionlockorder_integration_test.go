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
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A colleague's import of the same message holds a key share on the activity,
// then recomputes its audience. A party admission waits for that import before
// its link insert shares the activity. So the recompute never waits on it.
func TestAnImportRecomputesBesideAPartyAdmissionWaitingForIt(t *testing.T) {
	e := integration.Setup(t)
	sink := newCaptureSink(e.Pool, CaptureConfig{})
	ref := capturePartiesMailUsing(t, e, sink.WithEnsurer(nil, nil), partyMailProof{outbound: true, sentByOwner: true},
		"Bob Buyer <bob@buyer.test>", "", "Could we discuss the product requirements?")
	activity := ids.From[ids.ActivityKind](ref.ID)
	importer := holdColleagueImport(t, e, activity)

	admitted, returned := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(returned)
		admitted <- sink.AdmitActivityParties(threadConnectorCtx(e, e.AdminUser), activity)
	}()
	observer, err := e.Pool.Acquire(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Release()
	testdb.AwaitQueuedBehind(t, observer.Conn(), importer.pid, 1, returned, "the party admission")

	importer.recompute()
	testdb.AwaitReturnWithoutWaiting(t, observer.Conn(), importer.pid, importer.recomputed,
		"the import's recompute, beside the party admission waiting on the import,")
	importer.commit()
	if err := importer.wait(); err != nil {
		t.Fatalf("the colleague's import: %v", err)
	}
	if err := <-admitted; err != nil {
		t.Fatalf("the party admission: %v", err)
	}
	if e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)='bob@buyer.test'`) != 1 {
		t.Fatal("the admission did not file the recipient once the import committed")
	}
}

// colleagueImport is a second seat's import transaction on one activity: its
// import row is written, and it recomputes and commits when the test says so.
type colleagueImport struct {
	pid               int32
	recomputed        chan struct{}
	finished          chan struct{}
	recompute, commit func()
	err               error
}

func holdColleagueImport(t *testing.T, e *integration.Env, activity ids.ActivityID) *colleagueImport {
	t.Helper()
	start, release := make(chan struct{}), make(chan struct{})
	c := &colleagueImport{
		recomputed: make(chan struct{}), finished: make(chan struct{}),
		recompute: sync.OnceFunc(func() { close(start) }), commit: sync.OnceFunc(func() { close(release) }),
	}
	written := make(chan error, 1)
	go func() {
		defer close(c.finished)
		c.err = database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			err := tx.QueryRow(e.Admin(), `SELECT pg_backend_pid()`).Scan(&c.pid)
			if err == nil {
				_, err = tx.Exec(e.Admin(), `INSERT INTO capture_import (activity_id, user_id, posture_at_import) VALUES ($1, $2, 'shared')`, activity, e.Rep3)
			}
			written <- err
			if err != nil {
				return err
			}
			<-start
			err = activities.RecomputeAudienceTx(e.Admin(), tx, activity)
			close(c.recomputed)
			if err != nil {
				return err
			}
			<-release
			return nil
		})
	}()
	if err := <-written; err != nil {
		t.Fatalf("writing the colleague's import row: %v", err)
	}
	t.Cleanup(func() {
		c.recompute()
		c.commit()
		<-c.finished
	})
	return c
}

func (c *colleagueImport) wait() error {
	<-c.finished
	return c.err
}
