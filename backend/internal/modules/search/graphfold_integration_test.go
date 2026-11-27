// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package search

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Two mails of one second, three seats each, their copies alternating by the
// millisecond. A bound of four rows reads part of both groups, so the anchor's
// timeline must widen and name every copy of both mails.
func TestTheAnchorTimelineWidensWhenItsBoundCutsTheOldestSecond(t *testing.T) {
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
			t.Errorf("rolling back the seeded mail: %v", err)
		}
	}()
	now := clocktest.Now(t)
	anchor := ids.NewV7()
	for _, seed := range []struct {
		statement string
		args      []any
	}{
		{`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Anchor', 'manual', 'human:seed')`, []any{anchor}},
		{`CREATE TEMP TABLE seeded ON COMMIT DROP AS
		  SELECT gen_random_uuid() id, g, k FROM generate_series(0, 1) g, generate_series(0, 2) k`, nil},
		{`INSERT INTO activity (id, kind, subject, body, direction, counterparty_email, occurred_at, created_at,
		                        source_system, source_id, thread_key, source, captured_by)
		  SELECT id, 'email', 'Topic '||g, 'Body '||g, 'inbound', 'buyer@customer.test',
		         date_trunc('second', $1::timestamptz) - interval '2 days' + (2 * k + g) * interval '1 ms',
		         $1::timestamptz - (3 - k) * interval '1 hour',
		         'email', 'graph-'||g||'@customer.test'||CASE k WHEN 0 THEN '' ELSE ' seat seat-'||k END,
		         'thread-'||g, 'manual', 'human:seed'
		    FROM seeded`, []any{now}},
		{`INSERT INTO activity_link (activity_id, entity_type, contact_id) SELECT id, 'contact', $1 FROM seeded`, []any{anchor}},
	} {
		if _, err := tx.Exec(ctx, seed.statement, seed.args...); err != nil {
			t.Fatalf("seeding the mail: %v", err)
		}
	}
	reader := readerFor(entityContact, entityActivity)
	linkCol := anchorLinkColumn[string(entityContact)]
	const limit = 2
	_, _, whole, short, err := anchorTimelineAt(reader, tx, linkCol, anchor, 10, now, projectScope{}, limit, 0)
	if err != nil || short || len(whole) != 6 {
		t.Fatalf("the unbounded timeline names %d copies, short %t, err %v; want 6", len(whole), short, err)
	}
	_, _, _, short, err = anchorTimelineAt(reader, tx, linkCol, anchor, 10, now, projectScope{}, limit, 4)
	if err != nil || !short {
		t.Errorf("a bound that cuts the second between two mails was taken for the whole timeline: short %t, err %v", short, err)
	}
	_, _, got, err := anchorTimeline(reader, tx, linkCol, anchor, 10, now, projectScope{})
	if err != nil || len(got) != 6 {
		t.Fatalf("the anchor timeline names %d copies, err %v; want 6", len(got), err)
	}
}
