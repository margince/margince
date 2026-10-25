// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A commitment read out of an email goes to the same rule a meeting's does:
// the customer's is watched on their record, a colleague's becomes their task,
// and mail only one member may read stays theirs.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// commitmentReply is the model naming one commitment on a message.
func commitmentReply(t *testing.T, message ids.UUID, promiser, quote, due string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"events": []map[string]any{{
		"kind": "commitment_made", "message_id": message.String(), "promiser_ref": promiser,
		"summary": "Send the revised pricing", "quote": quote, "due_date": due,
		"confidence": 0.95,
		"commitment": OpenCommitmentMeaning(due, quote),
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
		INSERT INTO activity_participant (activity_id, user_id, contact_id, role) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
		message, user, contact, role); err != nil {
		t.Fatalf("seed a participant: %v", err)
	}
}

const pricingPromise = "We will send the revised pricing over by Friday."

// The customer's promise is filed on them to watch, and never becomes a task.
func TestACustomersEmailedPromiseIsWatchedNotTasked(t *testing.T) {
	e := Setup(t)
	e.Contacts.WithClock(func() time.Time { return extractClock })
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := employeeOf(t, e, company, "Ines Huber")
	message := captureCommitmentMessage(t, e, ines, "thread-volumes", "Volumes", "Thanks. "+pricingPromise, "inbound",
		extractClock.Add(-48*time.Hour))
	party(t, message, "from", nil, &ines)

	extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, message, "contact:"+ines.String(), pricingPromise, "2026-06-05")})

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
	message := captureCommitmentMessage(t, e, ines, "thread-pricing", "Pricing", "Hi Ines, "+pricingPromise, "outbound",
		extractClock.Add(-48*time.Hour))
	party(t, message, "from", &e.Rep1, nil)
	party(t, message, "to", nil, &ines)

	extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, message, "seat:"+e.Rep1.String(), "send the revised pricing over by Friday", "2026-06-05")})

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
	extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, message, "seat:"+e.Rep1.String(), pricingPromise, "2026-06-05")})
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 1 {
		t.Errorf("a second reading of one promise left %d tasks", n)
	}
}

// A promise a colleague wrote in mail only another member may read asks
// nobody. The colleague gets no task, the mailbox owner gets no request, and
// the finding stays on the message.
func TestAPromiseInPrivateMailStaysWithItsOwner(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := employeeOf(t, e, company, "Ines Huber")
	e.WsExec(t, `UPDATE contact SET visibility = 'owner', owner_id = $2 WHERE id = $1`, ines, e.Rep1)
	message := captureCommitmentMessage(t, e, ines, "thread-private", "Pricing", "Hi Ines, "+pricingPromise, "outbound",
		extractClock.Add(-48*time.Hour))
	party(t, message, "from", &e.Rep2, nil)
	party(t, message, "to", nil, &ines)

	extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, message, "seat:"+e.Rep2.String(), pricingPromise, "")})

	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
		t.Errorf("a promise in another member's private mail became %d task(s) for the colleague", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'commitment_task'`); n != 0 {
		t.Errorf("%d request(s) staged for a promise nobody may be asked about, want none", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission
		WHERE outcome = 'promiser_not_assignable' AND NOT current_work`); n != 1 {
		t.Errorf("the promise is recorded %d time(s) on the message, want once", n)
	}
}

// Mail from our seat that names a colleague not on the message as the one who
// acts reports no promiser. The sender gets no task and nobody is asked; the
// finding stays on the message.
func TestACommitmentForAColleagueNotOnTheMailFilesNoTaskForTheSender(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := employeeOf(t, e, company, "Ines Huber")
	const demo = "Product demo on the 5th at 10:00, online with Robin."
	message := captureCommitmentMessage(t, e, ines, "thread-demo", "Product demo", "Hi Ines, "+demo, "outbound",
		extractClock.Add(-48*time.Hour))
	party(t, message, "from", &e.Rep1, nil)
	party(t, message, "to", nil, &ines)

	extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, message, "", demo, "")})

	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
		t.Errorf("a demo Robin runs became %d task(s) for the sender", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'commitment_task'`); n != 0 {
		t.Errorf("%d request(s) staged for a promise with no promiser, want none", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission
		WHERE outcome = 'promiser_not_assignable' AND NOT current_work`); n != 1 {
		t.Errorf("the promise is recorded %d time(s) on the message, want once", n)
	}
}

