// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

// A cancellation must wait for the seat's own import row still in flight.
// Otherwise it judges the seat a stranger, and the meeting stays booked.
func TestACancellationWaitsForTheSeatsImportInFlight(t *testing.T) {
	e := integration.Setup(t)
	id := captureMeeting(t, e, e.AdminUser)

	importer, written := make(chan int32, 1), make(chan error, 1)
	commit := make(chan struct{})
	release := sync.OnceFunc(func() { close(commit) })
	t.Cleanup(release)
	go func() {
		written <- database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			var pid int32
			err := tx.QueryRow(context.Background(), `SELECT pg_backend_pid()`).Scan(&pid)
			if err == nil {
				_, err = tx.Exec(context.Background(),
					`INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, id, e.Rep1)
			}
			if err != nil {
				return err
			}
			importer <- pid
			<-commit
			return nil
		})
		close(importer)
	}()
	holder, ok := <-importer
	if !ok {
		t.Fatalf("writing the seat's import row: %v", <-written)
	}

	cancelled, returned := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(returned)
		cancelled <- calendarSink(e).CancelMeeting(calendarOwnerCtx(e, e.Rep1), meetingKey, meetingStart)
	}()
	observer, err := e.Pool.Acquire(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Release()
	testdb.AwaitQueuedBehind(t, observer.Conn(), holder, 1, returned, "the cancellation")
	release()
	if err := <-written; err != nil {
		t.Fatalf("committing the seat's import row: %v", err)
	}
	if err := <-cancelled; err != nil {
		t.Fatalf("cancelling the meeting: %v", err)
	}
	if status, set := readMeetingStatus(t, e, id); !set || status != "canceled" {
		t.Fatalf("meeting_status = %q (set=%v), want canceled once the seat's import committed", status, set)
	}
}
