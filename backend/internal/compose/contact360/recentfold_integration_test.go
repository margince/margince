// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contact360

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Every mail of the recent section happened in one second and was captured by
// three seats, the copies alternating by the millisecond. A short bound reads
// part of every group. The section must then widen, not show a partial count.
func TestTheRecentSectionWidensWhenItsBoundCutsTheOldestSecond(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, testdb.For(t).OwnerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the seeded history: %v", err)
		}
	}()
	contact := ids.From[ids.ContactKind](ids.NewV7())
	const mails = sectionCap + 2
	for _, seed := range []struct {
		statement string
		args      []any
	}{
		{`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Long History', 'manual', 'human:seed')`, []any{contact}},
		{`CREATE TEMP TABLE seeded ON COMMIT DROP AS
		  SELECT gen_random_uuid() id, g, k FROM generate_series(1, $1::int) g, generate_series(0, 2) k`, []any{mails}},
		{`INSERT INTO activity (id, kind, subject, body, direction, counterparty_email, occurred_at, created_at,
		                        source_system, source_id, source, captured_by)
		  SELECT id, 'email', 'Topic '||g, 'Body '||g, 'inbound', 'buyer@customer.test',
		         date_trunc('second', now()) - interval '1 day' + (k * 100 + g) * interval '1 ms',
		         now() - (3 - k) * interval '1 hour',
		         'email', 'recent-'||g||'@customer.test'||CASE k WHEN 0 THEN '' ELSE ' seat seat-'||k END, 'manual', 'human:seed'
		    FROM seeded`, nil},
		{`INSERT INTO activity_link (activity_id, entity_type, contact_id) SELECT id, 'contact', $1 FROM seeded`, []any{contact}},
	} {
		if _, err := tx.Exec(ctx, seed.statement, seed.args...); err != nil {
			t.Fatalf("seeding the history: %v", err)
		}
	}
	reader := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:reader", UserID: ids.NewV7(),
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{
			"activity": {Read: true}, "contact": {Read: true},
		}},
	})
	s := &Service{now: func() time.Time { return clocktest.Now(t) }}
	_, _, incomplete, err := s.readActivitiesAt(reader, tx, contact, AssembleOptions{}, "", byRecency, mails)
	if err != nil {
		t.Fatal(err)
	}
	if !incomplete {
		t.Error("a bound that reads one copy of every mail of one second was taken for the whole history")
	}
	rows, hasMore, err := s.readActivities(reader, tx, contact, AssembleOptions{}, "", byRecency)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != sectionCap || !hasMore {
		t.Fatalf("the section shows %d rows, more %t; want %d and more", len(rows), hasMore, sectionCap)
	}
	for _, row := range rows {
		if row.Copies == nil || *row.Copies != 3 {
			t.Fatalf("a row stands for %v copies, want 3", row.Copies)
		}
	}
}
