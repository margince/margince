// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// markedVendor lands a vendor_pitch verdict for sender, so the contact carries
// the mark the sender check sets.
func markedVendor(t *testing.T, e *integration.Env, sender, domain string) {
	t.Helper()
	mail := seedCapturedMail(t, e, sender, "Free audit")
	row := seedPendingDisposition(t, e, sender, domain, mail)
	runVerdict(t, e, &scriptedVerdictBrain{verdicts: map[string]string{row.String(): capture.KindVendorPitch}})
	if mark, _, _ := markOf(t, e, sender); mark != "vendor_marketing" {
		t.Fatalf("setup: %s carries mark %q, want vendor_marketing", sender, mark)
	}
}

// sendFiled records one attested outbound mail to the address and files it under
// the contact (nil means the contact holding the address). age is how long ago it was sent.
func sendFiled(t *testing.T, e *integration.Env, sourceID, address, thread, body string, under *ids.UUID, age time.Duration) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var activity ids.UUID
		if err := tx.QueryRow(context.Background(), `
			INSERT INTO activity (kind, subject, body, direction, source_system, source_id,
			                      source, captured_by, thread_key, counterparty_email,
			                      counterparty_outbound_attested, occurred_at)
			VALUES ('email', 'Re: Free audit', $4, 'outbound', 'gmail', $1,
			        'gmail:'||$1, 'connector:gmail', $3, $2, true, now() - make_interval(secs => $5))
			RETURNING id`, sourceID, address, thread, body, age.Seconds()).Scan(&activity); err != nil {
			return err
		}
		contact := under
		if contact == nil {
			var held ids.UUID
			if err := tx.QueryRow(context.Background(),
				`SELECT contact_id FROM contact_email WHERE email = $1`, address).Scan(&held); err != nil {
				return err
			}
			contact = &held
		}
		_, err := tx.Exec(context.Background(),
			`INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`,
			activity, *contact)
		return err
	}); err != nil {
		t.Fatalf("filing the workspace's mail to %s: %v", address, err)
	}
}

func liftMarks(t *testing.T, e *integration.Env) {
	t.Helper()
	engine := NewCounterpartyVerdictEngine(e.Pool, &scriptedVerdictBrain{}, CaptureConfig{}, slog.Default())
	if err := engine.LiftVendorMarksWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
		t.Fatalf("lifting vendor marks: %v", err)
	}
}

func markWrites(t *testing.T, e *integration.Env, email string) (audits, events int) {
	t.Helper()
	const contactOf = `(SELECT contact_id FROM contact_email WHERE email = $1)`
	audits = countIn(t, e, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'contact' AND entity_id = `+contactOf+` AND action = 'update'
		  AND before ? 'relationship_mark'`, email)
	events = countIn(t, e, `SELECT count(*) FROM event_outbox o JOIN audit_log a
		    ON a.id = (o.envelope->'trace'->>'audit_log_id')::uuid
		WHERE a.entity_id = `+contactOf+` AND a.before ? 'relationship_mark'
		  AND o.envelope->>'type' = 'contact.updated'`, email)
	return audits, events
}

func TestAnAutomaticVendorMarkEndsOnceTheWorkspaceWritesToTheContact(t *testing.T) {
	e := integration.Setup(t)
	const sender = "dana@seo-growth.example"
	markedVendor(t, e, sender, "seo-growth.example")

	liftMarks(t, e)
	if mark, _, _ := markOf(t, e, sender); mark == "" {
		t.Fatal("the mark was cleared with no correspondence")
	}

	sendFiled(t, e, "lift-out-1", sender, "lift-t1", "Anbei.", nil, 0)
	sendFiled(t, e, "lift-out-2", sender, "lift-t2", "Anbei.", nil, 0)
	liftMarks(t, e)

	mark, setBy, _ := markOf(t, e, sender)
	if mark != "" || setBy != "" {
		t.Errorf("after the workspace wrote twice the contact has mark %q set by %q, want none", mark, setBy)
	}
	audits, events := markWrites(t, e, sender)
	if audits != 1 || events != 1 {
		t.Errorf("clearing wrote %d audit rows and %d events, want 1 and 1", audits, events)
	}

	liftMarks(t, e)
	if a, ev := markWrites(t, e, sender); a != audits || ev != events {
		t.Errorf("a second pass wrote again: %d audit rows, %d events", a, ev)
	}
}

func TestAHandSetMarkSurvivesCorrespondence(t *testing.T) {
	e := integration.Setup(t)
	const sender = "kai@listbuilder.example"
	markedVendor(t, e, sender, "listbuilder.example")
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE contact SET relationship_mark_set_by = 'human:' || $2::text
			 WHERE id = (SELECT contact_id FROM contact_email WHERE email = $1)`, sender, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("recording a hand-set mark: %v", err)
	}

	sendFiled(t, e, "hand-out-1", sender, "hand-t1", "Anbei.", nil, 0)
	sendFiled(t, e, "hand-out-2", sender, "hand-t2", "Anbei.", nil, 0)
	liftMarks(t, e)

	if mark, _, _ := markOf(t, e, sender); mark != "vendor_marketing" {
		t.Errorf("a mark a human set was cleared by correspondence (mark %q)", mark)
	}
}

