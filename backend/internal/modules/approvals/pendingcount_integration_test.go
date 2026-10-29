// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ownSeat holds the deal and activity grants over its own rows only, so every
// proposal about a colleague's deal is one it cannot decide.
func ownSeat(ws, user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	full := principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{RowScope: principal.RowScopeOwn, Objects: map[string]principal.ObjectGrant{tableDeal: full, objectActivity: full}},
	})
}

// pagedPending walks every page of the pending inbox, as a client must without
// the count, and keeps the rows the wire still calls pending.
func pagedPending(t *testing.T, svc *Service, ctx context.Context, in ListInput) []crmcontracts.Approval {
	t.Helper()
	status := statusPending
	in.Status, in.Limit = &status, 50
	var out []crmcontracts.Approval
	for range 100 {
		rows, page, err := svc.ListWire(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range rows {
			if a.Status == crmcontracts.ApprovalStatusPending {
				out = append(out, a)
			}
		}
		if !page.HasMore {
			return out
		}
		in.Cursor = page.NextCursor
	}
	t.Fatal("the pending inbox did not finish paging")
	return nil
}

func earliestExpiry(rows []crmcontracts.Approval) *time.Time {
	var first *time.Time
	for _, a := range rows {
		if a.ExpiresAt != nil && (first == nil || a.ExpiresAt.Before(*first)) {
			first = a.ExpiresAt
		}
	}
	return first
}

// For each reader, the count is what a client gets by walking every page. The
// fixture is over three scan batches of pending, lazily expired and rejected
// rows. A third of them sit on a deal the own-scope reader cannot see. A count
// capped at one batch, or one that skips expiry or decidability, fails here.
func TestPendingCountAgreesWithEveryPageOfTheInboxForEachReader(t *testing.T) {
	e := setupStaging(t)
	seeder := newDealSeeder(t, e)
	colleague := e.secondRep(t)
	repDeal := seeder.dealOwnedBy(t, &e.rep)
	colleagueDeal := seeder.dealOwnedBy(t, &colleague)
	staging := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.ws), "background-review")
	allSeat := seatFor(e.ws, e.rep, tableDeal, objectActivity)
	short := time.Hour
	for i := range 3*inboxBatch + 17 {
		in := StageInput{
			Kind: "advance_deal", ProposedChange: []byte(`{"stage":"won"}`),
			DiffHash: fmt.Sprintf("pending-count-%d", i), TargetType: tableDeal, TargetID: repDeal,
			Summary: "Review the proposed deal stage",
		}
		if i%3 == 0 {
			in.TargetID = colleagueDeal
		}
		if i%4 == 1 {
			in.TTL = &short
		}
		id, err := e.svc.Stage(staging, in)
		if err != nil {
			t.Fatal(err)
		}
		if i%5 == 2 {
			if _, err := e.svc.Decide(allSeat, id, false, nil); err != nil {
				t.Fatalf("rejecting a fixture: %v", err)
			}
		}
	}
	later := time.Now().Add(2 * short)
	e.svc.now = func() time.Time { return later }

	readers := map[string]context.Context{"all-scope rep": allSeat, "own-scope colleague": ownSeat(e.ws, colleague)}
	for name, reader := range readers {
		t.Run(name, func(t *testing.T) {
			want := pagedPending(t, e.svc, reader, ListInput{})
			if len(want) == 0 {
				t.Fatal("the fixture left this reader nothing pending, so agreement proves nothing")
			}
			got, err := e.svc.CountPending(reader, ListInput{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Count != len(want) {
				t.Fatalf("count %d, every page holds %d pending", got.Count, len(want))
			}
			first := earliestExpiry(want)
			if got.NextExpiry == nil || first == nil || !got.NextExpiry.Equal(*first) {
				t.Fatalf("next expiry %v, earliest listed %v", got.NextExpiry, first)
			}
			if !got.AsOf.Equal(later) {
				t.Fatalf("as_of %v, service clock %v", got.AsOf, later)
			}
			mine := pagedPending(t, e.svc, reader, ListInput{RequestedForReader: true})
			scoped, err := e.svc.CountPending(reader, ListInput{RequestedForReader: true})
			if err != nil || scoped.Count != len(mine) {
				t.Fatalf("requested-for-reader count %d, pages hold %d: %v", scoped.Count, len(mine), err)
			}
		})
	}
	all, err := e.svc.CountPending(allSeat, ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	if all.Count <= PendingScanCap {
		t.Fatalf("all-scope count %d does not exceed one scan batch (%d), so a capped count would pass", all.Count, PendingScanCap)
	}
	own, err := e.svc.CountPending(readers["own-scope colleague"], ListInput{})
	if err != nil || own.Count >= all.Count {
		t.Fatalf("own-scope count %d is not below all-scope %d, so undecidable rows were counted: %v", own.Count, all.Count, err)
	}
}

func TestPendingCountEndpointAnswersTheEngineCount(t *testing.T) {
	e := setupStaging(t)
	seeder := newDealSeeder(t, e)
	deal := seeder.dealOwnedBy(t, &e.rep)
	staging := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.ws), "background-review")
	for i := range 3 {
		if _, err := e.svc.Stage(staging, StageInput{
			Kind: "advance_deal", ProposedChange: []byte(`{"stage":"won"}`),
			DiffHash: fmt.Sprintf("pending-count-endpoint-%d", i), TargetType: tableDeal, TargetID: deal,
			Summary: "Review the proposed deal stage",
		}); err != nil {
			t.Fatal(err)
		}
	}
	reader := seatFor(e.ws, e.rep, tableDeal, objectActivity)
	request := httptest.NewRequest(http.MethodGet, "/approvals/pending-count", nil).WithContext(reader)
	recorder := httptest.NewRecorder()
	NewHandlers(e.svc).CountPendingApprovals(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var body crmcontracts.ApprovalPendingCount
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Count != 3 || body.NextExpiryAt == nil || body.AsOf.IsZero() {
		t.Fatalf("answered %+v, want three pending with an expiry and a clock", body)
	}
	request = httptest.NewRequest(http.MethodGet, "/approvals/pending-count", nil).WithContext(context.Background())
	recorder = httptest.NewRecorder()
	NewHandlers(e.svc).CountPendingApprovals(recorder, request)
	if recorder.Code == http.StatusOK {
		t.Fatalf("a caller with no principal was answered a count: %s", recorder.Body.String())
	}
}
