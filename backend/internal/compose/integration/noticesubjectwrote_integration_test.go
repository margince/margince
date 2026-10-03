// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A contact who writes to us owes no notice, whichever order the backfill read
// the conversation in.
//
// A mailbox backfill does not read oldest first. When it reads our reply before
// the contact's first mail, it mints the contact from our side, cannot say where
// the address came from, and an Art. 14 case opens. On the first real backfill
// three of sixteen open cases were contacts whose earliest captured mail was
// their own.

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// withAddress gives a contact the address its mail is matched by.
func withAddress(t *testing.T, e *apptest.AppEnv, contactID, email string) {
	t.Helper()
	if _, err := e.Owner.Exec(context.Background(), `
		INSERT INTO contact_email (contact_id, email, source, captured_by)
		VALUES ($1, $2, 'manual', 'human:seed')`, contactID, email); err != nil {
		t.Fatalf("giving the contact an address: %v", err)
	}
}

// capturedUnknown makes a contact the way capture does when it cannot name the
// source: unknown_legacy, written by a connector.
func capturedUnknown(t *testing.T, e *apptest.AppEnv, name string) string {
	t.Helper()
	return capturedUnknownBy(t, e, name, "connector:gmail")
}

// capturedUnknownBy is capturedUnknown with the writer named: a connector, or
// the counterparty verdict that mints contacts from captured mail afterwards.
func capturedUnknownBy(t *testing.T, e *apptest.AppEnv, name, capturedBy string) string {
	t.Helper()
	contact := contactAcquiredAs(t, e, name, "unknown_legacy")
	if _, err := e.Owner.Exec(context.Background(), `
		UPDATE contact_acquisition_evidence SET captured_by = $2 WHERE contact_id = $1`,
		contact, capturedBy); err != nil {
		t.Fatalf("stating capture wrote the acquisition: %v", err)
	}
	return contact
}

// capturedMailFrom writes one mail a connector delivered from an address, and
// answers its id.
func capturedMailFrom(t *testing.T, e *apptest.AppEnv, from string, bulk bool) ids.UUID {
	t.Helper()
	return mailFrom(t, e, from, bulk, "connector:gmail")
}