func TestOneDecliningReplyDoesNotEndTheMark(t *testing.T) {
	e := integration.Setup(t)
	const sender = "lee@coldreach.example"
	markedVendor(t, e, sender, "coldreach.example")

	sendFiled(t, e, "decline-1", sender, "decline-t", "Not interested, please remove me.", nil, 0)
	liftMarks(t, e)

	if mark, _, _ := markOf(t, e, sender); mark != "vendor_marketing" {
		t.Errorf("one declining reply ended the mark (mark %q); it is not a correspondence", mark)
	}
}

func TestAVendorsAnswerToOurDeclineDoesNotEndItsMark(t *testing.T) {
	e := integration.Setup(t)
	const sender = "ines@coldreach.example"
	markedVendor(t, e, sender, "coldreach.example")

	sendFiled(t, e, "decl-out", sender, "decl-t", "No thanks, not interested.", nil, 0)
	captureInboundThroughRealSink(t, e, e.Rep1, "decl-in", sender, "decl-t")
	liftMarks(t, e)

	if mark, _, _ := markOf(t, e, sender); mark != "vendor_marketing" {
		t.Errorf("the vendor lifted its own mark by answering our decline (mark %q)", mark)
	}
}

func TestOnlyMailAfterTheMarkAndFiledUnderTheContactEndsIt(t *testing.T) {
	e := integration.Setup(t)
	const sender, other = "pia@mixedmail.example", "someone@elsewhere.example"
	markedVendor(t, e, sender, "mixedmail.example")
	otherID := seedContactWithEmail(t, e, "Else Where", other)

	before := 48 * time.Hour
	sendFiled(t, e, "old-1", sender, "old-t1", "Anbei.", nil, before)
	sendFiled(t, e, "old-2", sender, "old-t2", "Anbei.", nil, before)
	sendFiled(t, e, "wrong-1", sender, "wrong-t1", "Anbei.", &otherID, 0)
	sendFiled(t, e, "wrong-2", sender, "wrong-t2", "Anbei.", &otherID, 0)
	liftMarks(t, e)
	if mark, _, _ := markOf(t, e, sender); mark != "vendor_marketing" {
		t.Fatalf("old mail or mail filed under another contact ended the mark (mark %q)", mark)
	}

	sendFiled(t, e, "now-1", sender, "now-t1", "Anbei.", nil, 0)
	sendFiled(t, e, "now-2", sender, "now-t2", "Anbei.", nil, 0)
	liftMarks(t, e)
	if mark, _, _ := markOf(t, e, sender); mark != "" {
		t.Errorf("current mail filed under the contact left the mark %q", mark)
	}
}

func TestAnAddressThatMovedAwayLeavesTheOldContactsMark(t *testing.T) {
	e := integration.Setup(t)
	const sender = "rae@moved.example"
	markedVendor(t, e, sender, "moved.example")
	oldID := contactIDOf(t, e, sender)
	sendFiled(t, e, "mv-1", sender, "mv-t1", "Anbei.", nil, 0)
	sendFiled(t, e, "mv-2", sender, "mv-t2", "Anbei.", nil, 0)

	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE contact_email SET archived_at = now() WHERE contact_id = $1`, oldID)
		return err
	}); err != nil {
		t.Fatalf("moving the address away: %v", err)
	}
	err := database.WithWorkspaceTx(principal.WithWorkspaceID(e.Admin(), e.WS), e.Pool, func(tx pgx.Tx) error {
		return liftVendorMarkTx(e.Admin(), tx, newCounterpartyStore(e.Pool), ids.From[ids.ContactKind](oldID), sender)
	})
	if err != nil {
		t.Fatalf("clearing with a stale (contact, address) pair: %v", err)
	}
	var mark *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT relationship_mark FROM contact WHERE id = $1`, oldID).Scan(&mark)
	}); err != nil {
		t.Fatalf("reading the mark: %v", err)
	}
	if mark == nil {
		t.Error("the old contact lost its mark on evidence about an address it no longer holds")
	}
}

