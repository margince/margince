// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The capture-health page reports a backlog nobody else can see.
//
// The contacts it counts are owner-private, and ownerPrivateTables makes them
// invisible to every reader but their owner — not even an administrator. So an
// admin looking for them finds nothing, and a count is the only way the backlog
// can be reported at all. That is the case worth holding: the page must see
// what its reader cannot.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCaptureHealthCountsABacklogAnAdminCannotSee(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)

	// A captured contact still owner-private, with no settled answer about its
	// sender: exactly the row the sweeps repair and nobody can see.
	contact := ids.NewV7()
	e.WsExec(t, `INSERT INTO contact (id, full_name, owner_id, visibility, captured_by, source)
		VALUES ($1, 'Waiting Contact', $2, 'owner', 'connector:gmail', 'capture')`, contact, e.Rep1)
	e.WsExec(t, `INSERT INTO contact_email (contact_id, email, source, captured_by) VALUES ($1, 'waiting@example.test', 'capture', 'connector:gmail')`, contact)
	// And a thread whose confidentiality question is still open.
	e.WsExec(t, `INSERT INTO capture_thread_verdict (thread_key, user_id, status)
		VALUES ('thread:open', $1, 'pending')`, e.Rep1)
	// The ledger rows point at the message that raised the question.
	activity := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Hello', now(), 'capture', 'connector:gmail')`, activity)

	// A sender the classifier has not answered, and one it gave up on.
	e.WsExec(t, `INSERT INTO capture_pending_counterparty (email, owner_id, activity_id, status, next_attempt_at)
		VALUES ('asked@example.test', $1, $2, 'pending', now())`, e.Rep1, activity)
	e.WsExec(t, `INSERT INTO capture_pending_counterparty (email, owner_id, activity_id, status, next_attempt_at)
		VALUES ('gaveup@example.test', $1, $2, 'pending', NULL)`, e.Rep1, activity)
	e.WsExec(t, `INSERT INTO capture_pending_counterparty (email, owner_id, activity_id, status)
		VALUES ('cannottell@example.test', $1, $2, 'unsure')`, e.Rep1, activity)

	var health crmcontracts.CaptureHealth
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		health, err = readCaptureHealth(ctx, tx, time.Now().UTC())
		return err
	}); err != nil {
		t.Fatalf("reading capture health: %v", err)
	}

	var mine *int
	for _, box := range health.Mailboxes {
		if ids.UUID(box.UserId) == e.Rep1 {
			contacts := box.ContactsAwaitingDecision
			mine = &contacts
			if box.ThreadsAwaitingVerdict != 1 {
				t.Errorf("threads awaiting a verdict = %d, want the one held thread",
					box.ThreadsAwaitingVerdict)
			}
			if box.OldestContactAgeSeconds == nil {
				t.Error("a mailbox with a waiting contact reports no age — absent means an " +
					"empty queue, which is the opposite of what this row says")
			}
		}
	}
	if mine == nil {
		t.Fatalf("the mailbox with the backlog is absent from %+v — a page that cannot see "+
			"an owner-private contact reports the same nothing an admin already sees",
			health.Mailboxes)
	}
	if *mine != 1 {
		t.Errorf("contacts awaiting a decision = %d, want the one owner-private contact", *mine)
	}

	// The three classifier states are counted apart, which is the reading that
	// distinguishes "the mail is hard" from "the machine is not running".
	if health.Classifier.Pending != 1 {
		t.Errorf("pending = %d, want the one still due", health.Classifier.Pending)
	}
	if health.Classifier.Exhausted != 1 {
		t.Errorf("exhausted = %d, want the one nothing will ask about again",
			health.Classifier.Exhausted)
	}
	if health.Classifier.Unsure != 1 {
		t.Errorf("unsure = %d, want the one waiting on a human", health.Classifier.Unsure)
	}
}

