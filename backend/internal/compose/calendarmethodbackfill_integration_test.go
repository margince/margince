// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A contact's "Declined" mail, captured through the production sink, and the
// pass that reads the calendar method back for rows captured before capture
// recorded it.

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/capture/partslim"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// declineMail is the shape Google sends when an attendee declines: a text part
// and an unnamed text/calendar part whose Content-Type names the method.
func declineMail(to string, at time.Time) []byte {
	const from = "pat@customer.example"
	return []byte(strings.Join([]string{
		"From: " + from, "To: " + to,
		"Subject: Declined: Demo @ Thu 15:00 - 15:30 (CEST)",
		"Date: " + at.Format(time.RFC1123Z),
		"Message-ID: <decline-" + ids.NewV7().String() + "@customer.example>",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="b1"`,
		"",
		"--b1",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"Pat Buyer has declined this invitation.",
		"--b1",
		"Content-Type: text/calendar; charset=UTF-8; method=REPLY",
		"",
		"BEGIN:VCALENDAR",
		"METHOD:REPLY",
		"BEGIN:VEVENT",
		"ATTENDEE;PARTSTAT=DECLINED:mailto:" + from,
		"END:VEVENT",
		"END:VCALENDAR",
		"--b1--",
		"",
	}, "\r\n"))
}

func (o *owedEnv) captureRaw(t *testing.T, raw []byte) ids.UUID {
	t.Helper()
	parsed, err := mailmap.Parse(raw, o.seat)
	if err != nil {
		t.Fatalf("parsing the decline: %v", err)
	}
	ref, err := newCaptureSink(o.e.Pool, CaptureConfig{}).Upsert(
		threadConnectorCtx(o.e, o.owner), parsed.ToRecord("gmail", raw))
	if err != nil {
		t.Fatalf("capturing the decline: %v", err)
	}
	return ref.ID
}

func (o *owedEnv) calendarMethod(t *testing.T, message ids.UUID) string {
	t.Helper()
	var method string
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT coalesce(calendar_method, '') FROM activity WHERE id = $1`, message).Scan(&method)
	}); err != nil {
		t.Fatalf("reading the calendar method: %v", err)
	}
	return method
}

func runCalendarMethodBackfill(t *testing.T, o *owedEnv, blob blobstore.Store) int {
	t.Helper()
	files := activities.NewStore(InstallationDB(o.e.Pool)).WithBlobstore(blob)
	n, err := backfillCalendarMethodBatch(o.e.Admin(), o.e.Pool, files, calendarMethodBackfillPerTick, slog.Default())
	if err != nil {
		t.Fatalf("running the calendar method backfill: %v", err)
	}
	return n
}

// asCapturedBeforeTheColumn makes a captured row read as mail from before
// calendar_method existed: no method, created before its migration ran. age
// orders such rows among themselves.
func (o *owedEnv) asCapturedBeforeTheColumn(t *testing.T, message ids.UUID, age time.Duration) {
	t.Helper()
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE activity SET calendar_method = NULL,
			       created_at = (SELECT applied_at FROM schema_migrations_core WHERE version = $2) - $3::interval
			 WHERE id = $1`, message, activities.CalendarMethodMigration, fmt.Sprintf("%d seconds", int(age.Seconds())))
		return err
	}); err != nil {
		t.Fatalf("making the row read as captured before the column: %v", err)
	}
}

func TestACapturedDeclineIsOwedNothing(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	decline := o.captureRaw(t, declineMail(o.seat, o.now.Add(-time.Hour)))
	if got := o.calendarMethod(t, decline); got != "REPLY" {
		t.Fatalf("capture stored calendar method %q, want REPLY", got)
	}
	if o.owed(t, decline) {
		t.Error("a captured, unjudged decline is owed a reply")
	}
}

func TestTheBackfillReadsTheMethodOfALegacyDecline(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	decline := o.captureRaw(t, declineMail(o.seat, o.now.Add(-time.Hour)))
	o.asCapturedBeforeTheColumn(t, decline, time.Hour)
	if o.calendarMethod(t, decline) != "" || o.owed(t, decline) {
		t.Fatal("legacy unjudged decline must lack its method without claiming a reply")
	}

	if n := runCalendarMethodBackfill(t, o, nil); n != 1 {
		t.Fatalf("the pass settled %d messages, want 1", n)
	}
	if got := o.calendarMethod(t, decline); got != "REPLY" {
		t.Fatalf("the pass stored calendar method %q, want REPLY", got)
	}
	if n := o.count(t, `SELECT count(*) FROM activity_calendar_method_backfill
		WHERE activity_id = $1 AND outcome = 'set'`, decline); n != 1 {
		t.Errorf("the marker says set %d times, want 1", n)
	}
	if n := o.count(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'activity' AND entity_id = $1 AND after ? 'calendar_method'`, decline); n != 1 {
		t.Errorf("%d audit rows record the method, want 1", n)
	}
	if n := o.count(t, `SELECT count(*) FROM event_outbox
		WHERE envelope->'entity'->>'id' = $1::text
		  AND envelope->'payload'->'changed_fields'->>'calendar_method' = 'REPLY'`, decline); n != 1 {
		t.Errorf("%d events carry the method, want 1", n)
	}
	if o.owed(t, decline) {
		t.Error("the decline is still owed once its method was read back")
	}
	if n := runCalendarMethodBackfill(t, o, nil); n != 0 {
		t.Errorf("a second pass settled %d messages, want none", n)
	}
}

