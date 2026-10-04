// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A commitment read out of an email goes to the same rule a meeting's does:
// the customer's is watched on their record, a colleague's becomes their task,
// and mail only one member may read stays theirs.

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// commitmentReply is the model naming one commitment on a message.
func commitmentReply(t *testing.T, message ids.UUID, quote, due string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"events": []map[string]any{{
		"kind": "commitment_made", "message_id": message.String(),
		"summary": "Send the revised pricing", "quote": quote, "due_date": due,
		"confidence": 0.95,
	}}})
	if err != nil {
		t.Fatalf("build the scripted reply: %v", err)
	}
	return string(body)
}

// party records who wrote or received a message, the way capture does.
func party(t *testing.T, message ids.UUID, role string, user, contact *ids.UUID) {
	t.Helper()
	if _, err := OwnerConn(t).Exec(t.Context(), `
		INSERT INTO activity_participant (activity_id, user_id, contact_id, role) VALUES ($1, $2, $3, $4)`,
		message, user, contact, role); err != nil {
		t.Fatalf("seed a participant: %v", err)
	}
}

const pricingPromise = "We will send the revised pricing over by Friday."

// The customer's promise is filed on them to watch, and never becomes a task.
func TestACustomersEmailedPromiseIsWatchedNotTasked(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := employeeOf(t, e, company, "Ines Huber")
	message := seedMessage(t, e, ines, "thread-volumes", "Volumes", "Thanks. "+pricingPromise, "inbound",
		extractClock.Add(-48*time.Hour))
	party(t, message, "from", nil, &ines)

	extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, message, pricingPromise, "2026-06-05")})

	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
		t.Errorf("the customer's promise became %d task(s)", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM conversation_claim
		WHERE kind = 'commitment_theirs' AND contact_id = $1 AND due_at IS NOT NULL`, ines); n != 1 {
		t.Errorf("want the customer's dated promise filed once on them, got %d claims", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM signal WHERE kind = 'commitment_made'`); n != 1 {
		t.Errorf("want the account's commitment signal kept for Deal Scout, got %d", n)
	}
	// The company page counts it, for a reader whose scope reaches everything.
	var open int
	var complete bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		open, complete, err = e.Contacts.CountAccountCommitments(e.Admin(), tx, company)
		return err
	}); err != nil || open != 1 || !complete {
		t.Errorf("the account counts %d open commitment(s), complete=%v, err=%v; want 1 and complete", open, complete, err)
	}
}

