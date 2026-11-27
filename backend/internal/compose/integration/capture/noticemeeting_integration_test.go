// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A calendar resync changes the real participant response. The notice consumer
// must hear that change, while leaving unanswered invitations owed.
func TestAReferralAcceptingAnInvitationClosesTheirNoticeDuty(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	accepted := ensureGuestContact(t, e, guestKnown)
	unanswered := ensureGuestContact(t, e, guestUnread)
	organizer := ensureGuestContact(t, e, guestLater)
	declined := ensureGuestContact(t, e, guestOptional)
	tentative := ensureGuestContact(t, e, guestAddedLater)
	ctx := principal.SystemActing(e.Admin(), "system:notice-test")
	for _, contact := range []ids.ContactID{accepted, unanswered, organizer, declined, tentative} {
		if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
			var acquisition ids.UUID
			if err := tx.QueryRow(ctx, `INSERT INTO contact_acquisition_evidence (contact_id,kind,captured_by) VALUES ($1,'referral','human:seed') RETURNING id`, contact).Scan(&acquisition); err != nil {
				return err
			}
			return consent.OpenNoticeCaseTx(ctx, tx, consent.NoticeCaseInput{ContactID: contact, AcquisitionID: acquisition, Rule: consent.RuleArt14, State: consent.NoticeOpen, DueAt: captureNow.AddDate(0, 1, 0)})
		}); err != nil {
			t.Fatal(err)
		}
	}
	feed := &calendarFeed{}
	feed.set(lateGuestEvent("review", "confirmed", captureNow.AddDate(0, 0, 1), guestLater, ownerEntry("accepted"), guestEntry(guestKnown, "needsAction"), guestEntry(guestUnread, "needsAction"), guestEntry(guestOptional, "declined"), guestEntry(guestAddedLater, "tentative")))
	sync := connectFeed(t, env, feed)
	sync()
	meeting := gcalMeeting(t, e)
	consumer := compose.NewNoticeCaseOpen(e.Pool, func() time.Time { return captureNow }, slog.New(slog.DiscardHandler))
	drive := func(kind string) {
		t.Helper()
		if err := consumer.HandleEvent(context.Background(), events.Envelope{Type: kind, Entity: events.EntityRef{Type: "activity", ID: meeting}}); err != nil {
			t.Fatal(err)
		}
	}
	state := func(contact ids.ContactID) string {
		t.Helper()
		var found string
		if err := e.Owner.QueryRow(ctx, `SELECT state FROM privacy_notice_case WHERE contact_id=$1`, contact).Scan(&found); err != nil {
			t.Fatal(err)
		}
		return found
	}
	drive("activity.captured")
	if state(accepted) != "open" || state(unanswered) != "open" || state(organizer) != "exempt_with_reason" {
		t.Fatal("organizing or an unanswered invitation used the wrong notice rule")
	}
	feed.set(lateGuestEvent("review", "confirmed", captureNow.AddDate(0, 0, 1), guestLater, ownerEntry("accepted"), guestEntry(guestKnown, "accepted"), guestEntry(guestUnread, "needsAction"), guestEntry(guestOptional, "declined"), guestEntry(guestAddedLater, "tentative")))
	sync()
	drive("activity.updated")
	drive("activity.updated")
	if state(declined) != "open" || state(tentative) != "open" {
		t.Fatal("a tentative or declined answer closed a duty")
	}
	if state(accepted) != "exempt_with_reason" || state(unanswered) != "open" {
		t.Fatal("accepting did not close exactly the accepting contact's duty")
	}
}
