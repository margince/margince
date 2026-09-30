// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A rep's saved draft through the real routes: the wire answers a composer
// relies on, a send that takes the draft with it, and an erasure that reaches
// the draft a rep started to the subject and never sent.

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type wireDraft struct {
	ID      string   `json:"id"`
	Version int64    `json:"version"`
	Body    string   `json:"body"`
	To      []string `json:"to"`
}

// saveReplyDraft saves a draft on the preflight anchor through PUT, the way
// the composer does. headers carries If-Match when replacing.
func (p *preflightEnv) saveReplyDraft(t *testing.T, body string, headers map[string]string) (int, wireDraft) {
	t.Helper()
	var out wireDraft
	status := p.Call(t, "PUT", "/v1/mail-drafts", AnyMap{
		"anchor_type": "activity", "anchor_id": p.activityID,
		"to": []string{"buyer@preflight.test"}, "subject": "Re: pricing", "body": body,
	}, headers, &out)
	return status, out
}

func (p *preflightEnv) replyDraftStatus(t *testing.T) int {
	t.Helper()
	return p.Call(t, "GET", "/v1/mail-drafts?anchor_type=activity&anchor_id="+p.activityID, nil, nil, nil)
}

func (p *preflightEnv) draftRows(t *testing.T) int {
	t.Helper()
	var n int
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM mail_draft`).Scan(&n)
	}); err != nil {
		t.Fatalf("counting drafts: %v", err)
	}
	return n
}

// The answers the composer is built on: 404 when there is nothing to restore,
// 409 when a save would overwrite a draft it never read, 204 on discard.
func TestTheDraftRoutesAnswerTheComposersQuestions(t *testing.T) {
	p := setupPreflight(t)

	if status := p.replyDraftStatus(t); status != http.StatusNotFound {
		t.Fatalf("reading a draft nobody saved → %d, want 404", status)
	}
	status, saved := p.saveReplyDraft(t, "Half a thought", nil)
	if status != http.StatusOK || saved.Version != 1 {
		t.Fatalf("saving a first draft → %d v%d, want 200 v1", status, saved.Version)
	}
	if status, _ := p.saveReplyDraft(t, "A second tab", nil); status != http.StatusConflict {
		t.Errorf("creating over an existing draft → %d, want 409", status)
	}
	ifMatch := map[string]string{"If-Match": strconv.FormatInt(saved.Version, 10)}
	status, replaced := p.saveReplyDraft(t, "The whole thought", ifMatch)
	if status != http.StatusOK || replaced.Version != 2 || replaced.Body != "The whole thought" {
		t.Fatalf("replacing at the read version → %d v%d %q, want 200 v2", status, replaced.Version, replaced.Body)
	}
	if status, _ := p.saveReplyDraft(t, "Stale", ifMatch); status != http.StatusConflict {
		t.Errorf("replacing at a stale version → %d, want 409", status)
	}
	var got wireDraft
	if status := p.Call(t, "GET", "/v1/mail-drafts?anchor_type=activity&anchor_id="+p.activityID, nil, nil, &got); status != http.StatusOK || got.Body != "The whole thought" {
		t.Fatalf("reading the draft back → %d %q, want 200 with the accepted edit", status, got.Body)
	}
	if status := p.Call(t, "DELETE", "/v1/mail-drafts/"+saved.ID, nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("discarding → %d, want 204", status)
	}
	if status := p.Call(t, "DELETE", "/v1/mail-drafts/"+saved.ID, nil, nil, nil); status != http.StatusNotFound {
		t.Errorf("discarding again → %d, want 404", status)
	}
}

// The draft goes with the send that is accepted, and stays through the one
// that is refused: a refusal leaves the rep in the composer with what they
// wrote, even when the refused message is frozen for review.
func TestADraftStaysThroughARefusalAndGoesWithTheAcceptedSend(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)
	_, draft := p.saveReplyDraft(t, "As discussed.", nil)
	send := AnyMap{
		"subject": "Re: pricing", "body": "As discussed.",
		"to": []string{"buyer@preflight.test"}, "consent_purpose": "transactional",
		"mail_draft_id": draft.ID,
	}

	// Nothing on file supports a transactional send yet, so the engine refuses.
	if status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", send, nil, nil); status != http.StatusConflict {
		t.Fatalf("an unsupported send → %d, want 409", status)
	}
	if n := p.draftRows(t); n != 1 {
		t.Fatalf("%d draft(s) after a refused send, want the rep's draft kept", n)
	}

	p.stakeADeal(t)
	if status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", send, nil, nil); status != http.StatusAccepted {
		t.Fatalf("sending under an open deal → %d, want 202", status)
	}
	if n := p.draftRows(t); n != 0 {
		t.Fatalf("%d draft(s) survived the send they were composed for, want 0", n)
	}
}

// Art. 17 reaches the message a rep started to the subject and never sent: it
// holds their address and the words meant for them before any activity does.
func TestErasingARecipientDeletesTheDraftsAddressedToThem(t *testing.T) {
	p := setupPreflight(t)
	p.saveReplyDraft(t, "Written and never sent.", nil)

	contactID, err := ids.Parse(p.contactID)
	if err != nil {
		t.Fatalf("contact id %q: %v", p.contactID, err)
	}
	if err := privacy.NewEraser(compose.InstallationDB(p.Pool)).EraseContact(
		p.privacyAdmin(t), contactID, "art-17"); err != nil {
		t.Fatalf("erasing the recipient: %v", err)
	}
	if n := p.draftRows(t); n != 0 {
		t.Fatalf("%d draft(s) addressed to an erased contact survived the erasure, want 0", n)
	}
}

// The scheduled twin of the send above: the draft goes when the schedule
// commits, not when the message later fires.
func TestAScheduledReplyTakesItsDraftWithIt(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)
	_, draft := p.saveReplyDraft(t, "Written the night before.", nil)

	status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", AnyMap{
		"subject": "Monday morning", "body": "Written the night before.",
		"to": []string{"buyer@preflight.test"}, "consent_purpose": "transactional",
		"scheduled_at": time.Now().Add(12 * time.Hour).UTC().Format(time.RFC3339),
		"scheduled_tz": "Europe/Berlin", "mail_draft_id": draft.ID,
	}, nil, nil)
	if status != http.StatusCreated {
		t.Fatalf("scheduling → %d, want 201", status)
	}
	if n := p.draftRows(t); n != 0 {
		t.Fatalf("%d draft(s) survived the scheduling they were composed for, want 0", n)
	}
}

// Art. 15 owes the subject the message a rep started to them and never sent,
// with the blind-copy rule the scheduled messages follow: the subject sees
// their own blind copy and not who else was blind-copied. A draft opened on
// their record is theirs too; one to somebody else on another record is not.
func TestASubjectAccessExportCarriesTheDraftsWrittenToThem(t *testing.T) {
	p := setupPreflight(t)
	put := func(to, bcc []string, body string) {
		t.Helper()
		if status := p.Call(t, "PUT", "/v1/mail-drafts", AnyMap{
			"anchor_type": "activity", "anchor_id": p.activityID,
			"to": to, "bcc": bcc, "subject": "Re: pricing", "body": body,
		}, nil, nil); status != http.StatusOK {
			t.Fatalf("saving a draft → %d, want 200", status)
		}
	}
	put([]string{"colleague@preflight.test"}, []string{"Buyer@preflight.test", "other-blind@preflight.test"}, "Written and never sent.")
	if status := p.Call(t, "PUT", "/v1/mail-drafts", AnyMap{
		"anchor_type": "contact", "anchor_id": p.seedStranger(t),
		"to": []string{"stranger@preflight.test"}, "body": "Not about the subject.",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("saving the unrelated draft → %d, want 200", status)
	}

	// Opened on the subject's own record, to somebody else: still about them.
	if status := p.Call(t, "PUT", "/v1/mail-drafts", AnyMap{
		"anchor_type": "contact", "anchor_id": p.contactID,
		"to": []string{"assistant@preflight.test"}, "body": "About the subject.",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("saving the draft on the subject's record → %d, want 200", status)
	}

	contactID, err := ids.Parse(p.contactID)
	if err != nil {
		t.Fatalf("contact id %q: %v", p.contactID, err)
	}
	pkg, err := privacy.AssembleSAR(
		p.privacyAdmin(t), compose.InstallationDB(p.Pool), ids.From[ids.ContactKind](contactID))
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}

	if len(pkg.DraftMessages) != 2 {
		t.Fatalf("the export carried %d drafts, want the one written to this contact and the one on their record: %#v",
			len(pkg.DraftMessages), pkg.DraftMessages)
	}
	rendered := fmt.Sprintf("%#v", pkg.DraftMessages)
	if !strings.Contains(rendered, "About the subject.") {
		t.Errorf("the draft opened on the subject's record is missing from their export: %s", rendered)
	}
	if !strings.Contains(rendered, "Written and never sent.") {
		t.Errorf("the export withheld the body of the draft written to this contact: %s", rendered)
	}
	// Saved canonical, the way the send path compares addresses.
	if !strings.Contains(rendered, "buyer@preflight.test") {
		t.Errorf("the subject's own blind copy is missing from their export: %s", rendered)
	}
	if strings.Contains(rendered, "other-blind@preflight.test") {
		t.Errorf("the export disclosed another blind recipient's address: %s", rendered)
	}
}

// seedStranger writes a contact the subject has nothing to do with, as the
// table owner, so a draft can be opened on a record that is not theirs.
func (p *preflightEnv) seedStranger(t *testing.T) string {
	t.Helper()
	id := ids.NewV7()
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO contact (id, full_name, source, captured_by)
			VALUES ($1, 'Stranger', 'manual', 'human:x')`, id)
		return err
	}); err != nil {
		t.Fatalf("seeding the stranger: %v", err)
	}
	return id.String()
}