// mailFrom writes one inbound mail from an address, stamped by whoever wrote it.
func mailFrom(t *testing.T, e *apptest.AppEnv, from string, bulk bool, capturedBy string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	ctx := context.Background()
	if _, err := e.Owner.Exec(ctx, `
		INSERT INTO activity (id, kind, subject, direction, occurred_at, source_system, source_id,
		                      source, captured_by, bulk_mail_attested)
		VALUES ($1, 'email', 'hello', 'inbound', now() - interval '1 day', 'gmail', $2,
		        'gmail:seed', $4, $3)`, id, id.String(), bulk, capturedBy); err != nil {
		t.Fatalf("capturing a mail: %v", err)
	}
	if _, err := e.Owner.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'from', $2)`,
		id, from); err != nil {
		t.Fatalf("naming who sent the mail: %v", err)
	}
	return id
}

func driveCapturedMail(t *testing.T, e *apptest.AppEnv, activityID ids.UUID) {
	t.Helper()
	consumer := compose.NewNoticeCaseOpen(e.Pool, time.Now, slog.New(slog.DiscardHandler))
	if err := consumer.HandleEvent(context.Background(), events.Envelope{
		Type:   "activity.captured",
		Entity: events.EntityRef{Type: "activity", ID: activityID},
	}); err != nil {
		t.Fatalf("handling the captured mail: %v", err)
	}
}

func acquisitionKinds(t *testing.T, e *apptest.AppEnv, contactID string) map[string]bool {
	t.Helper()
	rows, err := e.Owner.Query(context.Background(),
		`SELECT kind FROM contact_acquisition_evidence WHERE contact_id = $1`, contactID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		out[k] = true
	}
	return out
}

// The case opened first, their mail arrived second: the mail settles it.
func TestTheirLaterMailSettlesTheNoticeCase(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contact := capturedUnknown(t, e, "Late Writer")
	withAddress(t, e, contact, "late.writer@customer.test")
	driveNoticeCase(t, e, contact)
	if _, state, _, found := noticeCaseFor(t, e, contact); !found || state != "open" {
		t.Fatalf("setup: case found=%v state=%q, want an open case for an unknown source", found, state)
	}

	driveCapturedMail(t, e, capturedMailFrom(t, e, "late.writer@customer.test", false))

	if _, state, _, _ := noticeCaseFor(t, e, contact); state != "exempt_with_reason" {
		t.Errorf("after their own mail the case is %q, want exempt_with_reason: they wrote to us", state)
	}
	if !acquisitionKinds(t, e, contact)["subject_initiated"] {
		t.Error("the mail that settled the case left no subject_initiated acquisition to rest on")
	}

	// A redelivery changes nothing and adds no second acquisition.
	driveCapturedMail(t, e, capturedMailFrom(t, e, "late.writer@customer.test", false))
	var n int
	if err := e.Owner.QueryRow(context.Background(), `
		SELECT count(*) FROM contact_acquisition_evidence WHERE contact_id = $1 AND kind = 'subject_initiated'`,
		contact).Scan(&n); err != nil || n != 1 {
		t.Errorf("subject_initiated rows = %d (err %v), want exactly one", n, err)
	}
}

// Their mail arrived before the case would open: no case opens.
func TestTheirEarlierMailOpensNoCase(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contact := capturedUnknown(t, e, "Early Writer")
	withAddress(t, e, contact, "early.writer@customer.test")
	capturedMailFrom(t, e, "early.writer@customer.test", false)

	driveNoticeCase(t, e, contact)

	if _, _, _, found := noticeCaseFor(t, e, contact); found {
		t.Error("a contact whose own mail was already captured was recorded as owed a notice")
	}
}

// A newsletter is a list writing to everyone, not the contact writing to us.
func TestABulkMailSettlesNothing(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contact := capturedUnknown(t, e, "List Sender")
	withAddress(t, e, contact, "news@list.test")
	driveNoticeCase(t, e, contact)

	driveCapturedMail(t, e, capturedMailFrom(t, e, "news@list.test", true))

	if _, state, _, _ := noticeCaseFor(t, e, contact); state != "open" {
		t.Errorf("a bulk mail moved the case to %q, want it still open", state)
	}
}

// A duty opened because the address came from somebody else is not answered by
// the contact writing later: the address still came from a referral.
func TestAReferralStaysOwedAfterTheyWrite(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contact := contactAcquiredAs(t, e, "Referred", "referral")
	withAddress(t, e, contact, "referred@customer.test")
	driveNoticeCase(t, e, contact)

	driveCapturedMail(t, e, capturedMailFrom(t, e, "referred@customer.test", false))

	if _, state, _, _ := noticeCaseFor(t, e, contact); state != "open" {
		t.Errorf("a referral's case moved to %q after they wrote, want it still open", state)
	}
}

// A mail a seat logged by hand carries whatever From the seat typed. It must
// not close a legal duty: only a mail a connector took out of a real mailbox
// counts.
func TestAHandLoggedMailSettlesNothing(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contact := capturedUnknown(t, e, "Typed Sender")
	withAddress(t, e, contact, "typed@customer.test")
	driveNoticeCase(t, e, contact)

	driveCapturedMail(t, e, mailFrom(t, e, "typed@customer.test", false, "human:seed"))

	if _, state, _, _ := noticeCaseFor(t, e, contact); state != "open" {
		t.Errorf("a hand-logged mail moved the case to %q, want it still open", state)
	}
}

// An unknown source a seat stated is a claim about somewhere else, and the
// contact writing later does not answer it. Only capture's own unknown is.
func TestAnUnknownASeatStatedStaysOwed(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contact := contactAcquiredAs(t, e, "Seat Unknown", "unknown_legacy")
	withAddress(t, e, contact, "seat.unknown@customer.test")
	driveNoticeCase(t, e, contact)

	driveCapturedMail(t, e, capturedMailFrom(t, e, "seat.unknown@customer.test", false))

	if _, state, _, _ := noticeCaseFor(t, e, contact); state != "open" {
		t.Errorf("a seat-stated unknown moved to %q after they wrote, want it still open", state)
	}
}

// The counterparty verdict mints contacts from captured mail after the fact,
// and its unknown is capture's own as much as a connector's is.
func TestTheirMailSettlesTheCaseOfAContactTheVerdictMade(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contact := capturedUnknownBy(t, e, "Verdict Made", "agent:capture_counterparty_verdict")
	withAddress(t, e, contact, "verdict.made@customer.test")
	driveNoticeCase(t, e, contact)

	driveCapturedMail(t, e, capturedMailFrom(t, e, "verdict.made@customer.test", false))

	if _, state, _, _ := noticeCaseFor(t, e, contact); state != "exempt_with_reason" {
		t.Errorf("a verdict-made contact who wrote to us has a %q case, want exempt_with_reason", state)
	}
}

// Their mail was captured before the verdict made the contact: no duty opens,
// for a verdict-made contact as for a connector-made one.
func TestTheirEarlierMailOpensNoCaseForAContactTheVerdictMade(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contact := capturedUnknownBy(t, e, "Verdict Early", "agent:capture_counterparty_verdict")
	withAddress(t, e, contact, "verdict.early@customer.test")
	capturedMailFrom(t, e, "verdict.early@customer.test", false)

	driveNoticeCase(t, e, contact)

	if _, state, _, found := noticeCaseFor(t, e, contact); found && state == "open" {
		t.Error("a verdict-made contact who had already written to us was given an open notice case")
	}
}

// A contact withdrawn before its contact.created was handled had its duties
// ended; the late event must not open a fresh one.
func TestALateContactCreatedOpensNoCaseForAWithdrawnContact(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contact := capturedUnknown(t, e, "Withdrawn Early")
	withAddress(t, e, contact, "withdrawn.early@customer.test")
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE contact SET archived_at = now() WHERE id = $1`, contact); err != nil {
		t.Fatal(err)
	}

	driveNoticeCase(t, e, contact)

	if _, _, _, found := noticeCaseFor(t, e, contact); found {
		t.Error("a contact archived before contact.created was handled was given a notice case")
	}
}
