// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Which earlier mail a lead's address files under it. History from before the
// lead was created is filed. Mail private to the seat is not, and neither is
// mail to an address the lead took later. A meeting keeps its project's seat.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// moveAddress creates a lead at from and changes its email to to, answering
// the lead and when the change was recorded.
func (l *leadMailEnv) moveAddress(t *testing.T, from, to string) (ids.LeadID, time.Time) {
	t.Helper()
	lead := l.lead(t, from)
	if _, err := l.e.Contacts.UpdateLead(l.e.Admin(), lead, contacts.UpdateLeadInput{Email: new(to)}); err != nil {
		t.Fatalf("changing the lead's address to %s: %v", to, err)
	}
	var changed time.Time
	if err := database.WithWorkspaceTx(l.e.Admin(), l.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT max(occurred_at) FROM audit_log
			WHERE entity_type = 'lead' AND entity_id = $1 AND before ? 'email'`, lead).Scan(&changed)
	}); err != nil {
		t.Fatalf("reading when the address changed: %v", err)
	}
	return lead, changed
}

func TestAnAddressALeadTookLaterFilesNoEarlierMail(t *testing.T) {
	l := setupLeadMail(t)
	earlier := l.at.AddDate(0, 0, -10)

	alice, changed := l.moveAddress(t, "alice@prospect.example", "info@prospect.example")
	old := l.mail(t, owedMail{from: "info@prospect.example", to: l.seat, subject: "Enquiry", at: earlier}, true)
	if l.filedUnderLead(t, old, alice) {
		t.Error("mail to info@ from before Alice took the address was filed under her")
	}
	since := l.mail(t, owedMail{from: "info@prospect.example", to: l.seat, subject: "Follow-up", at: changed.Add(time.Minute)}, true)
	if !l.filedUnderLead(t, since, alice) {
		t.Error("mail to Alice's address after she took it was not filed under her")
	}

	bob, _ := l.moveAddress(t, "sales@prospect.example", "bob@prospect.example")
	carol := l.lead(t, "sales@prospect.example")
	whileBobs := l.mail(t, owedMail{from: "sales@prospect.example", to: l.seat, subject: "Quote", at: earlier}, true)
	if l.filedUnderLead(t, whileBobs, carol) || l.filedUnderLead(t, whileBobs, bob) {
		t.Error("mail to a role address from while Bob held it was filed under Carol, who holds it now, or under Bob by his old address")
	}
	carols := l.mail(t, owedMail{from: "sales@prospect.example", to: l.seat, subject: "Order", at: l.createdAt(t, carol).Add(time.Minute)}, true)
	if !l.filedUnderLead(t, carols, carol) {
		t.Error("mail to the role address after Carol took it was not filed under her")
	}
}

// createdAt is when the database recorded the lead.
func (l *leadMailEnv) createdAt(t *testing.T, lead ids.LeadID) time.Time {
	t.Helper()
	var created time.Time
	if err := database.WithWorkspaceTx(l.e.Admin(), l.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT created_at FROM lead WHERE id = $1`, lead).Scan(&created)
	}); err != nil {
		t.Fatalf("reading when the lead was created: %v", err)
	}
	return created
}

// An address a retired lead still holds was that lead's. A new lead taking it
// gets the mail sent since it was created, and none from before.
func TestAnAddressARetiredLeadHeldFilesOnlyMailSinceTheNewLead(t *testing.T) {
	l := setupLeadMail(t)
	earlier := l.at.AddDate(0, 0, -10)

	dropped := l.lead(t, "info@prospect.example")
	if _, err := l.e.Contacts.DisqualifyLead(l.e.Admin(), dropped, contacts.DisqualifyLeadInput{Note: new("Wrong fit")}); err != nil {
		t.Fatalf("disqualifying the first holder: %v", err)
	}
	successor := l.lead(t, "info@prospect.example")
	assertFilesOnlySince(t, l, "info@prospect.example", successor, earlier)

	mergedAway := l.lead(t, "desk@prospect.example")
	survivor, _, err := l.e.Contacts.CreateLead(l.e.Admin(), contacts.CreateLeadInput{FullName: new("Desk Survivor"), Source: "manual"})
	if err != nil {
		t.Fatalf("creating the survivor: %v", err)
	}
	survivorID := ids.From[ids.LeadKind](ids.UUID(survivor.Id))
	if _, err := l.e.Contacts.MergeLead(l.e.Admin(), mergedAway, survivorID); err != nil {
		t.Fatalf("merging the first holder away: %v", err)
	}
	assertFilesOnlySince(t, l, "desk@prospect.example", survivorID, earlier)
}

