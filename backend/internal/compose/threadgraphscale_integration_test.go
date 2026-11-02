// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func seedThreadGraphScale(t *testing.T, size int, shape string) (*integration.Env, []string, string) {
	t.Helper()
	e := integration.Setup(t)
	addr := seatAddress(t, e, e.Rep1)
	keys := make([]string, size)
	messages := make([]ids.ActivityID, size)
	for i := range size {
		keys[i] = fmt.Sprintf("scale-%04d@counterparty.example", i)
		topic := ""
		if (shape == "reverse ancestor" || shape == "reverse frontier") && i == size-1 || (shape == "blank cycle with named branch" || shape == "star") && i == 0 {
			topic = "Price"
		}
		captureThreadMail(t, e, e.Rep1, subjectThreadMail(addr, keys[i], topic, ""))
		messages[i] = capturedThreadActivity(t, e, keys[i])
	}
	finishThreadRepairs(t, e)
	for i, id := range messages {
		parent := -1
		switch shape {
		case "reverse ancestor":
			if i < size-1 {
				parent = i + 1
			}
		case "reverse frontier", "star":
			if i > 0 {
				parent = i - 1
			}
			if shape == "star" && i > 0 {
				parent = 0
			}
		case "blank cycle with named branch":
			parent = i + 1
			if parent == size {
				parent = 1
			}
		}
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			if parent >= 0 {
				if err := activities.RecordMailReferencesTx(e.Admin(), tx, id, []string{keys[parent]}); err != nil {
					return err
				}
			}
			if i == 0 {
				return nil
			}
			return mergeThreadsTx(e.Admin(), tx, keys[i], keys[0])
		}); err != nil {
			t.Fatal(err)
		}
	}
	want := keys[0]
	if shape == "reverse ancestor" {
		want = keys[size-1]
	}
	return e, keys, want
}

func measureThreadGraphRepair(t *testing.T, size int, shape string) int {
	t.Helper()
	e, keys, want := seedThreadGraphScale(t, size, shape)
	units := drainMeasuredThreadGraph(t, e, keys[0], size, 7)
	for _, key := range keys {
		if got := threadKeyOfMessage(t, e, key); got != want {
			t.Fatalf("%s root=%s, want %s", key, got, want)
		}
	}
	t.Logf("shape=%s nodes=%d total persisted units=%d", shape, size, units)
	return units
}

func drainMeasuredThreadGraph(t *testing.T, e *integration.Env, source string, size, limit int) int {
	t.Helper()
	run := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error { return capture.QueueThreadRepairTx(e.Admin(), tx, source) }); err != nil {
		t.Fatal(err)
	}
	units := 0
	for units <= 96*size+128 {
		done := false
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			scanned, _, finished, err := capture.ThreadRepairBatchTx(e.Admin(), tx, limit, run, threadRepairApplier(e.Pool))
			if scanned > limit {
				t.Fatalf("one batch consumed %d units, limit %d", scanned, limit)
			}
			units += scanned
			done = finished
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if !done {
			continue
		}
		if failures := e.WsCount(t, `SELECT count(*) FROM capture_thread_repair WHERE state<>'ready'`); failures != 0 {
			t.Fatalf("repair left %d failed components", failures)
		}
		return units
	}
	t.Fatalf("repair consumed %d units for %d messages", units, size)
	return 0
}

func TestThreadRepairTotalWorkGrowsWithMessagesRatherThanTheirArrivalOrder(t *testing.T) {
	for _, shape := range []string{"reverse ancestor", "reverse frontier", "blank cycle with named branch", "star"} {
		t.Run(shape, func(t *testing.T) {
			small := measureThreadGraphRepair(t, 48, shape)
			large := measureThreadGraphRepair(t, 96, shape)
			if large > 2*small+128 {
				t.Fatalf("doubling messages increased total persisted units from%d to%d", small, large)
			}
		})
	}
}
