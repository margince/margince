// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A cohort repair locks the activities it writes before the contact. The
// capture ensure and the meeting roster settle lock an activity, then its
// contact. The opposite order closes a cycle.

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestACohortRepairWaitsForTheActivityBeforeItLocksTheContact(t *testing.T) {
	for name, seed := range map[string]func(e *dedupeEnv, ctx context.Context, t *testing.T, email string) ids.ActivityID{
		"mail": func(e *dedupeEnv, ctx context.Context, t *testing.T, email string) ids.ActivityID {
			return e.seedCapturedMail(ctx, t, email, "from")
		},
		"meeting": func(e *dedupeEnv, ctx context.Context, t *testing.T, email string) ids.ActivityID {
			return e.seedCapturedMeeting(ctx, t, email)
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			const email = "busy@cohort.test"
			activity := seed(e, ctx, t, email)
			contact := e.ensureContact(ctx, t, email)
			ensure := e.holdActivityForAnEnsure(ctx, t, activity, contact)

			repaired, returned := make(chan error, 1), make(chan struct{})
			go func() {
				defer close(returned)
				_, err := e.store.RepairContactCohort(ctx, contact)
				repaired <- err
			}()
			observer := observerConn(t)
			testdb.AwaitQueuedBehind(t, observer, ensure.pid, 1, returned, "the cohort repair")

			ensure.lockContact()
			testdb.AwaitReturnWithoutWaiting(t, observer, ensure.pid, ensure.contactLocked,
				"the ensure's contact lock, beside the cohort repair waiting on its activity,")
			ensure.commit()
			if err := ensure.wait(); err != nil {
				t.Fatalf("the ensure: %v", err)
			}
			if err := <-repaired; err != nil {
				t.Fatalf("the cohort repair: %v", err)
			}
			if linked, _ := e.cohortStateOf(ctx, t, activity, contact); !linked {
				t.Fatal("the repair did not file the activity once the ensure committed")
			}
		})
	}
}

// A caller that already holds the contact cannot wait for an activity: the
// activity's holder may be waiting for that contact. The busy activity is left
// for the next pass.
func TestACohortRepairOnAHeldContactSkipsABusyActivity(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const email = "held@cohort.test"
	busy := e.seedCapturedMail(ctx, t, email, "from")
	free := e.seedCapturedMail(ctx, t, email, "from")
	contact := e.ensureContact(ctx, t, email)
	ensure := e.holdActivityForAnEnsure(ctx, t, busy, contact)

	var pid int32
	started, repaired, returned := make(chan error, 1), make(chan error, 1), make(chan struct{})
	go func() {
		repaired <- e.store.tx(ctx, func(tx pgx.Tx) error {
			err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid)
			if err == nil {
				_, err = tx.Exec(ctx, `SELECT id FROM contact WHERE id = $1 FOR UPDATE`, contact)
			}
			started <- err
			if err != nil {
				return err
			}
			defer close(returned)
			_, err = e.store.PromoteHeldContactCohortTx(ctx, tx, contact)
			return err
		})
	}()
	if err := <-started; err != nil {
		t.Fatalf("locking the contact before the repair: %v", err)
	}
	testdb.AwaitReturnWithoutWaiting(t, observerConn(t), pid, returned,
		"the cohort repair on a held contact, beside an ensure holding one of its activities,")
	if err := <-repaired; err != nil {
		t.Fatalf("the cohort repair: %v", err)
	}
	if linked, _ := e.cohortStateOf(ctx, t, free, contact); !linked {
		t.Error("the repair did not file the activity nobody held")
	}
	if linked, _ := e.cohortStateOf(ctx, t, busy, contact); linked {
		t.Error("the repair filed the activity the ensure held")
	}

	ensure.lockContact()
	ensure.commit()
	if err := ensure.wait(); err != nil {
		t.Fatalf("the ensure: %v", err)
	}
	if _, err := e.store.RepairContactCohort(ctx, contact); err != nil {
		t.Fatalf("the next cohort repair: %v", err)
	}
	if linked, _ := e.cohortStateOf(ctx, t, busy, contact); !linked {
		t.Error("the next pass did not file the activity the ensure had held")
	}
}

func (e *dedupeEnv) ensureContact(ctx context.Context, t *testing.T, email string) ids.ContactID {
	t.Helper()
	res, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, email, "Busy Sender", "cohort.test"))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	return res.ContactID
}

// observerConn is a session outside every transaction under test, for probing
// which backend waits on which.
func observerConn(t *testing.T) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatalf("connecting the lock observer: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("closing the lock observer: %v", err)
		}
	})
	return conn
}

// ensureHold is an ensure's transaction in the capture ensure's order: the
// activity, then the contact on lockContact, then commit.
type ensureHold struct {
	pid                 int32
	contactLocked       chan struct{}
	finished            chan struct{}
	lockContact, commit func()
	err                 error
}

func (e *dedupeEnv) holdActivityForAnEnsure(
	ctx context.Context, t *testing.T, activity ids.ActivityID, contact ids.ContactID,
) *ensureHold {
	t.Helper()
	start, release := make(chan struct{}), make(chan struct{})
	h := &ensureHold{
		contactLocked: make(chan struct{}), finished: make(chan struct{}),
		lockContact: sync.OnceFunc(func() { close(start) }), commit: sync.OnceFunc(func() { close(release) }),
	}
	locked := make(chan error, 1)
	go func() {
		defer close(h.finished)
		h.err = e.store.tx(ctx, func(tx pgx.Tx) error {
			err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&h.pid)
			if err == nil {
				_, err = tx.Exec(ctx, `SELECT id FROM activity WHERE id = $1 FOR UPDATE`, activity)
			}
			locked <- err
			if err != nil {
				return err
			}
			<-start
			_, err = tx.Exec(ctx, `SELECT id FROM contact WHERE id = $1 FOR UPDATE`, contact)
			close(h.contactLocked)
			if err != nil {
				return err
			}
			<-release
			return nil
		})
	}()
	if err := <-locked; err != nil {
		t.Fatalf("locking the activity for the ensure: %v", err)
	}
	t.Cleanup(func() {
		h.lockContact()
		h.commit()
		<-h.finished
	})
	return h
}

func (h *ensureHold) wait() error {
	<-h.finished
	return h.err
}