func assertFilesOnlySince(t *testing.T, l *leadMailEnv, address string, lead ids.LeadID, earlier time.Time) {
	t.Helper()
	old := l.mail(t, owedMail{from: address, to: l.seat, subject: "Old enquiry", at: earlier}, true)
	if l.filedUnderLead(t, old, lead) {
		t.Errorf("mail to %s from when another lead held it was filed under the lead holding it now", address)
	}
	since := l.mail(t, owedMail{from: address, to: l.seat, subject: "New enquiry", at: l.createdAt(t, lead).Add(time.Minute)}, true)
	if !l.filedUnderLead(t, since, lead) {
		t.Errorf("mail to %s since the lead was created was not filed under it", address)
	}
}

// importedLead creates a lead the way the import job does: under an import
// run, which its create audit row carries as its batch.
func (l *leadMailEnv) importedLead(t *testing.T, address string) ids.LeadID {
	t.Helper()
	var run ids.UUID
	if err := database.WithWorkspaceTx(l.e.Admin(), l.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			INSERT INTO import_run (connector, status, mapping, source_ref, source, captured_by)
			VALUES ('csv', 'running', '{}'::jsonb, 'leads.csv', 'upload', 'human:test') RETURNING id`).Scan(&run)
	}); err != nil {
		t.Fatalf("opening the import run: %v", err)
	}
	ctx := storekit.WithImportRun(storekit.WithBatch(l.e.Admin(), run), run)
	created, _, err := l.e.Contacts.CreateLead(ctx, contacts.CreateLeadInput{
		FullName: new("Imported Prospect"), Email: new(address), Source: "import",
	})
	if err != nil {
		t.Fatalf("importing the lead for %s: %v", address, err)
	}
	return ids.From[ids.LeadKind](ids.UUID(created.Id))
}

// A lead created by hand, by capture or by the API gets no mail from before it
// existed. History from before a lead is filed only for one an import created.
func TestOnlyAnImportedLeadGetsMailFromBeforeIt(t *testing.T) {
	l := setupLeadMail(t)
	byHand := l.lead(t, "hand@prospect.example")
	old := l.mail(t, owedMail{from: "hand@prospect.example", to: l.seat, subject: "Prices?", at: l.at.AddDate(0, 0, -30)}, true)
	if l.filedUnderLead(t, old, byHand) {
		t.Error("mail from before a lead created by hand was filed under it")
	}
	if since := l.mail(t, owedMail{from: "hand@prospect.example", to: l.seat, subject: "Again"}, true); !l.filedUnderLead(t, since, byHand) {
		t.Error("mail since the hand-made lead was created was not filed under it")
	}
}

// An imported lead keeps its history, and the history does not move its reply
// clock before the lead existed.
func TestALeadsEarlierHistoryIsFiledAndItsClockStartsAtCreation(t *testing.T) {
	l := setupLeadMail(t)
	lead := l.importedLead(t, "lena@prospect.example")
	asked := l.mail(t, owedMail{from: "lena@prospect.example", to: l.seat, subject: "Prices?", at: l.at.AddDate(0, 0, -30)}, true)
	answered := l.mail(t, owedMail{from: l.seat, to: "lena@prospect.example", subject: "Re: Prices?", at: l.at.AddDate(0, 0, -29)}, true)
	for name, activity := range map[string]ids.UUID{"the question": asked, "the answer": answered} {
		if !l.filedUnderLead(t, activity, lead) {
			t.Fatalf("%s from before the lead was created is not filed under it", name)
		}
	}
	runLeadFirstResponse(t, l, answered)
	var created time.Time
	var responded *time.Time
	if err := database.WithWorkspaceTx(l.e.Admin(), l.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT created_at, first_response_at FROM lead WHERE id = $1`, lead).Scan(&created, &responded)
	}); err != nil {
		t.Fatal(err)
	}
	if responded == nil || !responded.Equal(created) {
		t.Errorf("first_response_at = %v, want the lead's creation %v: an answer older than the lead stamps no earlier response", responded, created)
	}
}

