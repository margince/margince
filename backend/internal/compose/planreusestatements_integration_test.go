// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
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

// boundIndexedColumn is a predicate on a column partial indexes are written
// over, compared against a bound value. A generic plan cannot see the value, so
// it cannot use such an index.
var boundIndexedColumn = regexp.MustCompile(
	`(?i)\.(kind|direction|role|status|audience|entity_type|meeting_status|archived_at)\s*(=|<>|!=|IN|=\s*ANY)\s*\(?\s*\$\d+`)

// The reads a composed worklist reuses generic plans for send every partial-index
// predicate as a literal, which is what lets a generic plan keep the index.
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
	subject, body, direction, at := "Following up", "Any news?", "outbound", time.Now()
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
		return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			_, err := contact360.LastTouchFor(ctx, tx, []ids.ContactID{ids.From[ids.ContactKind](contact)}, contact360.AssembleOptions{AsOf: at})
			return err
		})
	}); err != nil {
		t.Fatalf("running the reads: %v", err)
	}
	for _, bad := range []string{"WHERE a.kind = $3", "AND p.role IN ($2", "AND m.status = ANY($7)"} {
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
