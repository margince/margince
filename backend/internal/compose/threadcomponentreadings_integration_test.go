// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAPaidReadingWaitsForItsComponentWithoutRepeatingTheModel(t *testing.T) {
	o := signalEnv(t)
	const key, words = "busy-reading@customer.example", "We are pleased with our agreement."
	o.customerSays(t, key, "", words, "", o.now.Add(-48*time.Hour))
	brain := &askedBrain{}
	pass := principal.SystemActing(principal.WithWorkspaceID(context.Background(), o.e.WS), signalScanActor)
	extractor := NewSignalExtractor(o.e.Pool, brain, func() time.Time { return o.now.Add(72 * time.Hour) }, slog.Default())
	finished := make(chan error, 1)
	if err := database.WithWorkspaceTx(pass, o.e.Pool, func(tx pgx.Tx) error {
		if err := storekit.WaitThreadKeysTx(pass, tx, []string{key}); err != nil {
			return err
		}
		go func() { _, err := extractor.RunWorkspace(pass, ids.From[ids.WorkspaceKind](o.e.WS)); finished <- err }()
		waitForBackendBlockedBy(t, tx, backendPID(t, tx), finished)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if brain.timesShown(words) != 1 {
		t.Fatalf("model readings=%d, want one", brain.timesShown(words))
	}
	if scans := o.e.WsCount(t, `SELECT count(*) FROM signal_thread_scan WHERE thread_key=$1`, key); scans != 1 {
		t.Fatalf("scan marks=%d, want one", scans)
	}
	o.runEventPass(t, brain)
	if brain.timesShown(words) != 1 {
		t.Fatal("a committed reading was billed again")
	}
}

func TestAReadingWhoseThreadMergedAwayIsDeclinedWithoutAFailedJob(t *testing.T) {
	o := signalEnv(t)
	const oldKey, newKey = "reading-old@customer.example", "reading-new@customer.example"
	o.customerSays(t, oldKey, "", "The old conversation.", "", o.now.Add(-48*time.Hour))
	o.customerSays(t, newKey, "", "The surviving conversation.", "", o.now.Add(-49*time.Hour))
	pass := principal.SystemActing(principal.WithWorkspaceID(context.Background(), o.e.WS), signalScanActor)
	if err := database.WithWorkspaceTx(pass, o.e.Pool, func(tx pgx.Tx) error { return mergeThreadsTx(pass, tx, oldKey, newKey) }); err != nil {
		t.Fatal(err)
	}
	extractor := NewSignalExtractor(o.e.Pool, &askedBrain{}, func() time.Time { return o.now }, slog.Default())
	raised, read, err := extractor.commitReading(pass, settledThread{Key: oldKey}, nil, o.now, true)
	if err != nil || read || raised != 0 {
		t.Fatalf("stale reading: raised=%d read=%v err=%v", raised, read, err)
	}
	if scans := o.e.WsCount(t, `SELECT count(*) FROM signal_thread_scan WHERE thread_key=$1`, oldKey); scans != 0 {
		t.Fatal("retired thread acquired a scan marker")
	}
}
