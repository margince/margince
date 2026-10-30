// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"errors"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func withDueGroup(group crmcontracts.AttentionItemDueGroup) func(*crmcontracts.AttentionItem) {
	return func(item *crmcontracts.AttentionItem) { item.DueGroup = &group }
}

// viewsDay holds one row of each view and one task due tomorrow.
func viewsDay() crmcontracts.Attention {
	tomorrow := rankInstant.Add(30 * time.Hour)
	return crmcontracts.Attention{
		AsOf: rankInstant,
		Planned: []crmcontracts.AttentionItem{
			item("due-today", "task", withDue(rankInstant.Add(time.Hour)),
				withDueGroup(crmcontracts.AttentionItemDueGroupToday)),
			item("due-tomorrow", "task", withDue(tomorrow),
				withDueGroup(crmcontracts.AttentionItemDueGroupTomorrow)),
		},
		NeedsYou: []crmcontracts.AttentionItem{item("decision", "approval", withKind("send_email"), decidable())},
		Bounces:  &[]crmcontracts.AttentionItem{item("bounce", "bounce", withOccurred(rankInstant.Add(-time.Hour)))},
	}
}

func queueIDs(page crmcontracts.Worklist) []string {
	out := make([]string, 0, len(page.Queue))
	for _, row := range page.Queue {
		out = append(out, row.Id)
	}
	return out
}

func TestAViewKeepsOnlyItsOwnRows(t *testing.T) {
	for _, tc := range []struct {
		view, filter string
		want         []string
	}{
		{viewToday, "", []string{"due-today"}},
		{viewDecisions, "", []string{"decision"}},
		{viewSystem, "", []string{"bounce"}},
		{viewToday, string(crmcontracts.WorklistFilterUpcoming), []string{"due-tomorrow"}},
	} {
		t.Run(tc.view+"/"+tc.filter, func(t *testing.T) {
			svc := &Service{view: tc.view}
			page := svc.worklistFrom(t.Context(), viewsDay(), scopeAll, tc.filter, 25,
				waitingRead{}, worklistCursor{}, nil)
			if got := strings.Join(queueIDs(page), ","); got != strings.Join(tc.want, ",") {
				t.Errorf("the %s view holds %q, want %q", tc.view, got, tc.want)
			}
		})
	}
}

func TestUpcomingWorkStaysOutOfTheQueueAndTheTotal(t *testing.T) {
	page := (&Service{}).worklistFrom(t.Context(), viewsDay(), scopeAll, "", 25,
		waitingRead{}, worklistCursor{}, nil)
	for _, row := range page.Queue {
		if row.Category == categoryUpcoming {
			t.Errorf("upcoming row %s sits in a queue that did not ask for it", row.Id)
		}
	}
	if page.Summary.Total != 3 {
		t.Errorf("the summary counts %d items, want the 3 due now without the upcoming task", page.Summary.Total)
	}
	counted := false
	for _, count := range page.Counts {
		if count.Category == crmcontracts.WorklistCountCategory(categoryUpcoming) {
			counted = count.Considered == 1
		}
	}
	if !counted {
		t.Errorf("counts carry no upcoming category with the one task due tomorrow: %+v", page.Counts)
	}
}

func TestTodaysSummaryCountsOnlyTodaysWork(t *testing.T) {
	page := (&Service{view: viewToday}).worklistFrom(t.Context(), viewsDay(), scopeAll, "", 25,
		waitingRead{}, worklistCursor{}, nil)
	if page.Summary.Total != 1 {
		t.Errorf("the Today summary counts %d items, want only the task due today", page.Summary.Total)
	}
}

func TestADecisionsOrSystemFilterOpensItsView(t *testing.T) {
	for filter, want := range map[string]string{
		"decisions": viewDecisions, "system": viewSystem, "": "", "all": "", "tasks": "",
	} {
		got, err := resolveView("", filter)
		if err != nil || got != want {
			t.Errorf("filter %q without a view resolved to %q (err %v), want %q", filter, got, err, want)
		}
	}
}