func runLeadFirstResponse(t *testing.T, l *leadMailEnv, activity ids.UUID) {
	t.Helper()
	ctx := principal.WithActor(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), l.e.WS), ids.NewV7()),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
	for _, h := range contacts.LeadSLAWorkflows(contacts.NewStore(l.e.DB())) {
		if h.Spec().Name != "lead_first_response" {
			continue
		}
		ev := workflow.Event{
			ID: ids.NewV7(), Type: "activity.captured", WorkspaceID: l.e.WS, OccurredAt: clocktest.Now(t),
			Entity: datasource.EntityRef{Type: datasource.EntityActivity, ID: activity},
		}
		eff, err := h.Plan(ctx, ev)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.Apply(ctx, ev, eff, nil); err != nil {
			t.Fatalf("recording the first response: %v", err)
		}
		return
	}
	t.Fatal("lead_first_response is not among the registered lead workflows")
}

func TestMailPrivateToTheSeatGoesOnNoLead(t *testing.T) {
	l := setupLeadMail(t)
	ctx := principal.SystemActing(l.e.Admin(), "system:datatask-test")
	lead := l.lead(t, "lena@family.example")
	first := l.mail(t, owedMail{from: "lena@family.example", to: l.seat, subject: "Sunday lunch", messageID: "fam1@family.example"}, true)
	if !l.filedUnderLead(t, first, lead) {
		t.Fatal("the message is not filed under the lead before any verdict, so this test would prove nothing")
	}
	threadKey := l.e.WsScalar(t, `SELECT thread_key FROM activity WHERE id = $1`, first)
	runConfidentiality(t, l.e, seedThreadQuestion(t, l.e, threadKey, first), confidentialityPersonal, 0.95)

	if l.filedUnderLead(t, first, lead) {
		t.Error("a personal verdict left the lead link capture filed for the thread")
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'activity_relink' AND entity_id = $1
		AND before->>'entity_type' = 'lead' AND evidence->>'reason' = 'personal_thread_verdict'`, first); n != 1 {
		t.Errorf("taking the lead link off wrote %d audit rows, want one naming the verdict", n)
	}
	reply := l.mail(t, owedMail{from: "lena@family.example", to: l.seat, subject: "Re: Sunday lunch", inReplyTo: "fam1@family.example"}, true)
	if got := l.e.WsScalar(t, `SELECT thread_key FROM activity WHERE id = $1`, reply); got != threadKey {
		t.Fatalf("the reply landed on thread %q, not the judged %q", got, threadKey)
	}
	if l.filedUnderLead(t, reply, lead) {
		t.Error("a message on a thread already judged personal was filed under a lead")
	}
	asLeadReadingSeat(t, l)
	leadFilingPass(ctx, t, l, false)
	if l.filedUnderLead(t, first, lead) || l.filedUnderLead(t, reply, lead) {
		t.Error("the lead filing task filed a personal thread under a lead")
	}
}

func TestAMeetingKeepsASeatForItsProject(t *testing.T) {
	l := setupLeadMail(t)
	company := l.e.SeedCompany(t, "Acme Projects", nil)
	project, err := l.e.Projects.CreateProject(l.e.Admin(), projects.CreateProjectInput{
		Name: "Rollout", CompanyID: ids.From[ids.CompanyKind](company), Source: "manual",
	})
	if err != nil || project.Key == nil {
		t.Fatalf("creating the project: %+v, %v", project, err)
	}
	attendees := []string{}
	for i := range 24 {
		address := fmt.Sprintf("attendee%02d@customer.example", i)
		l.contact(t, fmt.Sprintf("Attendee %02d", i), address)
		attendees = append(attendees, address)
	}
	lead := l.lead(t, "lena@prospect.example")
	meeting := l.meetingWith(t, "["+*project.Key+"] Steering", append(attendees, "lena@prospect.example")...)

	if n := l.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND project_id = $2`, meeting, project.Id); n != 1 {
		t.Errorf("the meeting naming its project key is filed under the project %d times, want once: its attendees took the last seat", n)
	}
	if l.filedUnderLead(t, meeting, lead) {
		t.Error("the lead took the seat the meeting keeps for its project")
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1`, meeting); n != 25 {
		t.Errorf("the meeting is filed under %d records, want the 24 attendees and the project", n)
	}
}

// A personal sender verdict takes off the lead links capture filed for that
// sender's mail to the seat, each with its audit row. A link a colleague filed
// by hand stays.
func TestAPersonalSenderVerdictTakesTheSendersMailOffItsLead(t *testing.T) {
	l := setupLeadMail(t)
	const sister = "sister@family.example"
	lead := l.lead(t, sister)
	machine := l.mail(t, owedMail{from: sister, to: l.seat, subject: "Holiday photos"}, true)
	byHand := l.mail(t, owedMail{from: sister, to: l.seat, subject: "Flight times"}, false)
	if _, err := l.e.Activities.RelinkActivity(l.e.Admin(), ids.From[ids.ActivityKind](byHand),
		activities.RelinkActivityInput{EntityType: "lead", EntityID: lead.UUID}); err != nil {
		t.Fatalf("filing the second mail under the lead by hand: %v", err)
	}
	if !l.filedUnderLead(t, machine, lead) || !l.filedUnderLead(t, byHand, lead) {
		t.Fatal("both mails must be on the lead before the verdict, or this test proves nothing")
	}

	l.settleSenderVerdict(t, sister, machine, capture.KindPersonal)

	if l.filedUnderLead(t, machine, lead) {
		t.Error("a personal sender verdict left the lead link capture filed for that sender's mail")
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'activity_relink' AND entity_id = $1
		AND before->>'entity_type' = 'lead' AND evidence->>'reason' = 'personal_sender_verdict'`, machine); n != 1 {
		t.Errorf("taking the lead link off wrote %d audit rows, want one naming the verdict", n)
	}
	if !l.filedUnderLead(t, byHand, lead) {
		t.Error("a personal sender verdict took off a lead link a colleague filed by hand")
	}
}

// settleSenderVerdict runs the real sender verdict engine on the seat's open
// question about the address, answering kind.
func (l *leadMailEnv) settleSenderVerdict(t *testing.T, address string, mail ids.UUID, kind string) {
	t.Helper()
	var question ids.UUID
	err := database.WithWorkspaceTx(l.e.Admin(), l.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT id FROM capture_pending_counterparty
			WHERE email = $1 AND owner_id = $2`, address, l.owner).Scan(&question)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		question = seedPendingDisposition(t, l.e, address, "family.example", mail)
	} else if err != nil {
		t.Fatalf("reading the open sender question: %v", err)
	}
	l.e.WsExec(t, `UPDATE capture_pending_counterparty SET status = 'pending', next_attempt_at = now() WHERE id = $1`, question)
	brain := &scriptedVerdictBrain{verdicts: map[string]string{question.String(): kind}}
	engine := NewCounterpartyVerdictEngine(l.e.Pool, brain, CaptureConfig{}, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), l.e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}
}
