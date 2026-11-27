// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package sharedmailbox

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A shared mailbox has no owner to show a skipped span to. Its admins see it on
// the mailbox itself, beside the history import that recovers it. The health
// count names it too.
func TestASharedMailboxShowsItsUnreadSpanToItsAdmins(t *testing.T) {
	t.Parallel()
	s := newSharedMailboxSuite(t)
	e := s.e
	box := s.connect(t, e.Team1)
	personal := s.connectRepMailbox(t)
	from := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	to := from.Add(72 * time.Hour)
	for _, id := range []any{box.Id, personal} {
		e.WsExec(t, `UPDATE capture_connection SET unread_from = $2, unread_to = $3 WHERE id = $1`, id, from, to)
	}

	rec := s.call(s.admin(), t, http.MethodGet, "/shared-mailboxes", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("the list answered %d: %s", rec.Code, rec.Body)
	}
	var list crmcontracts.SharedMailboxList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].UnreadPeriod == nil {
		t.Fatalf("the list shows %+v, want the one shared mailbox with its unread span", list.Items)
	}
	if got := list.Items[0].UnreadPeriod; !got.From.Equal(from) || !got.To.Equal(to) {
		t.Errorf("the span reads %s to %s, want %s to %s", got.From, got.To, from, to)
	}

	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	var categories capture.OperationalCategories
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		categories, err = capture.ReadOperationalCategoriesTx(ctx, tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if categories.UnreadMailboxes != 2 {
		t.Errorf("the health count names %d mailboxes with an unread span, want the personal and the shared one", categories.UnreadMailboxes)
	}
}