// outlookDecline is Outlook's shape: an attached invite.ics whose Content-Type
// names no method, so the method is only in the bytes the part sweep moves.
func outlookDecline(from, to string, at time.Time) []byte {
	ics := "BEGIN:VCALENDAR\r\nMETHOD:REPLY\r\nBEGIN:VEVENT\r\n" +
		strings.Repeat("DESCRIPTION:padding so the part sweep moves these bytes\r\n", 200) +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(ics))
	var wrapped []string
	for len(encoded) > 76 {
		wrapped = append(wrapped, encoded[:76])
		encoded = encoded[76:]
	}
	wrapped = append(wrapped, encoded)
	return []byte(strings.Join(append([]string{
		"From: " + from, "To: " + to,
		"Subject: Declined: Demo",
		"Date: " + at.Format(time.RFC1123Z),
		"Message-ID: <outlook-" + ids.NewV7().String() + "@customer.example>",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="b1"`,
		"",
		"--b1",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"Declined.",
		"--b1",
		`Content-Type: text/calendar; name="invite.ics"`,
		`Content-Disposition: attachment; filename="invite.ics"`,
		"Content-Transfer-Encoding: base64",
		"",
	}, append(wrapped, "--b1--", "")...), "\r\n"))
}

// captureWithFiles captures raw with its files kept in blob.
func (o *owedEnv) captureWithFiles(t *testing.T, blob blobstore.Store, raw []byte) ids.UUID {
	t.Helper()
	parsed, err := mailmap.Parse(raw, o.seat)
	if err != nil {
		t.Fatalf("parsing the mail: %v", err)
	}
	ref, err := newCaptureSink(o.e.Pool, CaptureConfig{Blob: blob}).Upsert(
		threadConnectorCtx(o.e, o.owner), parsed.ToRecord("gmail", raw))
	if err != nil {
		t.Fatalf("capturing the mail: %v", err)
	}
	return ref.ID
}

// slimmedOriginalOf slims every original and returns this message's, slimmed.
func (o *owedEnv) slimmedOriginalOf(t *testing.T, blob blobstore.Store, message ids.UUID) []byte {
	t.Helper()
	if _, err := capture.NewPartSlimStore(o.e.DB(), blob).SlimBatch(o.e.Admin(), 1000); err != nil {
		t.Fatalf("slimming: %v", err)
	}
	var payload []byte
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT rc.payload FROM raw_capture rc
			JOIN activity a ON `+storekit.StoredOriginalSQL("a", "rc")+`
			WHERE a.id = $1`, message).Scan(&payload)
	}); err != nil {
		t.Fatalf("reading the original: %v", err)
	}
	raw, err := decodeStoredOriginal(payload)
	if err != nil {
		t.Fatalf("decoding the original: %v", err)
	}
	if !partslim.IsSlimmed(raw) {
		t.Fatal("the part sweep did not slim the original; the pass would not reach the restore")
	}
	return raw
}

func TestTheBackfillRestoresASlimmedOriginalToReadItsMethod(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	blob := blobstore.NewMemory()
	decline := o.captureWithFiles(t, blob, outlookDecline("pat@customer.example", o.seat, o.now.Add(-time.Hour)))
	o.slimmedOriginalOf(t, blob, decline)
	o.asCapturedBeforeTheColumn(t, decline, time.Hour)
	if o.calendarMethod(t, decline) != "" || o.owed(t, decline) {
		t.Fatal("legacy unjudged decline must lack its method without claiming a reply")
	}

	if n := runCalendarMethodBackfill(t, o, blob); n != 1 {
		t.Fatalf("the pass settled %d messages, want 1", n)
	}
	if got := o.calendarMethod(t, decline); got != "REPLY" {
		t.Fatalf("the pass stored calendar method %q from a slimmed original, want REPLY", got)
	}
}