// A customer's reply quoting our promise back to us is not their promise.
func TestAQuotedPromiseIsNotTheRepliersPromise(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"quoted reply", "Thanks, sounds good.\n\nOn Monday, Rep wrote:\n> " + pricingPromise},
		{"Apple forward", "FYI.\n\nBegin forwarded message:\nFrom: Rep <rep@workspace.example>\n\n" + pricingPromise},
		{"Vietnamese forward", "FYI.\n\nThư được chuyển tiếp:\nFrom: Rep <rep@workspace.example>\n\n" + pricingPromise},
		{"headers after own words", "FYI.\n\nFrom: Rep <rep@workspace.example>\nDate: Monday\n\n" + pricingPromise},
		{"forwarded message", "For your information.\n\n---------- Forwarded message ---------\nFrom: Rep <rep@workspace.example>\nSubject: Pricing\n\n" + pricingPromise},
	} {
		for _, direction := range []string{"inbound", "outbound"} {
			t.Run(tc.name+"/"+direction, func(t *testing.T) {
				e := Setup(t)
				company := e.SeedCompany(t, "Acme", &e.Rep1)
				ines := employeeOf(t, e, company, "Ines Huber")
				message := captureCommitmentMessage(t, e, ines, "thread-quoted", "Re: Pricing", tc.body, direction, extractClock.Add(-48*time.Hour))
				promiser := "contact:" + ines.String()
				if direction == "outbound" {
					party(t, message, "from", &e.Rep1, nil)
					promiser = "seat:" + e.Rep1.String()
				} else {
					party(t, message, "from", nil, &ines)
				}
				extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, message, promiser, pricingPromise, "")})
				if n := e.WsCount(t, `SELECT count(*) FROM conversation_claim`); n != 0 {
					t.Errorf("quoted or forwarded words became %d new claims", n)
				}
				if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task'`); n != 0 {
					t.Errorf("quoted or forwarded words became %d new tasks", n)
				}
			})
		}
	}
}

