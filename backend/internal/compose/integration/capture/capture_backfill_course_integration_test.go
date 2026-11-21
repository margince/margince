// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A retried page and a restarted scan both lower what the screen would show,
// for different reasons. The status read carries which one happened, so a
// client can say it instead of showing a count that fell for no stated cause.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestARetriedPageKeepsTheCommittedCountAndSaysItIsRetrying(t *testing.T) {
	e := integration.SetupSearch(t)
	prov := &reportingConnector{pagedConnector: &pagedConnector{messages: 20, pageSize: 10}}
	registry, grantCtx, runID := startReportingBackfill(t, e, prov, 0)
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	mailbox := capturemod.ProviderMailbox("gmail")
	rep := ids.From[ids.UserKind](e.Rep1)

	if _, _, _, err := registry.RunBackfillStep(wsCtx, runID); err != nil {
		t.Fatalf("page 1: %v", err)
	}
	prov.failAfter = 4
	if _, _, _, err := registry.RunBackfillStep(wsCtx, runID); err == nil {
		t.Fatal("a rate-limited page must surface its fault")
	}

	run, err := registry.BackfillStatus(grantCtx, mailbox, rep)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if run.Scanned != 10 || run.Captured != 10 {
		t.Errorf("after a retried page the count = %d/%d, want the committed 10/10 kept", run.Scanned, run.Captured)
	}
	if run.RetryAttempts != 1 || run.WindowRestarts != 0 {
		t.Errorf("retry_attempts = %d, window_restarts = %d, want 1 and 0", run.RetryAttempts, run.WindowRestarts)
	}

	prov.failAfter = 0
	if _, _, _, err := registry.RunBackfillStep(wsCtx, runID); err != nil {
		t.Fatalf("retried page: %v", err)
	}
	run, err = registry.BackfillStatus(grantCtx, mailbox, rep)
	if err != nil {
		t.Fatalf("status after the retry: %v", err)
	}
	if run.Scanned != 20 || run.RetryAttempts != 0 {
		t.Errorf("after the retry landed scanned = %d, retry_attempts = %d, want 20 and 0", run.Scanned, run.RetryAttempts)
	}
}

func TestARestartedScanIsReportedAsARestartNotAsLostWork(t *testing.T) {
	e := integration.SetupSearch(t)
	prov := &reportingConnector{pagedConnector: &pagedConnector{messages: 20, pageSize: 10}}
	registry, grantCtx, runID := startReportingBackfill(t, e, prov, 0)
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	mailbox := capturemod.ProviderMailbox("gmail")
	rep := ids.From[ids.UserKind](e.Rep1)

	if _, _, _, err := registry.RunBackfillStep(wsCtx, runID); err != nil {
		t.Fatalf("page 1: %v", err)
	}
	prov.tokenGone = true
	done, _, _, _ := registry.RunBackfillStep(wsCtx, runID)
	if done {
		t.Fatal("an expired page token restarts the scan once; the run stays live")
	}

	run, err := registry.BackfillStatus(grantCtx, mailbox, rep)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if run.Scanned != 0 || run.WindowRestarts != 1 || run.RetryAttempts == 0 {
		t.Errorf("after the restart scanned = %d, window_restarts = %d, retry_attempts = %d; want 0, 1 and a retry pending",
			run.Scanned, run.WindowRestarts, run.RetryAttempts)
	}

	prov.tokenGone = false
	if _, _, _, err := registry.RunBackfillStep(wsCtx, runID); err != nil {
		t.Fatalf("first page of the new walk: %v", err)
	}
	run, err = registry.BackfillStatus(grantCtx, mailbox, rep)
	if err != nil {
		t.Fatalf("status after the new walk began: %v", err)
	}
	if run.Scanned != 10 || run.WindowRestarts != 1 {
		t.Errorf("after the new walk's first page scanned = %d, window_restarts = %d, want 10 and the restart still reported", run.Scanned, run.WindowRestarts)
	}
}