func TestAFilterFromAnotherViewIsRefusedNamingItsView(t *testing.T) {
	for _, tc := range []struct{ view, filter, names string }{
		{viewDecisions, "tasks", "Today"},
		{viewToday, "decisions", "decisions"},
		{viewSystem, "upcoming", "today"},
		{viewToday, "system", "system"},
	} {
		_, err := resolveView(tc.view, tc.filter)
		detailed, ok := errors.AsType[*httperr.DetailedError](err)
		if !ok {
			t.Fatalf("view %s with filter %s answered %v, want a 422", tc.view, tc.filter, err)
		}
		if !strings.Contains(strings.ToLower(detailed.Error()), strings.ToLower(tc.names)) {
			t.Errorf("the refusal %q does not name the %s view", detailed.Error(), tc.names)
		}
	}
	for _, ok := range []struct{ view, filter string }{
		{viewToday, "tasks"},
		{viewToday, "urgent"},
		{viewToday, "upcoming"},
		{viewDecisions, "decisions"},
		{viewSystem, "all"},
	} {
		if _, err := resolveView(ok.view, ok.filter); err != nil {
			t.Errorf("view %s refused its own filter %s: %v", ok.view, ok.filter, err)
		}
	}
}

func TestTheEntryPointRefusesAFilterOutsideItsView(t *testing.T) {
	_, err := (&Service{}).WorklistView(t.Context(), viewDecisions, scopeMine, "tasks", ids.Nil, 25, "")
	if _, ok := errors.AsType[*httperr.DetailedError](err); !ok {
		t.Fatalf("a Decisions read filtered to tasks answered %v, want a 422 before any read", err)
	}
}

func TestACursorFromOneViewIsRefusedInAnother(t *testing.T) {
	minted := encodeCursor(1, scopeMine, askedFor(viewToday, ""), ids.Nil, ids.Nil)
	_, err := decodeCursor(minted, scopeMine, askedFor(viewDecisions, ""), ids.Nil)
	if _, ok := errors.AsType[*storekit.CursorSortMismatchError](err); !ok {
		t.Errorf("a Today cursor continued into Decisions (err=%v)", err)
	}
	if _, err := decodeCursor(minted, scopeMine, askedFor("", ""), ids.Nil); err == nil {
		t.Error("a Today cursor continued into the read with no view")
	}
}

func TestAnAliasSharesItsViewsCursor(t *testing.T) {
	view, err := resolveView("", "decisions")
	if err != nil {
		t.Fatal(err)
	}
	minted := encodeCursor(1, scopeMine, askedFor(view, "decisions"), ids.Nil, ids.Nil)
	if _, err := decodeCursor(minted, scopeMine, askedFor(viewDecisions, ""), ids.Nil); err != nil {
		t.Errorf("the filter alias and the view it opens are one walk, but the cursor was refused: %v", err)
	}
}

func TestAWalkThroughAViewCarriesItsViewToTheNextPage(t *testing.T) {
	svc := &Service{view: viewToday}
	page := svc.worklistFrom(t.Context(), tasksDay(4), scopeAll, "", 2, waitingRead{}, worklistCursor{}, nil)
	if page.NextCursor == nil {
		t.Fatal("a four-row Today view paged by two offered no cursor")
	}
	if page.View == nil || *page.View != crmcontracts.WorklistViewToday {
		t.Errorf("the page echoed view %v, want today", page.View)
	}
	if _, err := decodeCursor(*page.NextCursor, scopeAll, askedFor(viewDecisions, ""), ids.Nil); err == nil {
		t.Error("the minted Today cursor continued into Decisions")
	}
	if _, err := decodeCursor(*page.NextCursor, scopeAll, askedFor(viewToday, ""), ids.Nil); err != nil {
		t.Errorf("the minted Today cursor was refused in Today: %v", err)
	}
}
