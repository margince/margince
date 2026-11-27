// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A grouped search runs each record type as its own statement under its own
// savepoint. A type the time budget stops is named on the page, and the other
// types answer. An exclusive lock on a table holds its statement until the
// budget cancels it. That gives the SQLSTATE a slow plan gets, on any corpus
// size and any machine.

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
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// searchLaneBudget is the lane budget of the tests whose slow lane waits on a
// held lock, so that lane times out whatever the budget. It leaves the other
// lanes room to answer on a loaded runner. inTimeLaneBudget is for a test where
// every lane must answer.
const (
	searchLaneBudget = 2 * time.Second
	inTimeLaneBudget = 10 * time.Second
)

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

// seedOneWordEverywhere writes a company, a contact, a lead and a note that
// all carry the word.
func seedOneWordEverywhere(t *testing.T, e *SearchEnv) {
	t.Helper()
	for _, statement := range []string{
		`INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Quillfeather Werke', 'manual', 'human:x')`,
		`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Quillfeather Mara', 'manual', 'human:x')`,
		`INSERT INTO lead (id, company_name, email, source, captured_by) VALUES ($1, 'Quillfeather Freight', 'lead@quillfeather.test', 'manual', 'human:x')`,
		`INSERT INTO activity (id, kind, subject, body, source, captured_by) VALUES ($1, 'note', 'Quillfeather visit', 'Quillfeather', 'manual', 'human:x')`,
	} {
		e.SeedID(t, statement)
	}
}

func boundedStore(e *SearchEnv) *search.Store {
	return boundedStoreWithin(e, searchLaneBudget)
}

func boundedStoreWithin(e *SearchEnv, budget time.Duration) *search.Store {
	return search.NewStore(database.BindTo(e.Pool, ids.From[ids.WorkspaceKind](e.WS)).Bounded(budget))
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
	t.Parallel()
	e := SetupSearch(t)
	seedOneWordEverywhere(t, e)
	lockTable(t, e.Owner, "activity")
	perType := 5

	page, err := boundedStore(e).Search(e.Admin(), search.Input{Query: "quillfeather", PerType: &perType})
	if err != nil {
		t.Fatalf("a grouped search failed whole for one slow type: %v", err)
	}
	if want := []string{"company", "contact", "lead"}; !slices.Equal(typesOf(page), want) {
		t.Errorf("the types that answered are %v, want %v", typesOf(page), want)
	}
	if !slices.Equal(page.TypesTimedOut, []string{"activity"}) {
		t.Errorf("types_timed_out = %v, want [activity]", page.TypesTimedOut)
	}
}

func TestAGroupedSearchWithTwoSlowTypesNamesBothInBranchOrder(t *testing.T) {
	t.Parallel()
	e := SetupSearch(t)
	seedOneWordEverywhere(t, e)
	lockTable(t, e.Owner, "lead")
	lockTable(t, e.Owner, "activity")
	perType := 5

	page, err := boundedStore(e).Search(e.Admin(), search.Input{Query: "quillfeather", PerType: &perType})
	if err != nil {
		t.Fatal(err)
	}
	// The lead lane spends the whole budget, so the lanes after it have none
	// left and are named without sending a statement.
	if want := []string{"lead", "product", "offer_template", "activity"}; !slices.Equal(page.TypesTimedOut, want) {
		t.Errorf("types_timed_out = %v, want %v", page.TypesTimedOut, want)
	}
	if want := []string{"company", "contact"}; !slices.Equal(typesOf(page), want) {
		t.Errorf("the types that answered are %v, want %v", typesOf(page), want)
	}
}

func TestAGroupedSearchFailsWhenATypeFailsForAnotherReasonThanTime(t *testing.T) {
	t.Parallel()
	e := SetupSearch(t)
	seedOneWordEverywhere(t, e)
	ctx := context.Background()
	if _, err := e.Owner.Exec(ctx, `REVOKE SELECT ON lead FROM margince_app`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := e.Owner.Exec(ctx, `GRANT SELECT ON lead TO margince_app`); err != nil {
			t.Errorf("restoring the lead grant: %v", err)
		}
	})
	perType := 5

	page, err := boundedStore(e).Search(e.Admin(), search.Input{Query: "quillfeather", PerType: &perType})
	if err == nil {
		t.Fatalf("a type that failed on a missing grant was left out of the page: %+v", page)
	}
	if _, ok := errors.AsType[*search.QueryTooBroadError](err); ok {
		t.Errorf("a permission fault was reported as a time budget: %v", err)
	}
}

func TestAnAllDeniedGroupedSearchAnswersAtOnceAndReadsNothing(t *testing.T) {
	t.Parallel()
	e := SetupSearch(t)
	seedOneWordEverywhere(t, e)
	lockTable(t, e.Owner, "contact")
	ctx, report := auth.WithWithheldReport(principal.WithWorkspaceID(context.Background(), e.WS))
	companyOnly := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"company": {Read: true}, "installation_settings": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	perType := 5

	page, err := boundedStore(e).Search(companyOnly, search.Input{Query: "quillfeather", Types: []string{"contact"}, PerType: &perType})
	if err != nil {
		t.Fatalf("a grouped search of a denied type read the locked contact table: %v", err)
	}
	if len(page.Hits) != 0 || len(page.TypesTimedOut) != 0 || page.TypesWithMore == nil {
		t.Errorf("page = %+v, want an empty grouped page", page)
	}
	if report.Withheld() {
		t.Error("a denied search reported a capture-privacy withholding")
	}
}

func TestAGroupedSearchWithEveryTypeInTimeNamesNoType(t *testing.T) {
	t.Parallel()
	e := SetupSearch(t)
	seedOneWordEverywhere(t, e)
	perType := 5

	page, err := boundedStoreWithin(e, inTimeLaneBudget).Search(e.Admin(), search.Input{Query: "quillfeather", PerType: &perType})
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
	t.Parallel()
	e := SetupSearch(t)
	seedOneWordEverywhere(t, e)
	lockTable(t, e.Owner, "activity")

	_, err := boundedStore(e).Search(e.Admin(), search.Input{Query: "quillfeather"})
	if _, ok := errors.AsType[*search.QueryTooBroadError](err); !ok {
		t.Fatalf("a ranked search answered %v, want a QueryTooBroadError", err)
	}
}

func TestAGroupedSearchOfOnlyMailThatRunsOutOfTimeAnswersAnEmptyPage(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
