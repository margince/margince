// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Done on a deal at risk, through the attention service the server builds.
// The row must be on the reader's day, and Undo puts it back.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestADealAtRiskMarkedDoneLeavesTheReadersDayOnly(t *testing.T) {
	e := setupVerdicts(t)
	at := clocktest.Now(t)
	svc := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at })
	// Every grant the lanes ask for, at team scope, so the at-risk lane answers.
	perms := integration.AdminPerms
	perms.RowScope = principal.RowScopeTeam
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	lead := e.As(e.Rep2, []ids.UUID{e.Team1}, perms)
	rowOf := func(t *testing.T, ctx context.Context, deal ids.UUID) *crmcontracts.WorklistItem {
		t.Helper()
		day, err := svc.Worklist(ctx, "team", "", ids.UUID{}, 100, "")
		if err != nil {
			t.Fatalf("reading the worklist: %v", err)
		}
		for _, row := range day.Queue {
			if row.Source == "deal_at_risk" && row.Id == deal.String() {
				return &row
			}
		}
		return nil
	}
	shows := func(t *testing.T, reader ids.UUID, deal ids.UUID) bool {
		t.Helper()
		ctx := rep
		if reader == e.Rep2 {
			ctx = lead
		}
		return rowOf(t, ctx, deal) != nil
	}
	if !shows(t, e.Rep1, e.booked) || !shows(t, e.Rep2, e.booked) {
		t.Fatal("the deal is not at risk on both pages before Done, so this proves nothing")
	}
	row := attention.RowRef{Source: "deal_at_risk", RowID: e.booked.String()}

	if err := svc.MarkRowsDone(rep, []attention.RowRef{row}); err != nil {
		t.Fatalf("marking the deal done: %v", err)
	}
	if shows(t, e.Rep1, e.booked) {
		t.Error("the deal is still on the page of the rep who marked it done")
	}
	if !shows(t, e.Rep2, e.booked) {
		t.Error("one rep's Done took the deal off a colleague's page")
	}

	unseen := attention.RowRef{Source: "deal_at_risk", RowID: e.other.String()}
	if err := svc.MarkRowsDone(rep, []attention.RowRef{unseen}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("Done on a deal the rep is not shown answered %v, want not found", err)
	}

	if err := svc.ReopenRows(rep, []attention.RowRef{row}); err != nil {
		t.Fatalf("undoing: %v", err)
	}
	if !shows(t, e.Rep1, e.booked) {
		t.Error("Undo did not put the deal back")
	}

	// The row draws the deal page's move, so "Not now" there takes it off here.
	cards := newDealStatusService(e.Pool, func() time.Time { return at })
	card, err := cards.ReadFacts(rep, ids.From[ids.DealKind](e.booked))
	if err != nil || card.Next == nil || card.Next.Fingerprint == nil {
		t.Fatalf("the deal card names no move to set aside (%v), so this proves nothing", err)
	}
	if row := rowOf(t, rep, e.booked); row == nil || row.Move == nil {
		t.Fatal("the Worklist row draws no move before Not now, so this proves nothing")
	}
	body, err := json.Marshal(crmcontracts.DismissDealMoveRequest{Fingerprint: *card.Next.Fingerprint})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	path := "/v1/deals/" + e.booked.String() + "/move/dismiss"
	dealstatus.NewHandlers(cards).DismissDealMove(rec,
		httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)).WithContext(rep),
		crmcontracts.Id(e.booked))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST %s answered %d: %s", path, rec.Code, rec.Body.String())
	}
	if row := rowOf(t, rep, e.booked); row == nil || row.Move != nil {
		t.Errorf("after Not now on the deal page the Worklist row is %+v, want it with no move", row)
	}
}