// Every shape of "a draft that is the subject's" is exported AND erased, and
// by the same rule: an address padded and cased as a rep might type it, a
// draft opened on the contact itself, and one opened on the lead they were
// before promotion. A draft to somebody else on another record is neither.
func TestTheExportAndTheErasureAgreeOnWhichDraftsAreTheSubjects(t *testing.T) {
	p := setupPreflight(t)
	lead := p.seedLiveLead(t)
	stranger := p.seedStranger(t)
	for _, draft := range []AnyMap{
		{"anchor_type": "activity", "anchor_id": p.activityID, "to": []string{"  BUYER@Preflight.test "}, "body": "padded"},
		{"anchor_type": "contact", "anchor_id": p.contactID, "to": []string{"assistant@preflight.test"}, "body": "on their record"},
		{"anchor_type": "lead", "anchor_id": lead, "to": []string{"assistant@preflight.test"}, "body": "on their lead"},
		{"anchor_type": "contact", "anchor_id": stranger, "to": []string{"stranger@preflight.test"}, "body": "not theirs"},
	} {
		if status := p.Call(t, "PUT", "/v1/mail-drafts", draft, nil, nil); status != http.StatusOK {
			t.Fatalf("saving the %q draft → %d, want 200", draft["body"], status)
		}
	}
	p.promoteLead(t, lead)

	contactID, err := ids.Parse(p.contactID)
	if err != nil {
		t.Fatalf("contact id %q: %v", p.contactID, err)
	}
	subject := ids.From[ids.ContactKind](contactID)
	pkg, err := privacy.AssembleSAR(p.privacyAdmin(t), compose.InstallationDB(p.Pool), subject)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}
	exported := fmt.Sprintf("%#v", pkg.DraftMessages)
	for _, body := range []string{"padded", "on their record", "on their lead"} {
		if !strings.Contains(exported, body) {
			t.Errorf("the export is missing the %q draft: %s", body, exported)
		}
	}
	if strings.Contains(exported, "not theirs") || len(pkg.DraftMessages) != 3 {
		t.Errorf("the export carried %d drafts, want exactly the subject's three: %s", len(pkg.DraftMessages), exported)
	}

	if err := privacy.NewEraser(compose.InstallationDB(p.Pool)).EraseContact(
		p.privacyAdmin(t), contactID, "art-17"); err != nil {
		t.Fatalf("erasing the recipient: %v", err)
	}
	if n := p.draftRows(t); n != 1 {
		t.Fatalf("%d draft(s) left after the erasure, want only the one that was never the subject's", n)
	}
}

// seedLiveLead writes a lead the rep can open a composer on, before anything
// has promoted it.
func (p *preflightEnv) seedLiveLead(t *testing.T) string {
	t.Helper()
	id := ids.NewV7()
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO lead (id, owner_id, full_name, source, captured_by)
			VALUES ($1, $2, 'Buyer as a lead', 'manual', 'human:x')`, id, p.user)
		return err
	}); err != nil {
		t.Fatalf("seeding the lead: %v", err)
	}
	return id.String()
}

// promoteLead makes the lead the subject's twin, the state a promotion leaves.
func (p *preflightEnv) promoteLead(t *testing.T, lead string) {
	t.Helper()
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE lead SET status = 'promoted', promoted_contact_id = $2, archived_at = now()
			 WHERE id = $1`, lead, p.contactID)
		return err
	}); err != nil {
		t.Fatalf("promoting the lead: %v", err)
	}
}
