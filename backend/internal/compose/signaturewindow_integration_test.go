// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The signature pass reads only what the contact typed in that mail. It never
// writes a number a workspace member signs with. These tests use the real pass,
// writer and data task.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	bobOwnNumber    = "+44 20 7946 0000"
	quotedNumber    = "+49 30 1111111"
	memberNumber    = "+31 20 555 0101"
	bobOlderNumber  = "+49 30 9999999"
	quotedTitle     = "Head of Sales"
	quotedWebsite   = "seller.example"
	outlookEnvelope = "From: Bob Contact <bob@window.example>\nTo: Sam Seller <sam@seller.example>\n\n"
)

// replyQuotingSam is Bob's top-posted reply, his own signature above Outlook's
// unmarked quote of Sam's mail and Sam's signature.
func replyQuotingSam(ownLines string) string {
	return outlookEnvelope + "Works for me.\n\nBest\nBob Contact\n" + ownLines + "\n\n" +
		"From: Sam Seller <sam@seller.example>\nSent: Monday\nTo: Bob Contact <bob@window.example>\nSubject: Next steps\n\n" +
		"Hi Bob, see below.\n\nSam Seller\n" + quotedTitle + "\n" + quotedNumber + "\n" + quotedWebsite
}

func TestAReplyQuotingOurSignatureAddsOnlyTheSendersOwnNumber(t *testing.T) {
	e := integration.Setup(t)
	contact := seedEnrichContact(t, e, "bob@window.example", replyQuotingSam(bobOwnNumber))
	runEnrichPass(t, e, &signatureScriptBrain{fields: phoneReply(bobOwnNumber, quotedNumber)})

	if got := liveNumbers(t, e, contact); !slices.Equal(got, []string{"+442079460000"}) {
		t.Fatalf("live numbers = %v, want Bob's own number and none from the quoted signature", got)
	}
}

func TestANumberAMemberSignsWithIsNeverWrittenToAContact(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `INSERT INTO email_signature (owner_id, body, phone) VALUES ($1, $2, '')`,
		e.AdminUser, "Sam Seller\nHead of Sales\nMobile: "+memberNumber)
	contact := seedEnrichContact(t, e, "bob@window.example",
		"Works for me.\n\nBest\nBob Contact\n"+bobOwnNumber+"\nYour account manager: "+memberNumber)
	runEnrichPass(t, e, &signatureScriptBrain{fields: phoneReply(bobOwnNumber, memberNumber)})

	if got := liveNumbers(t, e, contact); !slices.Equal(got, []string{"+442079460000"}) {
		t.Fatalf("live numbers = %v, want Bob's own number and not the member's", got)
	}
	if _, member := numberEvidence(t, e, contact)["+31205550101"]; member {
		t.Fatal("the member's number left an evidence row on the contact")
	}
}

func TestAMemberKeepsTheirOwnNumberOnTheirOwnContact(t *testing.T) {
	e := integration.Setup(t)
	login := e.WsScalar(t, `SELECT email FROM app_user WHERE id = $1`, e.AdminUser)
	e.WsExec(t, `INSERT INTO email_signature (owner_id, body, phone) VALUES ($1, '', $2)`, e.AdminUser, memberNumber)
	contact := seedEnrichContact(t, e, login, "Works for me.\n\nBob Contact\n"+memberNumber)
	runEnrichPass(t, e, &signatureScriptBrain{fields: phoneReply(memberNumber)})

	if got := liveNumbers(t, e, contact); !slices.Equal(got, []string{"+31205550101"}) {
		t.Fatalf("live numbers = %v, want the member's number on the contact that is the member", got)
	}
}

