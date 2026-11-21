// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// A record's recent timeline folds the mailbox copies of its mail. A bounded
// fold must give the page the unbounded fold gave, and cost the page's rows.

import (
	"context"
	"reflect"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const copiedMailCount = 400

// seedCopiedMail files copiedMailCount mails on one contact, each captured by
// three seats. Mails come five to a second and the copies of one mail differ
// in the fraction of it. So the cut can fall inside a second and a group.
func (e *loadEnv) seedCopiedMail(t *testing.T, contact ids.UUID) {
	t.Helper()
	e.exec(t, `WITH seeded_copy AS (
		SELECT gen_random_uuid() id, g, k FROM generate_series(1, $1::int) g, generate_series(0, 2) k),
		copies AS (INSERT INTO activity (id, kind, subject, body, direction, counterparty_email, occurred_at, created_at,
		                      source_system, source_id, source, captured_by)
		SELECT id, 'email', 'Offer ' || g, 'Body ' || g, 'inbound', 'buyer@customer.test',
		       date_trunc('second', now()) - (g / 5) * interval '1 minute' + (g % 5) * interval '100 ms' + k * interval '1 ms',
		       now() - (3 - k) * interval '1 hour',
		       'email', 'msg-' || g || '@customer.test' ||
		         CASE k WHEN 0 THEN '' WHEN 1 THEN ' seat ' || $2::text ELSE ' seat ' || $3::text END,
		       'seed', 'system'
		FROM seeded_copy RETURNING id)
		INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		SELECT gen_random_uuid(), id, 'contact', $4::uuid FROM copies`,
		copiedMailCount, e.rep.String(), e.other.String(), contact)
	e.exec(t, `ANALYZE activity`)
	e.exec(t, `ANALYZE activity_link`)
}

func TestABoundedFoldPagesAContactLikeTheUnboundedFold(t *testing.T) {
	e := setupLoad(t)
	contact := e.buyer(t)
	e.seedCopiedMail(t, contact)
	store := handledStore(e)
	read := func(cursor *string, bounds []int) ([]crmcontracts.Activity, string, bool) {
		in := ListActivitiesInput{
			EntityType: new("contact"), EntityID: &contact, CollapseSeatCopies: true,
			Limit: new(7), Cursor: cursor, foldBounds: bounds,
		}
		rows, page, err := store.ListActivities(e.followUpReader(), in)
		if err != nil {
			t.Fatalf("reading a page: %v", err)
		}
		return rows, page.NextCursor, page.HasMore
	}
	var cursor *string
	seen := map[ids.UUID]bool{}
	for pageNumber := 1; ; pageNumber++ {
		want, wantCursor, wantMore := read(cursor, []int{0})
		got, gotCursor, gotMore := read(cursor, nil)
		if !reflect.DeepEqual(got, want) || gotCursor != wantCursor || gotMore != wantMore {
			t.Fatalf("page %d differs under the bounded fold:\n got %d rows, cursor %q, more %t\nwant %d rows, cursor %q, more %t",
				pageNumber, len(got), gotCursor, gotMore, len(want), wantCursor, wantMore)
		}
		for _, row := range got {
			if seen[ids.UUID(row.Id)] || row.Copies == nil || *row.Copies != 3 {
				t.Fatalf("page %d shows a mail twice or not as three copies: %+v", pageNumber, row.Copies)
			}
			seen[ids.UUID(row.Id)] = true
		}
		if !gotMore {
			break
		}
		cursor = &gotCursor
	}
	if len(seen) != copiedMailCount {
		t.Fatalf("the walk showed %d mails, want %d", len(seen), copiedMailCount)
	}
}

func TestABoundedFoldWidensWhenCopiesFillItsCandidates(t *testing.T) {
	e := setupLoad(t)
	contact := e.buyer(t)
	e.seedCopiedMail(t, contact)
	// Four candidates hold one or two groups, so a page of three comes up short
	// and the read goes on to the unbounded fold.
	in := ListActivitiesInput{
		EntityType: new("contact"), EntityID: &contact, CollapseSeatCopies: true,
		Limit: new(3), foldBounds: []int{4, 0},
	}
	want, _, err := handledStore(e).ListActivities(e.followUpReader(), ListActivitiesInput{
		EntityType: in.EntityType, EntityID: in.EntityID, CollapseSeatCopies: true, Limit: in.Limit, foldBounds: []int{0},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := handledStore(e).ListActivities(e.followUpReader(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) || len(got) != 3 {
		t.Fatalf("a bound too small for the page returned %d rows, want the unbounded %d", len(got), len(want))
	}
}

func TestACollapsedPageFoldsOnlyTheCandidatesNearItsRows(t *testing.T) {
	e := setupLoad(t)
	contact := e.buyer(t)
	e.seedCopiedMail(t, contact)
	ctx := e.followUpReader()
	foldedRows := func(foldRows int) float64 {
		in := ListActivitiesInput{EntityType: new("contact"), EntityID: &contact, CollapseSeatCopies: true}
		page, err := timelinePageAt(ctx, in, 7, foldRows)
		if err != nil {
			t.Fatal(err)
		}
		var raw string
		args := waitingListArgs(ctx, in, page.args)
		if err := e.owner.QueryRow(context.Background(), "EXPLAIN (ANALYZE, FORMAT JSON) "+page.sql, args...).Scan(&raw); err != nil {
			t.Fatalf("planning the page: %v", err)
		}
		var windowed float64
		decodePlan(t, raw).walk(func(n planNode) {
			if n.NodeType == "WindowAgg" {
				windowed = max(windowed, n.ActualRows)
			}
		})
		return windowed
	}
	all, bounded := foldedRows(0), foldedRows(8*8)
	if all != copiedMailCount*3 {
		t.Fatalf("the unbounded fold windows %.0f rows, want every candidate (%d)", all, copiedMailCount*3)
	}
	if bounded > 64 {
		t.Errorf("the bounded fold windows %.0f rows, want at most the 64 it reads", bounded)
	}
}

// seedInterleavedCopies files two three-copy mails in one second, their copies
// alternating by the millisecond. Any bound that reads fewer than all six rows
// reads a partial group from each.
func (e *loadEnv) seedInterleavedCopies(t *testing.T, contact ids.UUID) {
	t.Helper()
	e.exec(t, `WITH seeded_copy AS (SELECT gen_random_uuid() id, g, k FROM generate_series(0, 1) g, generate_series(0, 2) k),
		copies AS (INSERT INTO activity (id, kind, subject, body, direction, counterparty_email, occurred_at, created_at,
		                      source_system, source_id, source, captured_by)
		SELECT id, 'email', 'Offer ' || g, 'Body ' || g, 'inbound', 'buyer@customer.test',
		       date_trunc('second', now()) - interval '1 day' + (2 * k + g) * interval '1 ms',
		       now() - (3 - k) * interval '1 hour',
		       'email', 'twin-' || g || '@customer.test' ||
		         CASE k WHEN 0 THEN '' WHEN 1 THEN ' seat ' || $1::text ELSE ' seat ' || $2::text END,
		       'seed', 'system'
		FROM seeded_copy RETURNING id)
		INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		SELECT gen_random_uuid(), id, 'contact', $3::uuid FROM copies`, e.rep.String(), e.other.String(), contact)
}

func TestABoundedFoldNeverShowsAPartialGroupOfTheOldestSecond(t *testing.T) {
	e := setupLoad(t)
	contact := e.buyer(t)
	e.seedInterleavedCopies(t, contact)
	read := func(bounds []int) []crmcontracts.Activity {
		rows, _, err := handledStore(e).ListActivities(e.followUpReader(), ListActivitiesInput{
			EntityType: new("contact"), EntityID: &contact, CollapseSeatCopies: true, Limit: new(1), foldBounds: bounds,
		})
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	got, want := read([]int{4, 0}), read([]int{0})
	if !reflect.DeepEqual(got, want) || len(got) != 1 || got[0].Copies == nil || *got[0].Copies != 3 {
		t.Fatalf("a bound that cuts the second between two mails shows %+v, want the unbounded %+v", got, want)
	}
}
