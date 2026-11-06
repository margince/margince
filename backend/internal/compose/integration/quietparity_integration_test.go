// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// contactTouches reads the touch the contact page and the Worklist both start
// from, and the zone they count in, inside the caller's transaction.
func contactTouches(
	ctx context.Context, tx pgx.Tx, contacts ...ids.ContactID,
) (map[ids.ContactID]relstrength.Touches, *time.Location, error) {
	touched, err := contact360.LastTouchFor(ctx, tx, contacts, contact360.AssembleOptions{})
	if err != nil {
		return nil, nil, err
	}
	zone, err := identity.ZoneAppliedTx(ctx, tx)
	if err != nil {
		return nil, nil, err
	}
	out := make(map[ids.ContactID]relstrength.Touches, len(touched))
	for id, touch := range touched {
		out[id] = touch.Touches()
	}
	return out, zone, nil
}

// changesOf is the derivation as the contact page runs it.
func changesOf(ctx context.Context, t *testing.T, e *Env, contact ids.ContactID, now time.Time) []relstrength.Change {
	t.Helper()
	var out []relstrength.Change
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		touches, zone, err := contactTouches(ctx, tx, contact)
		if err != nil {
			return err
		}
		out, err = e.Contacts.ContactRelationshipChangesTx(ctx, tx, contact, now, nil, touches[contact], zone)
		return err
	})
	if err != nil {
		t.Fatalf("reading relationship changes: %v", err)
	}
	return out
}

// The quiet count starts from the touch the page prints, and a message a
// colleague wrote on a shared thread is not the contact writing.
func TestQuietCountStartsFromTheOwnersLastTouchAndIgnoresAColleaguesMessage(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	mine := e.SeedContact(t, "Anna Weber", &e.Rep1)
	colleague := e.SeedContact(t, "Ben Colleague", &e.Rep1)
	now := roomFixedNow

	sent := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by)
		VALUES ($1, 'email', 'we wrote', '2026-04-26T09:00:00Z', 'outbound', 'manual', 'human:x')`)
	LinkActivity(t, owner, sent, "contact", mine)
	met := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'the meeting', '2026-06-05T09:00:00Z', 'manual', 'human:x')`)
	LinkActivity(t, owner, met, "contact", mine)
	thread := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by)
		VALUES ($1, 'email', 'someone else wrote', '2026-07-25T09:00:00Z', 'inbound', 'manual', 'human:x')`)
	LinkActivity(t, owner, thread, "contact", mine)
	LinkActivitySender(t, owner, thread, colleague)

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, roomPerms)
	contact := ids.From[ids.ContactKind](mine)
	var touch contact360.LastTouch
	if err := database.WithWorkspaceTx(rep, e.Pool, func(tx pgx.Tx) error {
		touched, err := contact360.LastTouchFor(rep, tx, []ids.ContactID{contact}, contact360.AssembleOptions{})
		touch = touched[contact]
		return err
	}); err != nil {
		t.Fatal(err)
	}
	wantTouch := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
	if touch.ContactAt == nil || !touch.ContactAt.Equal(wantTouch) {
		t.Fatalf("ContactAt = %v, want the meeting %v: a colleague's message is not the contact", touch.ContactAt, wantTouch)
	}
	if touch.InboundAt != nil {
		t.Errorf("InboundAt = %v, want nil: the contact never wrote", touch.InboundAt)
	}

	page, err := contactRoomService(e).Assemble(rep, contact)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if page.LastContactAt == nil || !page.LastContactAt.Equal(wantTouch) {
		t.Errorf("the page prints last_contact_at %v, want the meeting %v", page.LastContactAt, wantTouch)
	}

	var quiet *relstrength.Change
	for _, c := range changesOf(rep, t, e, contact, now) {
		if c.Kind == relstrength.ChangeWentQuiet {
			quiet = &c
		}
	}
	if quiet == nil {
		t.Fatal("a contact last met 60 days ago was not reported as quiet")
	}
	if !quiet.At.Equal(*touch.ContactAt) {
		t.Errorf("quiet counts from %v, the page prints %v", quiet.At, *touch.ContactAt)
	}
	if want := 60; quiet.Days != want {
		t.Errorf("quiet days = %d, want %d from the meeting, not 100 from the mail we sent or 10 from the colleague's", quiet.Days, want)
	}
}