// A colleague's confident promise in their own mail is their task, dated as
// the mail said, and tied to the claim on the customer it was made to.
func TestAColleaguesEmailedPromiseIsTheirTask(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := employeeOf(t, e, company, "Ines Huber")
	message := seedMessage(t, e, ines, "thread-pricing", "Pricing", "Hi Ines, "+pricingPromise, "outbound",
		extractClock.Add(-48*time.Hour))
	party(t, message, "from", &e.Rep1, nil)
	party(t, message, "to", nil, &ines)

	extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, message, "send the revised pricing over by Friday", "2026-06-05")})

	task := e.WsScalar(t, `SELECT coalesce(assignee_id::text, '') || ' ' || captured_by || ' ' || to_char(due_at, 'YYYY-MM-DD')
		FROM activity WHERE kind = 'task'`)
	if want := e.Rep1.String() + " agent:signal-scan 2026-06-05"; task != want {
		t.Errorf("the task reads %q, want it held by the sender, written by the reader and due that Friday (%q)", task, want)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM conversation_claim
		WHERE kind = 'commitment_ours' AND contact_id = $1 AND task_activity_id IS NOT NULL`, ines); n != 1 {
		t.Errorf("want one claim on the customer pointing at the task, got %d", n)
	}

	// Reading the conversation again, quoting the whole sentence this time,
	// finds the same promise.
	e.WsExec(t, `DELETE FROM signal_thread_scan`)
	extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, message, pricingPromise, "2026-06-05")})
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 1 {
		t.Errorf("a second reading of one promise left %d tasks", n)
	}
}

// A promise in mail only one member may read is theirs alone: written by a
// colleague, it is proposed to the owner rather than handed to the colleague,
// and nothing about it is readable through the account.
func TestAPromiseInPrivateMailStaysWithItsOwner(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := employeeOf(t, e, company, "Ines Huber")
	e.WsExec(t, `UPDATE contact SET visibility = 'owner', owner_id = $2 WHERE id = $1`, ines, e.Rep1)
	message := seedMessage(t, e, ines, "thread-private", "Pricing", "Hi Ines, "+pricingPromise, "outbound",
		extractClock.Add(-48*time.Hour))
	party(t, message, "from", &e.Rep2, nil)
	party(t, message, "to", nil, &ines)

	extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, message, pricingPromise, "")})

	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
		t.Errorf("a promise in another member's private mail became %d task(s) for the colleague", n)
	}
	staged := e.WsScalar(t, `SELECT coalesce(on_behalf_of::text, '') FROM approval WHERE kind = 'commitment_task'`)
	if staged != e.Rep1.String() {
		t.Errorf("the proposal is staged for %q, want the private mail's owner", staged)
	}
}

// A customer's reply quoting our promise back to us is not their promise.
func TestAQuotedPromiseIsNotTheRepliersPromise(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := employeeOf(t, e, company, "Ines Huber")
	message := seedMessage(t, e, ines, "thread-quoted", "Re: Pricing",
		"Thanks, sounds good.\n\nOn Monday, Rep wrote:\n> "+pricingPromise, "inbound",
		extractClock.Add(-48*time.Hour))
	party(t, message, "from", nil, &ines)

	extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, message, pricingPromise, "")})

	if n := e.WsCount(t, `SELECT count(*) FROM conversation_claim`); n != 0 {
		t.Errorf("a promise the customer only quoted was filed as %d claim(s) of theirs", n)
	}
}

// A commitment signal a human filed is theirs, and the conversion leaves it.
func TestAHumanFiledCommitmentSignalIsNotConverted(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := employeeOf(t, e, company, "Ines Huber")
	cited := seedMessage(t, e, ines, "thread-filed", "Pricing", "Hi Ines, "+pricingPromise, "outbound",
		extractClock.Add(-72*time.Hour))
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := signals.RecordDerived(e.Admin(), tx, signals.DerivedSignal{
			Kind: "commitment_made", CompanyID: company, Summary: "Pricing promised.",
			Severity: "info", Fingerprint: "filed-" + cited.String(), Source: "manual",
			Evidence: []signals.DerivedEvidence{{Snippet: "Pricing promised.", ActivityID: cited}},
		}, extractClock.Add(-72*time.Hour))
		return err
	}); err != nil {
		t.Fatalf("seed the filed signal: %v", err)
	}
	if pass := extractPassStats(t, e, &scriptedBrain{reply: `{"events":[]}`}); pass.Converted != 0 {
		t.Errorf("the pass converted %d signal(s) a human filed", pass.Converted)
	}
}

// A promise its owner made in their own private mail is their task, and the
// task is visible to them alone.
func TestAPromiseInPrivateMailByItsOwnerIsTheirPrivateTask(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := employeeOf(t, e, company, "Ines Huber")
	e.WsExec(t, `UPDATE contact SET visibility = 'owner', owner_id = $2 WHERE id = $1`, ines, e.Rep1)
	message := seedMessage(t, e, ines, "thread-own", "Pricing", "Hi Ines, "+pricingPromise, "outbound",
		extractClock.Add(-48*time.Hour))
	party(t, message, "from", &e.Rep1, nil)
	party(t, message, "to", nil, &ines)

	extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, message, pricingPromise, "")})

	got := e.WsScalar(t, `SELECT coalesce(t.assignee_id::text, '') || ' ' || count(m.*)::text
		FROM activity t LEFT JOIN activity_audience_member m ON m.activity_id = t.id
		WHERE t.kind = 'task' GROUP BY t.id, t.assignee_id`)
	if want := e.Rep1.String() + " 1"; got != want {
		t.Errorf("the task reads %q, want it held by the owner with an audience of one (%q)", got, want)
	}
}

// An old signal whose message has since been archived is settled with
// nothing filed: there are no words left to file a claim on.
func TestAnOldSignalWhoseMessageIsGoneIsSettledEmpty(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := employeeOf(t, e, company, "Ines Huber")
	cited := seedMessage(t, e, ines, "thread-gone", "Pricing", "Hi Ines, "+pricingPromise, "outbound",
		extractClock.Add(-72*time.Hour))
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := signals.RecordDerived(e.Admin(), tx, signals.DerivedSignal{
			Kind: "commitment_made", CompanyID: company, Summary: "They promised pricing.",
			Severity: "info", Fingerprint: "legacy-" + cited.String(),
			Evidence: []signals.DerivedEvidence{{Snippet: "They promised pricing.", ActivityID: cited}},
		}, extractClock.Add(-72*time.Hour))
		return err
	}); err != nil {
		t.Fatalf("seed the old signal: %v", err)
	}
	e.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, cited)

	if pass := extractPassStats(t, e, &scriptedBrain{reply: commitmentReply(t, cited, pricingPromise, "")}); pass.Converted != 1 {
		t.Fatalf("the pass converted %d old signals, want the one", pass.Converted)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM conversation_claim`) + e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
		t.Errorf("a signal with no message left filed %d claim(s) and task(s)", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM signal WHERE status = 'open'`); n != 0 {
		t.Errorf("%d old signal(s) still open", n)
	}
}

// A commitment signal written before commitments had a rule is read again
// through it — around ITS message, however far back in the thread that is —
// and settled, once.
func TestAnOldCommitmentSignalIsReadThroughTheRuleOnce(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := employeeOf(t, e, company, "Ines Huber")
	start := extractClock.Add(-30 * 24 * time.Hour)
	var cited ids.UUID
	for i := range 9 {
		body, direction := "Status update.", "inbound"
		if i == 1 {
			body, direction = "Hi Ines, "+pricingPromise, "outbound"
		}
		id := seedMessage(t, e, ines, "thread-old", "Pricing", body, direction, start.Add(time.Duration(i)*time.Hour))
		if i == 1 {
			cited = id
			party(t, id, "from", &e.Rep1, nil)
			party(t, id, "to", nil, &ines)
		}
	}
	var signal ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := signals.RecordDerived(e.Admin(), tx, signals.DerivedSignal{
			Kind: "commitment_made", CompanyID: company, Summary: "They promised pricing.",
			Severity: "info", Fingerprint: "legacy-" + cited.String(),
			Evidence: []signals.DerivedEvidence{{Snippet: "They promised pricing.", ActivityID: cited}},
		}, start); err != nil {
			return err
		}
		return tx.QueryRow(e.Admin(), `SELECT id FROM signal WHERE kind = 'commitment_made'`).Scan(&signal)
	}); err != nil {
		t.Fatalf("seed the old signal: %v", err)
	}

	brain := &scriptedBrain{reply: commitmentReply(t, cited, pricingPromise, "")}
	if pass := extractPassStats(t, e, brain); pass.Converted != 1 {
		t.Fatalf("the pass converted %d old commitment signals, want 1", pass.Converted)
	}
	task := e.WsScalar(t, `SELECT coalesce(assignee_id::text, '') FROM activity
		WHERE kind = 'task' AND source_activity_id = $1`, cited)
	if task != e.Rep1.String() {
		t.Errorf("the old commitment became a task held by %q, want the colleague who sent it", task)
	}
	if got := e.WsScalar(t, `SELECT s.status || ' ' || r.source FROM signal s
		JOIN signal_resolution r ON r.signal_id = s.id WHERE s.id = $1`, signal); got != "acknowledged commitment_rule" {
		t.Errorf("the old signal reads %q, want it settled by the commitment rule", got)
	}

	// The ordinary read of the thread may be asked again — its newest window
	// does not hold the cited message, so that reading is refused and retried —
	// but the old signal is not read a second time.
	if pass := extractPassStats(t, e, brain); pass.Converted != 0 {
		t.Errorf("a second pass converted %d old signals; the one there was is done", pass.Converted)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 1 {
		t.Errorf("after a second pass there are %d tasks, want the one", n)
	}
}