func TestTheMisattributionSweepTakesTheNumberOffTheContact(t *testing.T) {
	e := integration.Setup(t)
	contact := seedForeignSignature(t, e)
	activity := onlyMailOf(t, e, contact)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, 'from')`, activity, contact)
	applySignature(t, e, contact, activity, phoneField(quotedNumber))
	e.WsExec(t, `DELETE FROM activity_participant
		WHERE activity_id = $1 AND contact_id = $2 AND role = 'from' AND address IS NULL`, activity, contact)

	if _, err := e.Contacts.RetractMisattributedSignatureFields(e.Admin()); err != nil {
		t.Fatal(err)
	}
	if got := liveNumbers(t, e, contact); len(got) != 0 {
		t.Fatalf("live numbers = %v after the sweep took their evidence back, want none", got)
	}
}

// signatureWindowFixture is what the pass wrote before it read only the
// sender's text. Bob holds his numbers and Sam's quoted details. Cy holds Sam's
// title, which a human has since changed, and Sam's number, which a human typed.
type signatureWindowFixture struct{ bob, cy ids.UUID }

func seedSignatureWindowFixture(t *testing.T, e *integration.Env) signatureWindowFixture {
	t.Helper()
	bob := seedEnrichContact(t, e, "bob@window.example", replyQuotingSam(bobOwnNumber+"\nYour account manager: "+memberNumber))
	reply := onlyMailOf(t, e, bob)
	seedLaterMail(t, e, bob, "bob@window.example", "Thanks\n\nBob Contact\n"+bobOlderNumber, "-2 days")
	older := otherMailOf(t, e, bob, reply)
	applySignature(t, e, bob, older, phoneField(bobOlderNumber))
	applySignature(t, e, bob, reply,
		phoneField(quotedNumber), phoneField(bobOwnNumber), phoneField(memberNumber),
		signatureField("title", quotedTitle), signatureField("website", quotedWebsite))
	if err := ai.NewFeedbackStore(InstallationDB(e.Pool)).Record(e.Admin(), ai.RecordInput{
		SubjectType: "contact", SubjectID: bob, ClaimKind: ai.ClaimProfileField,
		ClaimPath: ai.ProfileFieldClaimPath("website"), Verdict: ai.VerdictCorrected,
		CorrectedValue: new("bob.example"),
	}); err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `INSERT INTO email_signature (owner_id, body, phone) VALUES ($1, '', $2)`, e.AdminUser, memberNumber)

	cy := seedEnrichContact(t, e, "cy@window.example", "Hello\n\nCy")
	first := onlyMailOf(t, e, cy)
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](cy), contacts.UpdateContactInput{
		Phones: []contacts.ContactPhoneInput{{Phone: "+49301111111", PhoneType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatal(err)
	}
	seedLaterMail(t, e, cy, "cy@window.example", outlookEnvelope+"Fine.\n\nCy\n\n"+
		"From: Sam Seller <sam@seller.example>\nSent: Monday\n\nSam Seller\n"+quotedTitle+"\n"+quotedNumber, "+1 hour")
	applySignature(t, e, cy, otherMailOf(t, e, cy, first), signatureField("title", quotedTitle), phoneField(quotedNumber))
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](cy),
		contacts.UpdateContactInput{Title: new("CFO")}); err != nil {
		t.Fatal(err)
	}
	return signatureWindowFixture{bob: bob, cy: cy}
}

func TestTheSignatureWindowTaskTakesBackWhatTheSenderDidNotWrite(t *testing.T) {
	e := integration.Setup(t)
	f := seedSignatureWindowFixture(t, e)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	step := signatureWindowDataTask().Steps[0]

	rollback := errors.New("rolled back")
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if _, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 50, RunID: ids.NewV7()}); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("the rolled-back batch: %v", err)
	}
	if got := signatureEvidence(t, e, f.bob); len(got) != 5 {
		t.Fatalf("evidence after a rollback = %v, want all five rows the pass left", got)
	}

	task := signatureWindowDataTask()
	task.Steps[0].Limit, task.Steps[0].MinLimit = 1, 1
	runMachineAddressTask(ctx, t, e, task)

	if got := liveNumbers(t, e, f.bob); !slices.Equal(got, []string{"+442079460000"}) {
		t.Errorf("Bob's numbers = %v, want only the number with surviving evidence", got)
	}
	if got := signatureEvidence(t, e, f.bob); !slices.Equal(got, []string{"phone:+442079460000", "website:"}) {
		t.Errorf("Bob's evidence = %v, want the supported number and the corrected website", got)
	}
	if title := contactTitle(t, e, f.bob); title != nil {
		t.Errorf("Bob's title = %q, want the quoted title taken back", *title)
	}
	if title := contactTitle(t, e, f.cy); title == nil || *title != "CFO" {
		t.Errorf("Cy's title = %v, want the one a human typed", title)
	}
	if got := signatureEvidence(t, e, f.cy); len(got) != 0 {
		t.Errorf("Cy's evidence = %v, want the quoted rows gone", got)
	}
	if got := liveNumbers(t, e, f.cy); !slices.Equal(got, []string{"+49301111111"}) {
		t.Errorf("Cy's numbers = %v, want the number a human typed kept", got)
	}
	audits := retractionAudits(t, e)
	if audits != 5 {
		t.Errorf("%d retraction audit rows, want one per field taken back, at one field per batch", audits)
	}

	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 50, RunID: ids.NewV7()})
		if err == nil && out.Changed != 0 {
			t.Errorf("a replay changed %d rows, want none", out.Changed)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if again := retractionAudits(t, e); again != audits {
		t.Errorf("a replay wrote %d more audit rows", again-audits)
	}
}

func TestAClaimCannotQuoteTheMailItAnswers(t *testing.T) {
	e := integration.Setup(t)
	contact := seedEnrichContact(t, e, "bob@window.example", replyQuotingSam(bobOwnNumber))
	claim := contacts.ClaimInput{
		ContactID: ids.From[ids.ContactKind](contact), Kind: claimKindOurs, Body: "Sam sends next steps",
		ActivityID: onlyMailOf(t, e, contact), Quote: "Hi Bob, see below.", Source: "test",
	}
	if _, err := e.Contacts.RecordConversationClaim(e.Admin(), claim); err == nil {
		t.Fatal("a claim quoting Sam's mail under Bob's reply was filed against Bob's mail")
	}
	claim.Quote = "Works for me."
	if _, err := e.Contacts.RecordConversationClaim(e.Admin(), claim); err != nil {
		t.Fatalf("a claim quoting what Bob wrote was refused: %v", err)
	}
}

func phoneField(number string) contacts.SignatureField {
	return signatureField("phone", number)
}

func signatureField(name, value string) contacts.SignatureField {
	return contacts.SignatureField{
		Name: name, Value: value, Evidence: value, Confidence: 0.9,
		ClaimKey: ai.ClaimKey(ai.ProfileFieldClaimPath(name)),
	}
}

// applySignature writes fields as the pass does, through the real applier.
func applySignature(t *testing.T, e *integration.Env, contact, activity ids.UUID, fields ...contacts.SignatureField) {
	t.Helper()
	if _, err := e.Contacts.ApplySignatureFields(e.Admin(), ids.From[ids.ContactKind](contact), activity, fields, true); err != nil {
		t.Fatalf("applying the signature through the real writer: %v", err)
	}
}

func onlyMailOf(t *testing.T, e *integration.Env, contact ids.UUID) ids.UUID {
	t.Helper()
	return ids.MustParse(e.WsScalar(t, `SELECT activity_id::text FROM activity_link WHERE contact_id = $1`, contact))
}

func otherMailOf(t *testing.T, e *integration.Env, contact, besides ids.UUID) ids.UUID {
	t.Helper()
	return ids.MustParse(e.WsScalar(t,
		`SELECT activity_id::text FROM activity_link WHERE contact_id = $1 AND activity_id <> $2`, contact, besides))
}

// signatureEvidence is the contact's signature evidence as field:value_key.
func signatureEvidence(t *testing.T, e *integration.Env, contact ids.UUID) []string {
	t.Helper()
	var fields []string
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT field || ':' || value_key FROM contact_profile_field
			 WHERE contact_id = $1 ORDER BY 1`, contact)
		if err != nil {
			return err
		}
		fields, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return fields
}

func contactTitle(t *testing.T, e *integration.Env, contact ids.UUID) *string {
	t.Helper()
	var title *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT title FROM contact WHERE id = $1`, contact).Scan(&title)
	}); err != nil {
		t.Fatal(err)
	}
	return title
}

func retractionAudits(t *testing.T, e *integration.Env) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM audit_log
		 WHERE entity_type = 'contact' AND evidence->>'reason' IN
		   ('signature read off text the contact did not write in that mail', 'a number a workspace member signs with')`)
}
