// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the who-is-waiting seam attaches to a wait, against a real database.
//
// The lane spans email and channel messages (the waiting query's own
// `a.kind IN ('email', 'message')`), and only an email has an email's shape.
// The kind filter lives in the seam, so a unit test that builds a
// WaitingCustomer by hand cannot see it fail — this file goes through the
// seam's own read.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedWaitingOfKind writes one inbound message of the given kind, filed under a
// person so it is sales mail rather than a rep's private correspondence — the
// lane requires that link, and a message seeded without one correctly never
// appears at all.
func seedWaitingOfKind(
	t *testing.T, e *integration.Env, person ids.UUID, kind, subject string, at time.Time,
) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id,
			                      source, captured_by, audience, occurred_at, thread_key,
			                      channel_provider)
			VALUES ($1, $2, $3, 'What did we agree on the price?', 'inbound', 'gmail', $4,
			        'gmail:'||$4, 'connector:gmail', 'workspace', $5, $6,
			        CASE WHEN $2 = 'message' THEN 'telegram' END)`,
			id, kind, subject, "wait-"+id.String(), at, "thread-"+id.String()); err != nil {
			return err
		}
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity_participant (activity_id, role, address)
			VALUES ($1, 'from', 'dana@acme.example')`, id); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity_link (activity_id, entity_type, person_id)
			VALUES ($1, 'person', $2)`, id, person)
		return err
	}); err != nil {
		t.Fatalf("seeding a waiting %s: %v", kind, err)
	}
	return id
}

func waitingOf(rows []attention.WaitingCustomer, id ids.UUID) *attention.WaitingCustomer {
	for i := range rows {
		if rows[i].ActivityID == id {
			return &rows[i]
		}
	}
	return nil
}

// An email wait carries the canonical row; a chat waiting in the same lane
// carries none. Both halves in one test, because either alone passes for the
// wrong reason: a seam attaching nothing satisfies the chat claim, and one
// attaching a row to everything satisfies the email claim.
func TestTheWaitingSeamAttachesAnEmailRowToEmailsAlone(t *testing.T) {
	e := integration.Setup(t)
	asOf := time.Now()
	person := seedLinkedPerson(t, e, "dana@acme.example")
	mail := seedWaitingOfKind(t, e, person, "email", "Re: the renewal quote", asOf.Add(-3*24*time.Hour))
	chat := seedWaitingOfKind(t, e, person, "message", "ping about the quote", asOf.Add(-2*24*time.Hour))

	seam := attentionWaiting{
		store: activities.NewStore(e.DB()),
		now:   func() time.Time { return asOf },
	}
	rows, _, err := seam.Unanswered(e.Admin(), asOf)
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}

	gotMail := waitingOf(rows, mail)
	if gotMail == nil {
		t.Fatalf("the waiting email did not reach the lane at all; %d row(s) came back", len(rows))
	}
	if gotMail.EmailSummary == nil {
		t.Fatal("a waiting email carried no canonical row")
	}
	if gotMail.EmailSummary.Subject == nil || *gotMail.EmailSummary.Subject != "Re: the renewal quote" {
		t.Errorf("the email's row named %v, want its own subject", gotMail.EmailSummary.Subject)
	}
	if gotMail.EmailSummary.Preview == nil || *gotMail.EmailSummary.Preview == "" {
		t.Error("the email's row carried no preview; the queue shows one and the timeline shows the same one")
	}

	gotChat := waitingOf(rows, chat)
	if gotChat == nil {
		t.Fatalf("the waiting chat did not reach the lane; the kind claim below would prove nothing")
	}
	if gotChat.EmailSummary != nil {
		t.Errorf("a waiting chat carried an email row: %+v — only an email has an email's shape",
			*gotChat.EmailSummary)
	}
}

// A message whose content this reader may not read produces no waiting row at
// all — so there is nothing to attach a row to. The seam's reader carries the
// content gate a second time, which is the lock that holds if the lane's own
// gate is ever loosened.
func TestTheWaitingSeamHandsNoRowForAMessageTheReaderCannotRead(t *testing.T) {
	e := integration.Setup(t)
	asOf := time.Now()
	person := seedLinkedPerson(t, e, "dana@acme.example")
	mail := seedWaitingOfKind(t, e, person, "email", "Severance terms", asOf.Add(-3*24*time.Hour))

	// Admitted first, so the refusal below is the audience's doing rather than
	// the fixture never having qualified.
	seam := attentionWaiting{
		store: activities.NewStore(e.DB()),
		now:   func() time.Time { return asOf },
	}
	before, _, err := seam.Unanswered(e.Admin(), asOf)
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}
	if got := waitingOf(before, mail); got == nil || got.EmailSummary == nil {
		t.Fatalf("the open mail carried no row to begin with; the held case proves nothing")
	}

	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(context.Background(),
			`UPDATE activity SET audience = 'participants' WHERE id = $1`, mail)
		return execErr
	}); err != nil {
		t.Fatalf("limiting the message: %v", err)
	}

	// AdminPerms deliberately: the refusal below must be the AUDIENCE's, not a
	// missing permission. A reader who could not have seen it either way would
	// pass this test while proving nothing about the audience.
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, integration.AdminPerms)
	after, _, err := seam.Unanswered(colleague, asOf)
	if err != nil {
		t.Fatalf("colleague reading who is waiting: %v", err)
	}
	if got := waitingOf(after, mail); got != nil {
		t.Errorf("a limited message reached a colleague's queue: %+v", *got)
	}
}
