// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A kind that names an owner column is read under the role's row scope, as a
// deal is. Every core reader applies it: search, the row check, a Live List's
// engine, a report count and a contact's History. A row with no owner is
// everyone's. A kind without an owner column stays all or nothing.

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

const (
	scopeDemoTable  = "ext_scopedemo_visit"
	scopeDemoKind   = "ext:scopedemo:visit"
	scopeDemoObject = "ext_scopedemo_visit"
)

// scopedVisits are one visit per owner: Rep1, Rep2 on Rep1's team, Rep3 on
// another team, and nobody. Each title names its owner.
type scopedVisits struct {
	contact ids.UUID
	byTitle map[string]ids.UUID
}

func registerScopedVisits(t *testing.T, e *integration.Env, ownerColumn string) scopedVisits {
	t.Helper()
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE SCHEMA IF NOT EXISTS ext`,
		`GRANT USAGE ON SCHEMA ext TO margince_app`,
		`DROP TABLE IF EXISTS ext.` + scopeDemoTable,
		`CREATE TABLE ext.` + scopeDemoTable + ` (id uuid PRIMARY KEY DEFAULT uuidv7(), archived_at timestamptz,
			title text NOT NULL UNIQUE, owner_id uuid, contact_id uuid, visited_at timestamptz NOT NULL DEFAULT now())`,
		`CREATE INDEX ON ext.` + scopeDemoTable + ` (owner_id)`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ext.` + scopeDemoTable + ` TO margince_app`,
	} {
		if _, err := owner.Exec(ctx, stmt); err != nil {
			t.Fatalf("preparing the visit table (%s): %v", stmt, err)
		}
	}
	unit := extension.Name("scopedemo")
	kinds := []extension.RecordKindDeclaration{{
		Kind: "visit", DisplayName: "Visit", Table: scopeDemoTable,
		Fields:     []extension.RecordKindField{{Name: "title", Label: "Title"}},
		NaturalKey: []string{"title"}, OwnerColumn: ownerColumn,
		Timeline: &extension.RecordKindTimeline{
			LabelField: "title", AtColumn: "visited_at",
			Subjects: []extension.TimelineSubject{{Entity: "contact", Column: "contact_id"}},
		},
	}}
	recordkinds.Register(unit, kinds)
	registerReports([]extension.Extension{{
		Name: unit, RecordKinds: kinds,
		ReportCounts: []extension.ReportCount{{Name: "visits", Label: "Visits", Population: "visit"}},
	}})
	t.Cleanup(func() {
		recordkinds.Reset()
		registerReports(nil)
		if _, err := owner.Exec(context.Background(), `DROP TABLE IF EXISTS ext.`+scopeDemoTable); err != nil {
			t.Errorf("dropping the visit table: %v", err)
		}
	})
	visits := scopedVisits{contact: e.SeedContact(t, "Visited Person", nil), byTitle: map[string]ids.UUID{}}
	for title, who := range map[string]*ids.UUID{"rep1 visit": &e.Rep1, "rep2 visit": &e.Rep2, "rep3 visit": &e.Rep3, "unowned visit": nil} {
		var id ids.UUID
		if err := owner.QueryRow(ctx, `INSERT INTO ext.`+scopeDemoTable+` (title, owner_id, contact_id) VALUES ($1, $2, $3) RETURNING id`,
			title, who, visits.contact).Scan(&id); err != nil {
			t.Fatalf("seeding %s: %v", title, err)
		}
		visits.byTitle[title] = id
	}
	return visits
}

func scopedReader(e *integration.Env, scope principal.RowScope) context.Context {
	return e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RowScope: scope,
		Objects: map[string]principal.ObjectGrant{
			scopeDemoObject: {Read: true}, "contact": {Read: true}, "signal": {Read: true},
			"list": {Create: true, Read: true, Update: true, Delete: true},
		},
	})
}

// scopeLevels are the role levels and the visits each one reads.
var scopeLevels = []struct {
	scope principal.RowScope
	want  []string
}{
	{principal.RowScopeOwn, []string{"rep1 visit", "unowned visit"}},
	{principal.RowScopeTeam, []string{"rep1 visit", "rep2 visit", "unowned visit"}},
	{principal.RowScopeAll, []string{"rep1 visit", "rep2 visit", "rep3 visit", "unowned visit"}},
}

func TestAnOwnedExtensionKindIsReadUnderTheRoleRowScopeEverywhere(t *testing.T) {
	e := integration.Setup(t)
	visits := registerScopedVisits(t, e, "owner_id")
	for _, level := range scopeLevels {
		ctx := scopedReader(e, level.scope)
		assertTitles(t, level.scope, "search", searchedVisits(t, e, ctx), level.want)
		assertTitles(t, level.scope, "Live List engine", engineVisits(t, e, ctx, visits), level.want)
		assertTitles(t, level.scope, "History", historyVisits(t, e, ctx, visits.contact), level.want)
		assertTitles(t, level.scope, "row check", probedVisits(t, e, ctx, visits), level.want)
		if count := reportedVisits(t, e, ctx); count != len(level.want) {
			t.Errorf("%s: report count = %d, want %d", level.scope, count, len(level.want))
		}
	}
}