// An old signal is settled even when the reading finds its commitment in
// another message: only a commitment on the message the signal cited is filed.
func TestAnOldSignalReadElsewhereIsSettledEmpty(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := employeeOf(t, e, company, "Ines Huber")
	earlier := captureCommitmentMessage(t, e, ines, "thread-elsewhere", "Pricing", "Hi Ines, "+pricingPromise, "outbound",
		extractClock.Add(-96*time.Hour))
	party(t, earlier, "from", &e.Rep1, nil)
	cited := captureCommitmentMessage(t, e, ines, "thread-elsewhere", "Re: Pricing", "Thanks, noted.", "inbound",
		extractClock.Add(-72*time.Hour))
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := signals.RecordDerived(e.Admin(), tx, signals.DerivedSignal{
			Kind: "commitment_made", CompanyID: company, Summary: "Pricing promised.",
			Severity: "info", Fingerprint: "legacy-" + cited.String(),
			Evidence: []signals.DerivedEvidence{{Snippet: "Pricing promised.", ActivityID: cited}},
		}, extractClock.Add(-72*time.Hour))
		return err
	}); err != nil {
		t.Fatalf("seed the old signal: %v", err)
	}
	brain := &scriptedBrain{reply: commitmentReply(t, earlier, "seat:"+e.Rep1.String(), pricingPromise, "")}
	if pass := extractPassStats(t, e, brain); pass.Converted != 1 {
		t.Fatalf("the pass converted %d old signals, want the one", pass.Converted)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE source_activity_id = $1`, cited); n != 0 {
		t.Errorf("a commitment found on another message was filed against the cited one %d time(s)", n)
	}
}

// A commitment signal a human filed is theirs, and the conversion leaves it.
func TestAHumanFiledCommitmentSignalIsNotConverted(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := employeeOf(t, e, company, "Ines Huber")
	cited := captureCommitmentMessage(t, e, ines, "thread-filed", "Pricing", "Hi Ines, "+pricingPromise, "outbound",
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
	message := captureCommitmentMessage(t, e, ines, "thread-own", "Pricing", "Hi Ines, "+pricingPromise, "outbound",
		extractClock.Add(-48*time.Hour))
	party(t, message, "from", &e.Rep1, nil)
	party(t, message, "to", nil, &ines)

	extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, message, "seat:"+e.Rep1.String(), pricingPromise, "")})

	got := e.WsScalar(t, `SELECT coalesce(t.assignee_id::text, '') || ' ' ||
		       coalesce(string_agg(m.subject_type || ':' || m.subject_id::text, ','), '')
		FROM activity t LEFT JOIN activity_audience_member m ON m.activity_id = t.id
		WHERE t.kind = 'task' GROUP BY t.id, t.assignee_id`)
	if want := e.Rep1.String() + " user:" + e.Rep1.String(); got != want {
		t.Errorf("the task reads %q, want it held by the owner with an audience of one (%q)", got, want)
	}
}

// An old signal whose message has since been archived is settled with
// nothing filed: there are no words left to file a claim on.
func TestAnOldSignalWhoseMessageIsGoneIsSettledEmpty(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := employeeOf(t, e, company, "Ines Huber")
	cited := captureCommitmentMessage(t, e, ines, "thread-gone", "Pricing", "Hi Ines, "+pricingPromise, "outbound",
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

	if pass := extractPassStats(t, e, &scriptedBrain{reply: commitmentReply(t, cited, "seat:"+e.Rep1.String(), pricingPromise, "")}); pass.Converted != 1 {
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
	for _, test := range []struct {
		name     string
		messages int
		reason   string
	}{
		{"complete history", 2, "historical_without_current_deadline"},
		{"incomplete history", 30, "later_context_not_read"},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := Setup(t)
			company := e.SeedCompany(t, "Acme", &e.Rep1)
			ines := employeeOf(t, e, company, "Ines Huber")
			start := extractClock.Add(-45 * 24 * time.Hour)
			var cited ids.UUID
			for i := range test.messages {
				body, direction := "Status update.", "inbound"
				if i == 1 {
					body, direction = "Hi Ines, "+pricingPromise, "outbound"
				}
				id := captureCommitmentMessage(t, e, ines, "thread-old", "Pricing", body, direction, start.Add(time.Duration(i)*time.Hour))
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

			brain := &scriptedBrain{reply: commitmentReply(t, cited, "seat:"+e.Rep1.String(), pricingPromise, "")}
			if pass := extractPassStats(t, e, brain); pass.Converted != 1 {
				t.Fatalf("the pass converted %d old commitment signals, want 1", pass.Converted)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task' AND source_activity_id = $1`, cited); n != 0 {
				t.Fatalf("historical evidence created %d new tasks", n)
			}

			if got := e.WsScalar(t, `SELECT s.status || ' ' || r.source FROM signal s
		JOIN signal_resolution r ON r.signal_id = s.id WHERE s.id = $1`, signal); got != "acknowledged commitment_rule" {
				t.Errorf("the old signal reads %q, want it settled by the commitment rule", got)
			}

			// A retry may read the thread again, but cannot convert the old signal twice.
			if pass := extractPassStats(t, e, brain); pass.Converted != 0 {
				t.Errorf("a second pass converted %d old signals; the one there was is done", pass.Converted)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
				t.Errorf("historical replay created %d tasks", n)
			}
			if got := e.WsScalar(t, `SELECT outcome FROM activity_commitment_admission WHERE source_activity_id = $1`, cited); got != test.reason {
				t.Fatalf("legacy disposition = %q, want %q", got, test.reason)
			}
		})
	}
}

func TestACurrentQuotedPromiseSurvivesALongSignature(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	buyer := employeeOf(t, e, company, "Pat Buyer")
	body := pricingPromise + "\n\n" + strings.Repeat("Signature and legal footer. ", 100)
	message := captureCommitmentMessage(t, e, buyer, "long-signature", "Pricing", body, "outbound", extractClock.Add(-48*time.Hour))
	party(t, message, "from", &e.Rep1, nil)
	party(t, message, "to", nil, &buyer)
	extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, message, "seat:"+e.Rep1.String(), pricingPromise, "2026-06-05")})
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task' AND assignee_id=$1`, e.Rep1); got != 1 {
		t.Fatalf("a quoted current promise with a long footer produced %d tasks", got)
	}
}

type commitmentMailRecipient struct {
	contact ids.UUID
	role    string
}

func commitmentMailAddress(t *testing.T, e *Env, contact ids.UUID) string {
	t.Helper()
	address := "buyer-" + contact.String() + "@buyer.example"
	if _, err := e.Contacts.UpdateContact(e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms), ids.From[ids.ContactKind](contact), contacts.UpdateContactInput{Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}}}); err != nil {
		t.Fatal(err)
	}
	return address
}