// seedCapturedContact writes a contact capture minted into rep's mailbox, still
// owner-private, with one address.
func seedOwnerPrivateContact(t *testing.T, e *integration.Env, owner ids.UUID, name, email string) ids.UUID {
	t.Helper()
	contact := ids.NewV7()
	e.WsExec(t, `INSERT INTO contact (id, full_name, owner_id, visibility, captured_by, source)
		VALUES ($1, $2, $3, 'owner', 'connector:gmail', 'capture')`, contact, name, owner)
	e.WsExec(t, `INSERT INTO contact_email (contact_id, email, source, captured_by)
		VALUES ($1, $2, 'capture', 'connector:gmail')`, contact, email)
	return contact
}

func ownerReader(e *integration.Env, user ids.UUID) context.Context {
	return e.As(user, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"contact": {Read: true}},
		RowScope: principal.RowScopeAll,
	})
}

// The owner's list and the admin's count read one predicate. A contact whose
// address has no sender entry at all is the case that splits them if either
// side is rebuilt from the senders list, which cannot see it.
func TestTheOwnersWaitingListIsWhatTheAdminCountCounts(t *testing.T) {
	e := integration.Setup(t)
	noEntry := seedOwnerPrivateContact(t, e, e.Rep1, "No Entry Yet", "noentry@waiting.test")
	pending := seedOwnerPrivateContact(t, e, e.Rep1, "Asked Not Answered", "pending@waiting.test")
	decided := seedOwnerPrivateContact(t, e, e.Rep1, "Answered", "decided@waiting.test")
	colleagues := seedOwnerPrivateContact(t, e, e.Rep3, "Someone Elses", "colleague@waiting.test")
	activity := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Hello', now(), 'capture', 'connector:gmail')`, activity)
	e.WsExec(t, `INSERT INTO capture_pending_counterparty (email, owner_id, activity_id, status, next_attempt_at)
		VALUES ('pending@waiting.test', $1, $2, 'pending', now())`, e.Rep1, activity)
	e.WsExec(t, `INSERT INTO capture_pending_counterparty (email, owner_id, activity_id, status)
		VALUES ('decided@waiting.test', $1, $2, 'real')`, e.Rep1, activity)

	listed, err := capture.ContactsAwaitingDecisionFor(ownerReader(e, e.Rep1), InstallationDB(e.Pool))
	if err != nil {
		t.Fatalf("the owner's list: %v", err)
	}
	var got []ids.UUID
	for _, c := range listed {
		got = append(got, c.ContactID)
	}
	if len(got) != 2 || !slices.Contains(got, noEntry) || !slices.Contains(got, pending) {
		t.Fatalf("the owner sees %v, want exactly the contact with no sender entry and the one still asked", got)
	}
	if slices.Contains(got, decided) || slices.Contains(got, colleagues) {
		t.Fatalf("the owner sees %v, which includes a decided contact or a colleague's", got)
	}

	for _, box := range readHealth(t, e).Mailboxes {
		if ids.UUID(box.UserId) == e.Rep1 && box.ContactsAwaitingDecision != len(listed) {
			t.Errorf("the admin counts %d for this mailbox and its owner is shown %d — the two "+
				"must be one set", box.ContactsAwaitingDecision, len(listed))
		}
	}

	senders, err := capture.SendersFor(ownerReader(e, e.Rep1), InstallationDB(e.Pool),
		capture.DefaultPersonalPurgeWindows())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range senders {
		if s.Address == "noentry@waiting.test" {
			t.Fatal("the senders list now carries the contact with no ledger row; this test no " +
				"longer shows why the owner needs a list of their own")
		}
	}
}

func TestTheOwnersWaitingListRefusesAnAgent(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		// Bound to the owner's seat, so only the human rung can refuse it.
		Type: principal.PrincipalAgent, ID: "agent:reader", UserID: e.Rep1,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{"contact": {Read: true}}, RowScope: principal.RowScopeAll,
		},
	})
	if _, err := capture.ContactsAwaitingDecisionFor(ctx, InstallationDB(e.Pool)); err == nil {
		t.Fatal("an agent read a seat's owner-private contacts")
	}
}