func TestAnExtensionKindWithoutAnOwnerColumnShowsEveryRowToItsReaders(t *testing.T) {
	e := integration.Setup(t)
	visits := registerScopedVisits(t, e, "")
	every := scopeLevels[len(scopeLevels)-1].want
	ctx := scopedReader(e, principal.RowScopeOwn)
	assertTitles(t, principal.RowScopeOwn, "search", searchedVisits(t, e, ctx), every)
	assertTitles(t, principal.RowScopeOwn, "row check", probedVisits(t, e, ctx, visits), every)
}

func assertTitles(t *testing.T, scope principal.RowScope, reader string, got, want []string) {
	t.Helper()
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("%s at %s scope = %v, want %v", reader, scope, got, want)
	}
}

func searchedVisits(t *testing.T, e *integration.Env, ctx context.Context) []string {
	t.Helper()
	page, err := search.NewStore(e.DB()).Search(ctx, search.Input{Query: "visit", Types: []string{scopeDemoKind}})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	var out []string
	for _, hit := range page.Hits {
		out = append(out, hit.Title)
	}
	return out
}

func engineVisits(t *testing.T, e *integration.Env, ctx context.Context, visits scopedVisits) []string {
	t.Helper()
	engine, ok, err := collections.NewStore(e.DB()).SegmentEngine(ctx, scopeDemoKind)
	if err != nil || !ok {
		t.Fatalf("the kind's engine: ok=%v err=%v", ok, err)
	}
	var found []ids.UUID
	err = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var selectErr error
		found, selectErr = engine.SelectIDs(ctx, tx, storekit.Predicate{Field: "title", Op: storekit.OpContains, Value: "visit"}, 10)
		return selectErr
	})
	if err != nil {
		t.Fatalf("selecting through the engine: %v", err)
	}
	return visits.titlesOf(found)
}

// reportedVisits runs the kind's count with no group floor, which would
// otherwise withhold counts this small.
func reportedVisits(t *testing.T, e *integration.Env, ctx context.Context) int {
	t.Helper()
	for _, r := range registeredReports() {
		if r.name != "ext:scopedemo:visits" {
			continue
		}
		answer, ok, err := newReportEngine(e.Pool).runRegistered(ctx, r, 0)
		if err != nil || !ok || answer.Total == nil {
			t.Fatalf("running the visit count: ok=%v err=%v answer=%+v", ok, err, answer)
		}
		return *answer.Total
	}
	t.Fatal("the visit count is not registered")
	return 0
}

func historyVisits(t *testing.T, e *integration.Env, ctx context.Context, contact ids.UUID) []string {
	t.Helper()
	code, entries := readExtensionHistory(ctx, t, e, contact)
	if code != http.StatusOK {
		t.Fatalf("History status = %d", code)
	}
	var out []string
	for _, entry := range entries {
		out = append(out, entry.Title)
	}
	return out
}

// probedVisits are the visits the row check behind links, attachments and
// writes admits; it refuses the others as missing.
func probedVisits(t *testing.T, e *integration.Env, ctx context.Context, visits scopedVisits) []string {
	t.Helper()
	var out []string
	for title, id := range visits.byTitle {
		err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			return auth.EnsureLinkTarget(ctx, tx, scopeDemoKind, id)
		})
		switch {
		case err == nil:
			out = append(out, title)
		case !errors.Is(err, apperrors.ErrNotFound):
			t.Fatalf("probing %s: %v, want admitted or not found", title, err)
		}
	}
	return out
}

func (v scopedVisits) titlesOf(found []ids.UUID) []string {
	var out []string
	for title, id := range v.byTitle {
		if slices.Contains(found, id) {
			out = append(out, title)
		}
	}
	return out
}

func TestBootRefusesAnOwnerColumnCoreCannotScopeBy(t *testing.T) {
	e := integration.Setup(t)
	registerScopedVisits(t, e, "owner_id")
	declared, ok := recordkinds.Lookup(scopeDemoKind)
	if !ok {
		t.Fatal("the visit kind did not register")
	}
	qualified := pgx.Identifier{"ext", scopeDemoTable}.Sanitize()
	conn := integration.OwnerConn(t)
	for column, want := range map[string]string{
		"owner_id":   "",
		"title":      "no uuid column",
		"missing":    "no uuid column",
		"contact_id": "no index for the row scope",
	} {
		declared.OwnerColumn = column
		err := verifyKindOwner(context.Background(), conn, declared, qualified)
		if want == "" && err != nil || want != "" && (err == nil || !strings.Contains(err.Error(), want)) {
			t.Errorf("owner column %s: %v, want %q", column, err, want)
		}
	}
}