func captureCommitmentMessage(t *testing.T, e *Env, contact ids.UUID, key, subject, body, direction string, at time.Time, recipients ...commitmentMailRecipient) ids.UUID {
	t.Helper()
	owner, buyer := "rep@workspace.example", commitmentMailAddress(t, e, contact)
	e.WsExec(t, `INSERT INTO capture_connection (user_id,provider,status,credential_ref,account_label) VALUES ($1,'gmail','connected','vault:test',$2) ON CONFLICT (user_id,provider) DO UPDATE SET account_label=$2`, e.Rep1, owner)
	from, to := buyer, owner
	if direction == "outbound" {
		from, to = owner, buyer
	}
	var headers, toAddresses strings.Builder
	toAddresses.WriteString(to)
	for _, recipient := range recipients {
		address := commitmentMailAddress(t, e, recipient.contact)
		if recipient.role == "cc" {
			headers.WriteString("Cc: " + address + "\r\n")
		} else {
			toAddresses.WriteString(", " + address)
		}
	}
	raw := []byte(headers.String() + fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%s@buyer.example>\r\nReferences: <%s>\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s", from, toAddresses.String(), subject, at.Format(time.RFC1123Z), ids.NewV7(), key, body))
	message, err := mailmap.Parse(raw, owner)
	if err != nil {
		t.Fatal(err)
	}
	record := message.ToRecord("gmail", raw)
	ctx := principal.WithActor(connectorCtx(e, e.Rep1), principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:gmail", UserID: e.Rep1, OnBehalfOf: e.Rep1,
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"activity": {Create: true, Read: true}}, RowScope: principal.RowScopeAll},
	})
	ref, err := capture.NewSink(e.DB()).Upsert(ctx, record)
	if err != nil {
		t.Fatal(err)
	}
	LinkActivity(t, OwnerConn(t), ref.ID, "contact", contact)
	reader := principal.WithActor(connectorCtx(e, e.Rep1), principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1, SeatType: principal.SeatFull, Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"activity": {Read: true, Update: true}}, RowScope: principal.RowScopeAll}})
	if _, err := compose.NewThreadAudienceSetter(e.Pool).Decide(reader, key, true); err != nil {
		t.Fatal(err)
	}
	return ref.ID
}