// Whether a decline is owed changes later, so every legacy row is read, owed
// or not, archived or not. A row under a hold is neither read nor marked.
func TestTheBackfillReadsEveryLegacyDeclineButAHeldOne(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	answered := o.captureRaw(t, declineMail(o.seat, o.now.Add(-4*time.Hour)))
	o.weWrite(t, owedMail{
		to: "pat@customer.example", subject: "Re: Declined: Demo @ Thu 15:00 - 15:30 (CEST)",
		at: o.now.Add(-3 * time.Hour), inReplyTo: o.messageIDOf(t, answered),
	})
	archived := o.captureRaw(t, declineMail(o.seat, o.now.Add(-2*time.Hour)))
	held := o.captureRaw(t, declineMail(o.seat, o.now.Add(-time.Hour)))
	o.asCapturedBeforeTheColumn(t, answered, 3*time.Hour)
	o.asCapturedBeforeTheColumn(t, archived, 2*time.Hour)
	o.asCapturedBeforeTheColumn(t, held, time.Hour)
	o.setRow(t, archived, "archived_at = now()")
	restrict(t, o.e, held)
	if o.owed(t, answered) {
		t.Fatal("the answered decline is owed; it would not show a row owed nothing is read")
	}

	if n := runCalendarMethodBackfill(t, o, nil); n != 2 {
		t.Fatalf("the pass settled %d messages, want 2", n)
	}
	for name, id := range map[string]ids.UUID{"answered": answered, "archived": archived} {
		if got := o.calendarMethod(t, id); got != "REPLY" {
			t.Errorf("the %s decline stored %q, want REPLY", name, got)
		}
	}
	if got := o.calendarMethod(t, held); got != "" {
		t.Errorf("the held decline was written: %q", got)
	}
	if n := o.count(t, `SELECT count(*) FROM activity_calendar_method_backfill WHERE activity_id = $1`, held); n != 0 {
		t.Errorf("the held decline was marked %d times, want 0", n)
	}
	if n := runCalendarMethodBackfill(t, o, nil); n != 0 {
		t.Errorf("a drained pass settled %d messages, want none", n)
	}
}

// setRow sets one fixture state on an activity row.
func (o *owedEnv) setRow(t *testing.T, message ids.UUID, assignment string) {
	t.Helper()
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE activity SET `+assignment+` WHERE id = $1`, message)
		return err
	}); err != nil {
		t.Fatalf("setting %s: %v", assignment, err)
	}
}

func (o *owedEnv) messageIDOf(t *testing.T, message ids.UUID) string {
	t.Helper()
	var id string
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT source_id FROM activity WHERE id = $1`, message).Scan(&id)
	}); err != nil {
		t.Fatalf("reading the message id: %v", err)
	}
	return id
}

// The stanza naming a moved part sits in mail a sender wrote. A sender who
// copies another message's stanza must not get that message's object read.
func TestTheBackfillReadsOnlyTheMessagesOwnStoredParts(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	blob := blobstore.NewMemory()
	victim := o.captureWithFiles(t, blob, outlookDecline("pat@customer.example", o.seat, o.now.Add(-2*time.Hour)))
	stolen := o.slimmedOriginalOf(t, blob, victim)
	forged := bytes.Replace(stolen, []byte("Message-ID: <outlook-"), []byte("Message-ID: <forged-"), 1)
	forgery := o.captureWithFiles(t, blob, forged)
	o.asCapturedBeforeTheColumn(t, forgery, time.Hour)
	if o.calendarMethod(t, forgery) != "" || o.owed(t, forgery) {
		t.Fatal("unjudged forged mail must lack its method without claiming a reply")
	}

	runCalendarMethodBackfill(t, o, blob)
	if got := o.calendarMethod(t, forgery); got != "" {
		t.Errorf("the pass stored %q, read from another message's object", got)
	}
	if n := o.count(t, `SELECT count(*) FROM activity_calendar_method_backfill
		WHERE activity_id = $1 AND outcome = 'unreadable'`, forgery); n != 1 {
		t.Errorf("the marker says unreadable %d times, want 1", n)
	}
}
