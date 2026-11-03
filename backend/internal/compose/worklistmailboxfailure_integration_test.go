// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A mailbox whose sync fails is its owner's to reconnect. The failure reaches
// the owner's System view with what it costs, and no other seller's view.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
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

// unreachableMailbox is a gmail connector whose provider never answers.
type unreachableMailbox struct{ plainSyncConnector }

func (unreachableMailbox) Sync(context.Context, connector.Auth, connector.Cursor, connector.Sink) (connector.Cursor, error) {
	return nil, connector.ErrUnreachable
}

// failMailboxSync connects Rep1's mailbox and runs one sync that fails, through
// the capture registry the sync worker uses.
func failMailboxSync(t *testing.T, e *integration.Env) {
	t.Helper()
	registry := capture.NewRegistry(e.DB(), capture.NewSink(e.DB()), backfillAuthority{}, keyvault.NewMemory())
	registry.Register(unreachableMailbox{plainSyncConnector{name: "gmail"}})
	human := principal.WithWorkspaceID(context.Background(), e.WS)
	human = principal.WithCorrelationID(human, ids.NewV7())
	human = principal.WithActor(human, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		TeamIDs: []ids.UUID{e.Team1}, SeatType: principal.SeatFull, Scopes: principal.NewScopeSet(principal.ScopeRead),
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{"activity": {Create: true, Read: true}}, RowScope: principal.RowScopeTeam,
		},
	})
	connection, err := registry.Connect(human, "gmail", connector.Auth("refresh"))
	if err != nil {
		t.Fatalf("connecting the mailbox: %v", err)
	}
	if err := registry.SyncOnce(human, connection); !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("the failing sync answered %v, want unreachable", err)
	}
}

func TestAFailedMailboxSyncReachesItsOwnersSystemViewAndNoOtherSellers(t *testing.T) {
	e := integration.Setup(t)
	failMailboxSync(t, e)
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
