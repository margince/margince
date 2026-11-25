// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The list readers over a kind with an owner column count and name only the
// rows inside the reader's row scope. That holds for Shortlist and Live List
// members and totals, and for the pulse and the joined-since-visit marks. It
// holds for the filter preview a record picker pages through too.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// scopedLists are a Shortlist holding every visit and a Live List selecting
// every visit, made and checked by a reader who sees all of them.
type scopedLists struct {
	shortlist, live ids.ListID
}

func makeScopedLists(t *testing.T, e *integration.Env, visits scopedVisits) scopedLists {
	t.Helper()
	store := collections.NewStore(e.DB())
	ctx := scopedReader(e, principal.RowScopeAll)
	short, err := store.CreateList(ctx, collections.CreateListInput{Name: "Visits", EntityType: scopeDemoKind, ListType: "static"})
	if err != nil {
		t.Fatalf("creating the Shortlist: %v", err)
	}
	for title, id := range visits.byTitle {
		if _, err := store.AddMember(ctx, short.ID, collections.MemberChange{EntityType: scopeDemoKind, EntityID: id, Reason: collections.ReasonChosen}); err != nil {
			t.Fatalf("adding %s: %v", title, err)
		}
	}
	live, err := store.CreateList(ctx, collections.CreateListInput{
		Name: "Every visit", EntityType: scopeDemoKind, ListType: "dynamic",
		Definition: map[string]any{"field": "title", "op": "contains", "value": "visit"},
	})
	if err != nil {
		t.Fatalf("creating the Live List: %v", err)
	}
	// The reader's last visit lies far behind the check, so every row the check
	// sees entering counts as joined since it.
	if _, err := store.VisitList(ctx, live.ID); err != nil {
		t.Fatalf("visiting the Live List: %v", err)
	}
	e.WsExec(t, `UPDATE list_visit SET visited_at = '2000-01-01T00:00:00Z' WHERE list_id = $1`, live.ID)
	checks, err := CheckLiveLists(context.Background(), e.Pool, e.WS, func() time.Time { return clocktest.Now(t) })
	if err != nil {
		t.Fatalf("checking the Live List: %v", err)
	}
	for _, check := range checks {
		if check.ListID == live.ID && (check.Err != nil || check.Members != len(visits.byTitle)) {
			t.Fatalf("the check = %+v, want every visit a member", check)
		}
	}
	return scopedLists{shortlist: short.ID, live: live.ID}
}

func TestTheListReadersOfAnOwnedKindFollowTheRowScope(t *testing.T) {
	e := integration.Setup(t)
	visits := registerScopedVisits(t, e, "owner_id")
	lists := makeScopedLists(t, e, visits)
	store := collections.NewStore(e.DB())
	for _, level := range scopeLevels {
		ctx := scopedReader(e, level.scope)
		want := len(level.want)
		for name, list := range map[string]ids.ListID{"Shortlist": lists.shortlist, "Live List": lists.live} {
			members, _, err := store.ListMembers(ctx, list, 50, "")
			if err != nil {
				t.Fatalf("%s members: %v", name, err)
			}
			var found []ids.UUID
			for _, m := range members {
				found = append(found, m.EntityID)
			}
			assertTitles(t, level.scope, name+" members", visits.titlesOf(found), level.want)
			if total, err := store.CountMembers(ctx, list); err != nil || total != want {
				t.Errorf("%s at %s scope: CountMembers = %d, %v, want %d", name, level.scope, total, err, want)
			}
			view, err := store.ListView(ctx, list)
			if err != nil || view.VisibleCount == nil || *view.VisibleCount != want {
				t.Errorf("%s at %s scope: list total = %v, %v, want %d", name, level.scope, view.VisibleCount, err, want)
			}
		}
		view, err := store.ListView(ctx, lists.live)
		if err != nil {
			t.Fatalf("reading the Live List: %v", err)
		}
		if view.SinceLastVisit == nil || view.SinceLastVisit.Entered != want {
			t.Errorf("%s scope: pulse = %+v, want %d entered", level.scope, view.SinceLastVisit, want)
		}
		var joined []ids.UUID
		if view.JoinedSinceVisit != nil {
			for _, id := range *view.JoinedSinceVisit {
				joined = append(joined, ids.UUID(id))
			}
		}
		assertTitles(t, level.scope, "joined since the visit", visits.titlesOf(joined), level.want)
		assertTitles(t, level.scope, "picker preview", previewedVisits(t, e, ctx, visits), level.want)
	}
}

// previewedVisits pages through the filter preview a record picker reads,
// two rows a page.
func previewedVisits(t *testing.T, e *integration.Env, ctx context.Context, visits scopedVisits) []string {
	t.Helper()
	var out []string
	cursor := ""
	for range len(visits.byTitle) + 1 {
		body := `{"resource":"` + scopeDemoKind + `","filter":{"field":"title","op":"contains","value":"visit"},"limit":2`
		if cursor != "" {
			body += `,"cursor":"` + cursor + `"`
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/filters/preview", strings.NewReader(body+`}`)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		filterPreviewHandlers{pool: e.Pool, collections: collections.NewStore(e.DB())}.PreviewFilter(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("preview status = %d: %s", rec.Code, rec.Body)
		}
		var page crmcontracts.FilterPreview
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		for _, row := range page.Rows {
			if title, ok := row["title"].(string); ok && !slices.Contains(out, title) {
				out = append(out, title)
			}
		}
		if page.NextCursor == nil || *page.NextCursor == "" {
			return out
		}
		cursor = *page.NextCursor
	}
	t.Fatal("the preview never reached its last page")
	return nil
}
