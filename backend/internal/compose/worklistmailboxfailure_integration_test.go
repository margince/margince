// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A mailbox whose sync fails is its owner's to reconnect. The failure reaches
// the owner's System view with what it costs, and no other seller's view.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func captureRows(ctx context.Context, t *testing.T, e *integration.Env, scope, view string) []crmcontracts.WorklistItem {
	t.Helper()
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return laneDay })
	day, err := feed.WorklistView(ctx, view, scope, "", ids.Nil, 25, "")
	if err != nil {
		t.Fatalf("reading the %s %s view: %v", scope, view, err)
	}
	var rows []crmcontracts.WorklistItem
	for _, row := range day.Queue {
		if row.Source == crmcontracts.WorklistItemSourceCaptureHealth {
			rows = append(rows, row)
		}
	}
	return rows
}

func TestAFailedMailboxSyncReachesItsOwnersSystemViewAndNoOtherSellers(t *testing.T) {
	e := integration.Setup(t)
	seedCalendar(t, e, "gmail", "connected", true)
	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)
	colleague := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	rows := captureRows(owner, t, e, "mine", "system")
	if len(rows) != 1 {
		t.Fatalf("the mailbox owner's System view holds %d capture rows, want the failing mailbox", len(rows))
	}
	got, kind := rows[0], ""
	if rows[0].Kind != nil {
		kind = *rows[0].Kind
	}
	if got.Category != crmcontracts.WorklistItemCategorySystem || got.Consequence != "mailbox_blind" || kind != "sync_failing" {
		t.Errorf("the row reads category %q, consequence %q, kind %q; want system, mailbox_blind, sync_failing",
			got.Category, got.Consequence, kind)
	}
	if today := captureRows(owner, t, e, "mine", "today"); len(today) != 0 {
		t.Errorf("the owner's Today view also holds %d capture rows; the System view owns them", len(today))
	}
	for _, scope := range []string{"mine", "team"} {
		if theirs := captureRows(colleague, t, e, scope, "system"); len(theirs) != 0 {
			t.Errorf("a teammate's %s System view holds the owner's mailbox failure", scope)
		}
	}
	if all := captureRows(e.Admin(), t, e, "all", "system"); len(all) != 0 {
		t.Error("an admin's whole-workspace System view holds another seller's mailbox failure")
	}
}
