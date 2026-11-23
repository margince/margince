// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A grouped search runs each record type as its own statement under its own
// savepoint, so a type the time budget stops is named on the page and the
// other types answer. The slow type is made slow by an exclusive lock on its
// table, which holds its statement until the budget cancels it, the same
// SQLSTATE a slow plan gets, on any corpus size and any machine.

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const searchLaneBudget = 300 * time.Millisecond

// lockTable holds table in ACCESS EXCLUSIVE mode on owner until the test ends.
func lockTable(t *testing.T, owner *pgx.Conn, table string) {
	t.Helper()
	ctx := context.Background()
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("releasing the %s lock: %v", table, err)
		}
	})
	if _, err := tx.Exec(ctx, `LOCK TABLE `+pgx.Identifier{table}.Sanitize()+` IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
}

// seedOneWordEverywhere writes a company, a contact and a mail-timeline note
// that all carry the word, and answers their ids by type.
func seedOneWordEverywhere(t *testing.T, e *SearchEnv) map[string]ids.UUID {
	t.Helper()
	return map[string]ids.UUID{
		"company":  e.SeedID(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Quillfeather Werke', 'manual', 'human:x')`),
		"contact":  e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Quillfeather Mara', 'manual', 'human:x')`),
		"lead":     e.SeedID(t, `INSERT INTO lead (id, company_name, email, source, captured_by) VALUES ($1, 'Quillfeather Freight', 'lead@quillfeather.test', 'manual', 'human:x')`),
		"activity": e.SeedID(t, `INSERT INTO activity (id, kind, subject, body, source, captured_by) VALUES ($1, 'note', 'Quillfeather visit', 'Quillfeather', 'manual', 'human:x')`),
	}
}

func boundedStore(e *SearchEnv) *search.Store {
	return search.NewStore(database.BindTo(e.Pool, ids.From[ids.WorkspaceKind](e.WS)).Bounded(searchLaneBudget))
}

func typesOf(page search.Page) []string {
	var types []string
	for _, hit := range page.Hits {
		if !slices.Contains(types, hit.Type) {
			types = append(types, hit.Type)
		}
	}
	slices.Sort(types)
	return types
}

func TestAGroupedSearchShowsTheTypesThatAnsweredWhenMailRunsOutOfTime(t *testing.T) {
	e := SetupSearch(t)
	seedOneWordEverywhere(t, e)
	lockTable(t, e.Owner, "activity")
	perType := 5

	started := time.Now()
	page, err := boundedStore(e).Search(e.Admin(), search.Input{Query: "quillfeather", PerType: &perType})
	if err != nil {
		t.Fatalf("a grouped search failed whole for one slow type: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*searchLaneBudget {
		t.Errorf("the page took %s, over twice its %s budget", elapsed, searchLaneBudget)
	}
	if want := []string{"company", "contact", "lead"}; !slices.Equal(typesOf(page), want) {
		t.Errorf("the types that answered are %v, want %v", typesOf(page), want)
	}
	if !slices.Equal(page.TypesTimedOut, []string{"activity"}) {
		t.Errorf("types_timed_out = %v, want [activity]", page.TypesTimedOut)
	}
}

func TestAGroupedSearchWithEveryTypeInTimeNamesNoType(t *testing.T) {
	e := SetupSearch(t)
	seedOneWordEverywhere(t, e)
	perType := 5

	page, err := boundedStore(e).Search(e.Admin(), search.Input{Query: "quillfeather", PerType: &perType})
	if err != nil {
		t.Fatal(err)
	}
	if page.TypesTimedOut == nil || len(page.TypesTimedOut) != 0 {
		t.Errorf("types_timed_out = %#v, want non-nil and empty", page.TypesTimedOut)
	}
	if want := []string{"activity", "company", "contact", "lead"}; !slices.Equal(typesOf(page), want) {
		t.Errorf("the types that answered are %v, want %v", typesOf(page), want)
	}
}

func TestARankedSearchStillRefusesWhenMailRunsOutOfTime(t *testing.T) {
	e := SetupSearch(t)
	seedOneWordEverywhere(t, e)
	lockTable(t, e.Owner, "activity")

	_, err := boundedStore(e).Search(e.Admin(), search.Input{Query: "quillfeather"})
	var tooBroad *search.QueryTooBroadError
	if !errors.As(err, &tooBroad) {
		t.Fatalf("a ranked search answered %v, want a QueryTooBroadError", err)
	}
}

func TestAGroupedSearchOfOnlyMailThatRunsOutOfTimeAnswersAnEmptyPage(t *testing.T) {
	e := SetupSearch(t)
	seedOneWordEverywhere(t, e)
	lockTable(t, e.Owner, "activity")
	perType := 5

	page, err := boundedStore(e).Search(e.Admin(), search.Input{Query: "quillfeather", Types: []string{"activity"}, PerType: &perType})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 0 || !slices.Equal(page.TypesTimedOut, []string{"activity"}) {
		t.Errorf("page = %+v, want no hits and types_timed_out [activity]", page)
	}
}

func TestBothSearchDoorsNameATypeThatRanOutOfTime(t *testing.T) {
	d := newTwoDoors(t, "search-timed-out-doors", "read")
	company := createdRecord(t, d.e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Quillfeather Werke"})
	createdRecord(t, d.e, "/v1/activities", AnyMap{"kind": "task", "subject": "Quillfeather follow-up"})
	lockTable(t, d.e.Owner, "activity")

	var rest, tool json.RawMessage
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); rest = d.rest(t, "GET", "/v1/search?q=Quillfeather&per_type=5", nil) }()
	go func() {
		defer wg.Done()
		tool = d.tool(t, "search_records", map[string]any{"q": "Quillfeather", "record_types": []string{"company", "activity"}, "per_type": 5})
	}()
	wg.Wait()

	route := decodedAs[struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		TypesTimedOut []string `json:"types_timed_out"`
	}](t, rest)
	if len(route.Data) != 1 || route.Data[0].ID != company || !slices.Equal(route.TypesTimedOut, []string{"activity"}) {
		t.Errorf("the route answered %s, want the company alone and types_timed_out [activity]", rest)
	}
	answer := decodedAs[struct {
		Records []struct {
			ID string `json:"id"`
		} `json:"records"`
		TypesTimedOut []string `json:"types_timed_out"`
		Coverage      string   `json:"coverage"`
		Notes         []struct {
			Code string `json:"code"`
		} `json:"notes"`
	}](t, tool)
	if len(answer.Records) != 1 || answer.Records[0].ID != company || !slices.Equal(answer.TypesTimedOut, []string{"activity"}) ||
		answer.Coverage != "partial_degraded" || len(answer.Notes) != 1 || answer.Notes[0].Code != "types_timed_out" {
		t.Errorf("the tool answered %s, want the company, types_timed_out [activity], partial_degraded and one types_timed_out note", tool)
	}
}