// The page names mailboxes and nothing inside them. Every string a seeded
// message, meeting, sender or hold carries is planted, and neither a key nor a
// value of the response may carry one.
func TestCaptureHealthCarriesNoCorrespondence(t *testing.T) {
	e := integration.Setup(t)
	seedOwnerPrivateContact(t, e, e.Rep1, "Planted Contact Name", "planted.sender@private.test")
	seedFiledHeldMeetings(t, e, 1)
	activity := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by, counterparty_email)
		VALUES ($1, 'email', 'Planted subject line', 'Planted body text', now(), 'capture',
		        'connector:gmail', 'planted.counterparty@private.test')`, activity)
	e.WsExec(t, `INSERT INTO capture_thread_verdict (thread_key, user_id, status, disposition_reason)
		VALUES ('thread:planted-key', $1, 'unsure', 'planted hold reason')`, e.Rep1)
	e.WsExec(t, `INSERT INTO capture_pending_counterparty (email, owner_id, activity_id, status)
		VALUES ('planted.pending@private.test', $1, $2, 'unsure')`, e.Rep1, activity)
	if err := capture.NewSweepLedger(InstallationDB(e.Pool)).RecordSweep(
		principal.WithWorkspaceID(context.Background(), e.WS),
		sweepReceiptFor(capture.SweepStrandedContacts, time.Now(), time.Now(), sweepTally{},
			errors.New("planted.error@private.test refused"))); err != nil {
		t.Fatal(err)
	}

	rec := serveCaptureHealth(t, e)
	body := rec.Body.String()
	for _, planted := range []string{
		"Planted", "planted", "private.test", "Board review",
		"severance", "confidential agenda", "attendee@filed.test", "thread:",
	} {
		if strings.Contains(body, planted) {
			t.Errorf("the response carries %q: %s", planted, body)
		}
	}
	keys := jsonKeys(rec.Body.Bytes())
	if !slices.Contains(keys, "held_meetings") {
		t.Fatalf("the key walk found %v, which misses the report's own fields", keys)
	}
	for _, key := range keys {
		for _, forbidden := range []string{"subject", "body", "email", "address", "reason", "thread_key"} {
			if strings.Contains(key, forbidden) {
				t.Errorf("the response has a %q key, which names correspondence", key)
			}
		}
	}
	if !strings.Contains(body, `"error_class":"unclassified"`) {
		t.Errorf("the failed receipt's class is missing or not the vetted token: %s", body)
	}
}

// jsonKeys lists every object key in a JSON document, at any depth.
func jsonKeys(raw json.RawMessage) []string {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) == nil {
		var out []string
		for key, child := range object {
			out = append(out, key)
			out = append(out, jsonKeys(child)...)
		}
		return out
	}
	var array []json.RawMessage
	if json.Unmarshal(raw, &array) == nil {
		var out []string
		for _, child := range array {
			out = append(out, jsonKeys(child)...)
		}
		return out
	}
	return nil
}

func serveCaptureHealth(t *testing.T, e *integration.Env) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/capture-health", nil)
	req = req.WithContext(e.As(e.AdminUser, nil, principal.Permissions{
		Objects: map[string]principal.ObjectGrant{"job_health": {Read: true}},
	}))
	rec := httptest.NewRecorder()
	captureHealthHandlers{pool: e.Pool, now: time.Now}.GetCaptureHealth(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	return rec
}

// A calm installation is the common case, and the card refuses a report whose
// required lists are null — so an empty backlog must travel as an empty list.
func TestACalmInstallationReportsEmptyListsNotNull(t *testing.T) {
	e := integration.Setup(t)

	var report map[string]json.RawMessage
	if err := json.Unmarshal(serveCaptureHealth(t, e).Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if got := string(report["mailboxes"]); got != "[]" {
		t.Errorf("mailboxes = %s, want [] — null makes the card refuse the whole report", got)
	}
	var sweeps []json.RawMessage
	if err := json.Unmarshal(report["sweeps"], &sweeps); err != nil || len(sweeps) != len(capture.Sweeps()) {
		t.Errorf("sweeps = %s, want one entry per pass even before any has run", report["sweeps"])
	}
}