func TestLaterMailFromAMarkedContactClearsAnAutomaticMarkOnceTheyAnswer(t *testing.T) {
	e := integration.Setup(t)
	const sender = "robin@outreach.example"
	markedVendor(t, e, sender, "outreach.example")

	sendFiled(t, e, "hook-out-1", sender, "hook-t1", "Anbei.", nil, 0)
	captureInboundThroughRealSink(t, e, e.Rep1, "hook-in-1", sender, "hook-t1")

	if mark, _, _ := markOf(t, e, sender); mark != "" {
		t.Errorf("a reply on a thread the workspace started left the mark %q in place", mark)
	}
}

// A marked contact's request is not a buyer waiting, and the lane has it again
// once the mark is gone.
func TestAMarkedContactsMessageIsNotOnTheWaitingLane(t *testing.T) {
	o := setupOwed(t)
	pat := o.contact(t, "Pat Seller", "pat@seller.example")
	asked := o.customerWrites(t, "pat@seller.example", "Pricing for next year", o.now.Add(-3*time.Hour))
	o.judgeAsRequest(t, asked)
	if !o.waiting(t, asked) {
		t.Fatal("before the mark the message is not waiting; the case would pass for the wrong reason")
	}

	setMark := func(mark any) {
		t.Helper()
		if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
			_, err := tx.Exec(context.Background(), `
				UPDATE contact SET relationship_mark = $2,
				       relationship_mark_set_by = CASE WHEN $2::text IS NULL THEN NULL ELSE $3 END
				 WHERE id = $1`, pat, mark, verdictActor)
			return err
		}); err != nil {
			t.Fatalf("setting the mark: %v", err)
		}
	}
	setMark("vendor_marketing")
	if o.waiting(t, asked) {
		t.Fatal("a vendor_marketing contact's message stayed on the waiting lane")
	}
	setMark(nil)
	if !o.waiting(t, asked) {
		t.Fatal("clearing the mark did not bring the message back to the waiting lane")
	}
}

// The reconnect lane reads the contact's mark and nothing else about a vendor:
// the per-owner ledger keeps its vendor_pitch kind after the mark is cleared.
func TestTheReconnectLaneFollowsTheMarkNotTheLedgerKind(t *testing.T) {
	e := integration.Setup(t)
	const sender = "sam@bulkreach.example"
	markedVendor(t, e, sender, "bulkreach.example")
	allowed := func() bool {
		t.Helper()
		return countIn(t, e, `SELECT count(*) FROM contact_email e
			WHERE e.email = $1 AND `+capture.PrivateSenderClause("e", "$2"), sender, e.Rep1) == 1
	}
	if allowed() {
		t.Fatal("a marked vendor reached the reconnect lane")
	}

	sendFiled(t, e, "rc-out-1", sender, "rc-t1", "Anbei.", nil, 0)
	sendFiled(t, e, "rc-out-2", sender, "rc-t2", "Anbei.", nil, 0)
	liftMarks(t, e)
	if kind := countIn(t, e, `SELECT count(*) FROM capture_pending_counterparty
		WHERE email = $1 AND kind = 'vendor_pitch'`, sender); kind != 1 {
		t.Fatalf("setup: the ledger holds %d vendor_pitch rows, want 1", kind)
	}
	if !allowed() {
		t.Error("a contact whose mark was cleared stayed out of the reconnect lane because of its old ledger kind")
	}
}

func contactIDOf(t *testing.T, e *integration.Env, email string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT contact_id FROM contact_email WHERE email = $1`, email).Scan(&id)
	}); err != nil {
		t.Fatalf("finding the contact of %s: %v", email, err)
	}
	return id
}
