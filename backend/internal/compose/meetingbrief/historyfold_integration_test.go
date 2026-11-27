// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package meetingbrief

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Two mails of one second, three seats each, their copies alternating by the
// millisecond. A bound of four rows reads part of both groups. The history must
// then widen, or it names a copy that is not the shown one.
func TestTheHistoryWidensWhenItsBoundCutsTheOldestSecond(t *testing.T) {
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
	now := clocktest.Now(t)
	attendee, room := ids.NewV7(), ids.NewV7()
	for _, seed := range []struct {
		statement string
		args      []any
	}{
		{`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Attendee', 'manual', 'human:seed')`, []any{attendee}},
		{`CREATE TEMP TABLE seeded ON COMMIT DROP AS
		  SELECT gen_random_uuid() id, g, k FROM generate_series(0, 1) g, generate_series(0, 2) k`, nil},
		{`INSERT INTO activity (id, kind, subject, body, direction, counterparty_email, occurred_at, created_at,
		                        source_system, source_id, source, captured_by)
		  SELECT id, 'email', 'Topic '||g, 'Body '||g, 'inbound', 'buyer@customer.test',
		         date_trunc('second', $1::timestamptz) - interval '2 days' + (2 * k + g) * interval '1 ms',
		         $1::timestamptz - (3 - k) * interval '1 hour',
		         'email', 'brief-'||g||'@customer.test'||CASE k WHEN 0 THEN '' ELSE ' seat seat-'||k END, 'manual', 'human:seed'
		    FROM seeded`, []any{now}},
		{`INSERT INTO activity_link (activity_id, entity_type, contact_id) SELECT id, 'contact', $1 FROM seeded`, []any{attendee}},
	} {
		if _, err := tx.Exec(ctx, seed.statement, seed.args...); err != nil {
			t.Fatalf("seeding the history: %v", err)
		}
	}
	reader := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:reader", UserID: ids.NewV7(),
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{
			"activity": {Read: true}, "contact": {Read: true}, "deal": {Read: true},
		}},
	})
	meet := meeting{ID: room, Opens: now.Add(time.Hour), StartsAt: now.Add(time.Hour), Room: []ids.UUID{attendee}}
	s := &Service{}
	const limit = 2
	whole, full, err := s.readHistoryAt(reader, tx, meet, nil, now, meet.Room, limit, 0)
	if err != nil || len(whole) != limit || !full {
		t.Fatalf("the unbounded history has %d rows, full %t, err %v; want %d", len(whole), full, err, limit)
	}
	cut, full, err := s.readHistoryAt(reader, tx, meet, nil, now, meet.Room, limit, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !connector.FoldIncomplete(len(cut), limit, full) {
		t.Errorf("a bound that cuts the second between two mails was taken for the whole history: %d rows", len(cut))
	}
	got, err := s.readHistoryOf(reader, tx, meet, nil, now, meet.Room, limit)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != limit || got[0].ID != whole[0].ID || got[1].ID != whole[1].ID {
		t.Fatalf("the history shows %+v, want the unbounded %+v", got, whole)
	}
}
