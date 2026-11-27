// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A message stored with no direction counts as the contact writing when its
// one sender row is unambiguously them. No current writer stores such a row,
// so each case plants the shape older mail was kept in.

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// directionlessSender is the `from` row of one direction-less mail.
type directionlessSender struct {
	contact *ids.ContactID
	user    *ids.UUID
	address string
}

// seedDirectionlessMail files an email with no direction, linked to linked,
// whose `from` rows are senders.
func (e *dedupeEnv) seedDirectionlessMail(t *testing.T, linked ids.ContactID, at time.Time, senders ...directionlessSender) {
	t.Helper()
	id := ids.New[ids.ActivityKind]()
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, occurred_at, source_system, source_id, source, captured_by)
			VALUES ($1, 'email', 'Re: Hallo', $2, 'gmail', $3, 'gmail:seed', 'connector:gmail')`, id, at, id.String()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, id, linked); err != nil {
			return err
		}
		for _, s := range senders {
			if _, err := tx.Exec(ctx, `
				INSERT INTO activity_participant (activity_id, role, contact_id, user_id, address)
				VALUES ($1, 'from', $2, $3, nullif($4, ''))`, id, s.contact, s.user, s.address); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding a direction-less mail: %v", err)
	}
}

func (e *dedupeEnv) seedContactAddress(t *testing.T, contact ids.ContactID, address string) {
	t.Helper()
	e.execTx(e.as(), t, `INSERT INTO contact_email (contact_id, email, source, captured_by)
		VALUES ($1, $2, 'manual', 'human:test')`, contact, address)
}

func TestADirectionlessReplyFromTheContactCountsAsTheirs(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	now := present()
	sent, replied := now.AddDate(0, 0, -6), now.AddDate(0, 0, -2)
	ann := e.seedVisibleContact(t, "Ann Directionless")
	e.seedContactAddress(t, ann, "ann@directionless.test")
	e.seedInteraction(t, ann, sent, "outbound")
	e.seedDirectionlessMail(t, ann, replied, directionlessSender{contact: &ann, address: "ann@directionless.test"})

	got := e.assertStoredIsLive(t, "a direction-less reply", ann, now)
	if got.Inbound90d != 1 || got.Outbound90d != 1 {
		t.Errorf("the fold counts %d in and %d out, want the reply in and our send out", got.Inbound90d, got.Outbound90d)
	}
	if !sameMoment(got.LastEngaged, &replied) || !sameMoment(got.LastInbound, &replied) {
		t.Errorf("last engaged %v and last inbound %v, want the reply at %v", got.LastEngaged, got.LastInbound, replied)
	}
}

func TestADirectionlessMailWithNoUnambiguousContactSenderIsNotTheirs(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	now := present()
	sent, wrote := now.AddDate(0, 0, -6), now.AddDate(0, 0, -2)
	bea := e.seedVisibleContact(t, "Bea Moved")
	cy := e.seedVisibleContact(t, "Cy Holder")
	dan := e.seedVisibleContact(t, "Dan Twice")
	eve := e.seedVisibleContact(t, "Eve Member")
	e.seedContactAddress(t, bea, "desk@shared.test")
	e.execTx(e.as(), t, `UPDATE contact_email SET archived_at = now() WHERE contact_id = $1`, bea)
	e.seedContactAddress(t, cy, "desk@shared.test")
	e.seedContactAddress(t, eve, "eve@member.test")
	rep := e.rep
	cases := []struct {
		name    string
		contact ids.ContactID
		senders []directionlessSender
	}{
		{"an address that moved to another contact", bea, []directionlessSender{{contact: &bea, address: "desk@shared.test"}}},
		{"two sender rows", dan, []directionlessSender{{contact: &dan}, {contact: &cy}}},
		{"a member's own sender row", eve, []directionlessSender{{user: &rep, address: "eve@member.test"}}},
	}
	for _, c := range cases {
		e.seedInteraction(t, c.contact, sent, "outbound")
		e.seedDirectionlessMail(t, c.contact, wrote, c.senders...)
		got := e.assertStoredIsLive(t, c.name, c.contact, now)
		if got.Inbound90d != 0 || got.LastInbound != nil || !sameMoment(got.LastEngaged, nil) {
			t.Errorf("%s: the fold counts %d in, last inbound %v, last engaged %v; want none of the mail as theirs",
				c.name, got.Inbound90d, got.LastInbound, got.LastEngaged)
		}
		if !sameMoment(got.LastInteraction, &wrote) {
			t.Errorf("%s: last interaction %v, want the mail at %v", c.name, got.LastInteraction, wrote)
		}
	}
}

// The page is the data task's: a member's request may not walk every contact.
func TestOnlyTheSystemReadsTheDirectionlessSenderPage(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, _, err := DirectionlessSendersPageTx(ctx, tx, ids.UUID{}, 10)
		return err
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a member's read of the direction-less sender page answered %v, want a permission denial", err)
	}
}
