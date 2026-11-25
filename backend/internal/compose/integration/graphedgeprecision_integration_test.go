// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An edge says how to read its last_at from the rows the fold counts. A row the
// edge does not count says nothing about it, even at the same instant.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// exchange is one activity of the member with two contacts, so it folds into
// both an interaction edge and a contact edge.
type exchange struct {
	kind, precision, audience string
	meetingStatus             *string
	archived                  bool
}

func (v graphSkipEnv) exchangeAt(t *testing.T, at time.Time, x exchange, first, second ids.ContactID) ids.UUID {
	t.Helper()
	var activity ids.UUID
	v.inTx(t, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `INSERT INTO activity
			(kind, subject, direction, occurred_at, occurred_precision, audience, meeting_status, archived_at, source, captured_by)
			VALUES ($1, 'Visit', CASE WHEN $1 = 'email' THEN 'inbound' END, $2, $3, $4, $5,
			        CASE WHEN $6 THEN now() END, 'manual', 'human:test') RETURNING id`,
			x.kind, at, x.precision, x.audience, x.meetingStatus, x.archived).Scan(&activity); err != nil {
			return err
		}
		for _, participant := range []struct {
			column string
			id     ids.UUID
			role   string
		}{{"user_id", v.e.Rep1, "to"}, {"contact_id", first.UUID, "from"}, {"contact_id", second.UUID, "cc"}} {
			if _, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, `+participant.column+`, role)
				VALUES ($1, $2, $3)`, activity, participant.id, participant.role); err != nil {
				return err
			}
		}
		return nil
	})
	return activity
}

// precisions reads the stored last_precision of the member's edge to first and
// of the contact edge between the two.
func (v graphSkipEnv) precisions(t *testing.T, first, second ids.ContactID) (string, string) {
	t.Helper()
	a, b := first.UUID, second.UUID
	if b.String() < a.String() {
		a, b = b, a
	}
	var member, pair *string
	v.inTx(t, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `SELECT last_precision FROM graph_interaction_edge WHERE user_id = $1 AND contact_id = $2`,
			v.e.Rep1, first).Scan(&member); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT last_precision FROM graph_contact_edge WHERE contact_a = $1 AND contact_b = $2`,
			a, b).Scan(&pair)
	})
	deref := func(p *string) string {
		if p == nil {
			return "<null>"
		}
		return *p
	}
	return deref(member), deref(pair)
}

func TestAnEdgeReadsItsLastPrecisionFromTheRowsItCounts(t *testing.T) {
	v := graphSkipEnv{e: Setup(t)}
	held, canceled := "held", "canceled"
	visit := time.Now().UTC().AddDate(0, 0, -10).Truncate(24 * time.Hour).Add(12 * time.Hour)
	for _, c := range []struct {
		name string
		tied exchange
	}{
		{"a limited message at the same instant", exchange{kind: "email", precision: "instant", audience: "participants"}},
		{"a canceled meeting at the same instant", exchange{kind: "meeting", precision: "instant", audience: "workspace", meetingStatus: &canceled}},
		{"an archived message at the same instant", exchange{kind: "email", precision: "instant", audience: "workspace", archived: true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			first, second := v.contact(t, "Ada "+c.name), v.contact(t, "Bo "+c.name)
			day := v.exchangeAt(t, visit, exchange{kind: "meeting", precision: "day", audience: "workspace", meetingStatus: &held}, first, second)
			tied := v.exchangeAt(t, visit, c.tied, first, second)
			v.recompute(t, day, tied)
			if member, pair := v.precisions(t, first, second); member != "day" || pair != "day" {
				t.Errorf("after the fold the edges read %q and %q, want day for both", member, pair)
			}
			v.inTx(t, func(tx pgx.Tx) error { return search.RebuildEdges(v.e.Admin(), tx) })
			if member, pair := v.precisions(t, first, second); member != "day" || pair != "day" {
				t.Errorf("after the rebuild the edges read %q and %q, want day for both", member, pair)
			}
		})
	}
	t.Run("a meeting still ahead", func(t *testing.T) {
		first, second := v.contact(t, "Ada ahead"), v.contact(t, "Bo ahead")
		day := v.exchangeAt(t, visit, exchange{kind: "meeting", precision: "day", audience: "workspace", meetingStatus: &held}, first, second)
		ahead := v.exchangeAt(t, time.Now().UTC().AddDate(0, 0, 3),
			exchange{kind: "meeting", precision: "instant", audience: "workspace", meetingStatus: &held}, first, second)
		v.recompute(t, day, ahead)
		if member, pair := v.precisions(t, first, second); member != "day" || pair != "day" {
			t.Errorf("a meeting still ahead changed the edges to %q and %q, want day for both", member, pair)
		}
	})
	t.Run("a visible instant at the same instant wins", func(t *testing.T) {
		first, second := v.contact(t, "Ada tie"), v.contact(t, "Bo tie")
		day := v.exchangeAt(t, visit, exchange{kind: "meeting", precision: "day", audience: "workspace", meetingStatus: &held}, first, second)
		mail := v.exchangeAt(t, visit, exchange{kind: "email", precision: "instant", audience: "workspace"}, first, second)
		v.recompute(t, day, mail)
		if member, pair := v.precisions(t, first, second); member != "instant" || pair != "instant" {
			t.Errorf("a counted instant tied with the day entry reads %q and %q, want instant for both", member, pair)
		}
	})
}
