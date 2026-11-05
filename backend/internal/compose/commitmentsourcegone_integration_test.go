// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCommitmentWhoseSourceIsArchivedDuringReadingCreatesNothing(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	contact := e.SeedContact(t, "Ines Huber", &e.Rep1)
	e.WsExec(t, `INSERT INTO relationship (kind,contact_id,company_id,source,captured_by) VALUES ('employment',$1,$2,'manual','human:x')`, contact, company)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	message := captureSeatPromise(t, e, contact, "archived-during-read@buyer.example", seatStatusPromise, now.Add(-48*time.Hour))
	brain := erasingBrain{reply: mailedCommitmentReply(t, message, e.Rep2, 0.95), erase: func() {
		if _, err := e.Activities.ArchiveActivity(e.Admin(), ids.From[ids.ActivityKind](message), nil); err != nil {
			t.Fatal(err)
		}
	}}
	pass := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), signalScanActor)
	if _, err := NewSignalExtractor(e.Pool, brain, func() time.Time { return now }, slog.Default()).RunWorkspace(pass, ids.From[ids.WorkspaceKind](e.WS)); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM signal WHERE kind='commitment_made'`); got != 0 {
		t.Fatalf("signals=%d", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task'`); got != 0 {
		t.Fatalf("tasks=%d", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM approval`); got != 0 {
		t.Fatalf("approvals=%d", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM signal_thread_scan`); got != 0 {
		t.Fatalf("a conversation the rule no longer reads was retired: scans=%d", got)
	}
}
