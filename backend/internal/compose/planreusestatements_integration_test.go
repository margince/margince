// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sentStatements records every statement a pool sends.
type sentStatements struct{ sql []string }

func (s *sentStatements) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	s.sql = append(s.sql, data.SQL)
	return ctx
}

func (*sentStatements) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// predicateSpelling strips what a partial-index predicate says besides its
// columns: literals, casts, null tests and calls.
var predicateSpelling = regexp.MustCompile(`'(?:[^']|'')*'|::\w+(?:\[\])?|\b\w+\s+IS\s+(?:NOT\s+)?NULL\b|\b\w+\s*\(`)

// predicateWords are the SQL words left in a predicate once predicateSpelling ran.
var predicateWords = map[string]bool{
	"and": true, "or": true, "not": true, "any": true, "array": true,
	"is": true, "distinct": true, "from": true, "true": true, "false": true, "in": true, "null": true,
}

// boundIndexedColumns reads, from the migrated catalog, every column a partial
// index predicate compares with a value or reads as a boolean. A generic plan
// cannot see a bound value, so a read binding one of them loses that index.
// archived_at is added: live-row indexes test it with IS NULL, which a read
// must spell too.
func boundIndexedColumns(t *testing.T, pool *pgxpool.Pool) *regexp.Regexp {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT DISTINCT pg_get_expr(i.indpred, i.indrelid)
		FROM pg_index i JOIN pg_class c ON c.oid = i.indrelid JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE i.indpred IS NOT NULL AND n.nspname = 'public'`)
	if err != nil {
		t.Fatalf("reading partial index predicates: %v", err)
	}
	predicates, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("reading partial index predicates: %v", err)
	}
	columns := map[string]bool{"archived_at": true}
	for _, predicate := range predicates {
		for _, word := range regexp.MustCompile(`[a-z_][a-z0-9_]*`).FindAllString(predicateSpelling.ReplaceAllString(predicate, " "), -1) {
			if !predicateWords[word] {
				columns[word] = true
			}
		}
	}
	for _, column := range []string{"kind", "direction", "role", "status", "audience", "entity_type", "meeting_status", "is_done"} {
		if !columns[column] {
			t.Fatalf("the partial-index census found no predicate on %s among %d predicates: it reads a smaller catalog than the schema has", column, len(predicates))
		}
	}
	return regexp.MustCompile(`(?i)\.(` + strings.Join(slices.Sorted(maps.Keys(columns)), "|") +
		`)\s*(=|<>|!=|IN|=\s*ANY)\s*\(?\s*\$\d+`)
}

// The reads a composed worklist reuses generic plans for spell every
// partial-index predicate as a literal. That lets a generic plan keep the index.
func TestTheReadsThatReuseGenericPlansBindNoPartialIndexPredicate(t *testing.T) {
	e := integration.Setup(t)
	sent := &sentStatements{}
	config := e.Pool.Config()
	config.ConnConfig.Tracer = sent
	pool, err := testdb.OwnPoolFromConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("opening a traced pool: %v", err)
	}
	defer pool.Close()
	contact := e.SeedContact(t, "Planned Once", &e.Rep1)
	subject, body, direction, at := "Following up", "Any news?", "outbound", clocktest.Now(t)
	mail, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, Source: "manual",
		EmailFrom: "rep@ourco.test", EmailTo: []string{"buyer@customer.test"},
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := activities.NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.WS)))
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	if err := database.WithWorkspaceSnapshot(reader, pool, func(ctx context.Context) error {
		if _, err := emailSummariesReusingPlans(ctx, store, []ids.UUID{ids.UUID(mail.Id)}); err != nil {
			return err
		}
		if _, err := (attentionTasks{store: store}).RemindersForViewer(ctx, at); err != nil {
			return err
		}
		if _, _, err := (attentionAwaiting{store: store}).AwaitingReplies(ctx, at); err != nil {
			return err
		}
		return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			_, err := contact360.LastTouchFor(ctx, tx, []ids.ContactID{ids.From[ids.ContactKind](contact)}, contact360.AssembleOptions{AsOf: at})
			return err
		})
	}); err != nil {
		t.Fatalf("running the reads: %v", err)
	}
	boundIndexedColumn := boundIndexedColumns(t, e.Pool)
	for _, bad := range []string{"WHERE a.kind = $3", "AND p.role IN ($2", "AND m.status = ANY($7)", "AND a.is_done = $4"} {
		if !boundIndexedColumn.MatchString(bad) {
			t.Fatalf("the check misses %q, so it would pass a bound predicate", bad)
		}
	}
	if len(sent.sql) == 0 {
		t.Fatal("no statement was sent, so there is nothing to hold")
	}
	for _, sql := range sent.sql {
		if found := boundIndexedColumn.FindString(sql); found != "" {
			t.Errorf("a statement under generic plans binds %q; spell it as a literal or leave the read out of plan reuse:\n%s", found, sql)
		}
	}
}
