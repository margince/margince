// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The read's cursor against the thread's own lower end.
//
// The window reads one correspondence, and threadHasMoved offers a thread
// while its oldest message lies below the cursor. These tests stage a thread
// whose oldest mail is another correspondence's, where the two used to disagree.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// olderCorrespondenceThread captures one mail from orders@beta.test on the
// pricing key before alphaThread. The thread's oldest message is then outside
// the correspondence its window reads. It returns the thread key and that mail.
func olderCorrespondenceThread(t *testing.T, o *owedEnv) (string, ids.UUID) {
	t.Helper()
	beta := o.capture(t, pricingMail("orders@beta.test", o.seat, "order-1@beta.test", betaWrites, -30, true))
	confirms := alphaThread(t, o)
	requireOneThreadKey(t, o, confirms, 4)
	return o.e.WsScalar(t, `SELECT thread_key FROM activity WHERE id = $1`, confirms), beta
}

// threadEnds reads the thread's oldest message and newest instant as the
// queue measures them.
func threadEnds(t *testing.T, o *owedEnv, key string) (oldest time.Time, oldestID ids.UUID, newest time.Time) {
	t.Helper()
	as := o.e.Admin()
	if err := database.WithWorkspaceTx(as, o.e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(as, `
			SELECT occurred_at, id FROM activity
			 WHERE thread_key = $1 AND kind = 'email' AND archived_at IS NULL
			 ORDER BY occurred_at, id LIMIT 1`, key).Scan(&oldest, &oldestID); err != nil {
			return err
		}
		return tx.QueryRow(as, `
			SELECT max(occurred_at) FROM activity
			 WHERE thread_key = $1 AND kind = 'email' AND archived_at IS NULL`, key).Scan(&newest)
	}); err != nil {
		t.Fatalf("reading the thread's ends: %v", err)
	}
	return oldest, oldestID, newest
}

func readWindow(t *testing.T, o *owedEnv, thread *settledThread) []threadMessage {
	t.Helper()
	var window []threadMessage
	as := o.e.Admin()
	if err := database.WithWorkspaceTx(as, o.e.Pool, func(tx pgx.Tx) error {
		var err error
		window, err = threadMessages(as, tx, thread)
		return err
	}); err != nil {
		t.Fatalf("reading the window: %v", err)
	}
	return window
}

func requireCursorAtThreadStart(t *testing.T, thread settledThread, oldest time.Time, oldestID ids.UUID) {
	t.Helper()
	if thread.ReadFromNow == nil || thread.ReadFromIDNow == nil {
		t.Fatal("the read left no cursor")
	}
	if !thread.ReadFromNow.Equal(oldest) || *thread.ReadFromIDNow != oldestID {
		t.Errorf("the cursor stopped at (%s, %s), above the thread's oldest message (%s, %s): "+
			"threadHasMoved would offer this thread again every pass",
			thread.ReadFromNow, thread.ReadFromIDNow, oldest, oldestID)
	}
}

func TestAFirstReadThatReachesAllItMayReadCoversTheThread(t *testing.T) {
	o := setupOwed(t)
	key, beta := olderCorrespondenceThread(t, o)
	oldest, oldestID, newest := threadEnds(t, o, key)
	if oldestID != beta {
		t.Fatal("precondition: the thread's oldest message is not the other correspondence's mail")
	}

	thread := settledThread{Key: key, Newest: newest, Count: 4, Oldest: &oldest, OldestID: &oldestID}
	for _, message := range readWindow(t, o, &thread) {
		if message.ID == beta {
			t.Fatal("precondition: the window read the other correspondence's mail")
		}
	}
	requireCursorAtThreadStart(t, thread, oldest, oldestID)
}

func TestAReadWithNothingOlderLeftToReadStopsWalkingBack(t *testing.T) {
	o := setupOwed(t)
	key, _ := olderCorrespondenceThread(t, o)
	oldest, oldestID, newest := threadEnds(t, o, key)

	// The state a read before coverThreadStart left. Everything up to the
	// newest message is read, and the cursor sits at the correspondence's own
	// oldest mail, one message above the thread's.
	var alphaFrom time.Time
	var alphaFromID ids.UUID
	as := o.e.Admin()
	if err := database.WithWorkspaceTx(as, o.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(as, `
			SELECT occurred_at, id FROM activity
			 WHERE thread_key = $1 AND kind = 'email' AND id <> $2
			 ORDER BY occurred_at, id LIMIT 1`, key, oldestID).Scan(&alphaFrom, &alphaFromID)
	}); err != nil {
		t.Fatal(err)
	}
	thread := settledThread{
		Key: key, Newest: newest, Count: 4,
		ReadTo: &newest, ReadFrom: &alphaFrom, ReadFromID: &alphaFromID,
		Oldest: &oldest, OldestID: &oldestID,
	}
	if window := readWindow(t, o, &thread); len(window) == 0 {
		t.Fatal("the fallback to the newest window read nothing")
	}
	requireCursorAtThreadStart(t, thread, oldest, oldestID)
}