func TestLaterReplyDistinguishesUnreadContentFromQuotedHistory(t *testing.T) {
	for _, kind := range []string{"quoted history", "unread current text", "inline answer after quoted history"} {
		t.Run(kind, func(t *testing.T) {
			e := Setup(t)
			company := e.SeedCompany(t, "Acme", &e.Rep1)
			buyer := employeeOf(t, e, company, "Pat Buyer")
			message := captureCommitmentMessage(t, e, buyer, "reply-tail", "Pricing", pricingPromise, "outbound", extractClock.Add(-48*time.Hour))
			party(t, message, "from", &e.Rep1, nil)
			party(t, message, "to", nil, &buyer)
			reply := "Thanks, I look forward to it.\n\n"
			if kind != "unread current text" {
				reply += "On Mon, 1 Jun 2026, Pat wrote:\n> "
			}
			reply += strings.Repeat("Earlier correspondence. ", 200)
			if kind == "inline answer after quoted history" {
				reply += "\nThank you, the pricing arrived and is complete."
			}
			captureCommitmentMessage(t, e, buyer, "reply-tail", "Re: Pricing", reply, "inbound", extractClock.Add(-24*time.Hour))
			extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, message, "seat:"+e.Rep1.String(), pricingPromise, "2026-06-05")})
			wantTasks := 0
			if kind == "quoted history" {
				wantTasks = 1
			}
			if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task'`); got != wantTasks {
				t.Fatalf("tasks = %d, want %d", got, wantTasks)
			}
			if wantTasks == 0 && e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE outcome='later_context_not_read' AND NOT current_work`) != 1 {
				t.Fatal("unread reply has no durable review reason")
			}
		})
	}
}

func TestCapturedOldPromiseNeedsACurrentDeadline(t *testing.T) {
	for _, current := range []bool{false, true} {
		t.Run(map[bool]string{false: "history", true: "future deadline"}[current], func(t *testing.T) {
			e := Setup(t)
			company := e.SeedCompany(t, "Acme", &e.Rep1)
			buyer := employeeOf(t, e, company, "Pat Buyer")
			quote, due := "We will send the revised pricing.", ""
			if current {
				quote, due = "We will send the revised pricing by July 1, 2026.", "2026-07-01"
			}
			message := captureCommitmentMessage(t, e, buyer, "old-pricing", "Pricing", quote, "outbound", extractClock.AddDate(-1, 0, 0))
			party(t, message, "from", &e.Rep1, nil)
			party(t, message, "to", nil, &buyer)
			for range 2 {
				extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, message, "seat:"+e.Rep1.String(), quote, due)})
				e.WsExec(t, `DELETE FROM signal_thread_scan`)
			}
			expected := 0
			if current {
				expected = 1
			}
			if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); got != expected {
				t.Fatalf("tasks = %d, want %d", got, expected)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM signal WHERE kind = 'commitment_made' AND status = 'open'`); got != expected {
				t.Fatalf("current signals = %d, want %d", got, expected)
			}
			page, err := contactRoomService(e).Assemble(e.Admin(), ids.From[ids.ContactKind](buyer))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			if page.Activities != nil {
				for _, row := range page.Activities.Data {
					if ids.UUID(row.Id) == message && row.CommitmentAdmissions != nil {
						found = true
					}
				}
			}
			if !found {
				t.Fatal("contact timeline omitted the commitment review")
			}
			if !current && e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE outcome = 'historical_without_current_deadline' AND NOT current_work`) != 1 {
				t.Fatal("historical promise lost its review reason")
			}
		})
	}
}

func TestPartialDeliveryKeepsTheOriginalCommitmentTask(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	buyer := employeeOf(t, e, company, "Pat Buyer")
	quote := "I will send the standard and enterprise pricing by June 5, 2026."
	first := captureCommitmentMessage(t, e, buyer, "partial-pricing", "Pricing", quote, "outbound", extractClock.Add(-48*time.Hour))
	party(t, first, "from", &e.Rep1, nil)
	party(t, first, "to", nil, &buyer)
	extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, first, "seat:"+e.Rep1.String(), quote, "2026-06-05")})
	original := e.WsScalar(t, `SELECT id::text FROM activity WHERE kind = 'task'`)
	captureCommitmentMessage(t, e, buyer, "partial-pricing", "Re: Pricing", "Here is the standard tier. I still owe you the enterprise pricing and will send it by June 5, 2026.", "outbound", extractClock.Add(-time.Hour))
	extractPass(t, e, &scriptedBrain{reply: strings.ReplaceAll(commitmentReply(t, first, "seat:"+e.Rep1.String(), quote, "2026-06-05"), "Send the revised pricing", "Send the remaining enterprise pricing")})
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); got != 1 {
		t.Fatalf("partial delivery left %d tasks", got)
	}
	if got := e.WsScalar(t, `SELECT id::text FROM activity WHERE kind = 'task'`); got != original {
		t.Fatal("partial delivery replaced the original task")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task' AND done_at IS NULL AND archived_at IS NULL`); got != 1 {
		t.Fatal("partial delivery silently settled the remainder")
	}
}

func TestDismissedHistoricalPromiseDoesNotBecomeAFreshAccountSignal(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	buyer := employeeOf(t, e, company, "Pat Buyer")
	quote := "I will send the pricing."
	source := captureCommitmentMessage(t, e, buyer, "dismissed-history", "Pricing", quote, "inbound", extractClock.AddDate(-1, 0, 0))
	party(t, source, "from", nil, &buyer)
	brain := &scriptedBrain{reply: commitmentReply(t, source, "contact:"+buyer.String(), quote, "")}
	extractPass(t, e, brain)
	claim := ids.MustParse(e.WsScalar(t, `SELECT id::text FROM conversation_claim WHERE source_activity_id=$1`, source))
	if err := e.Contacts.SettleConversationClaim(e.Admin(), claim, "dismissed"); err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `DELETE FROM signal_thread_scan`)
	extractPass(t, e, brain)
	if got := e.WsCount(t, `SELECT count(*) FROM signal WHERE kind='commitment_made' AND status='open'`); got != 0 {
		t.Fatalf("dismissed history opened %d signals", got)
	}
	if got := e.WsScalar(t, `SELECT status FROM conversation_claim WHERE id=$1`, claim); got != "dismissed" {
		t.Fatalf("human dismissal changed to %q", got)
	}
}
