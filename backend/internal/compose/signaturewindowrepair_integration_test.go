// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Taking a signature field back on a narrowing, for a quoted name and for a
// mail that is gone. Also the narrowing against a writer holding the contact.

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnOutlookBoldFromReplyQuotingOurSignatureAddsNoMemberNumber(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `INSERT INTO email_signature (owner_id, body, phone) VALUES ($1, '', $2)`, e.AdminUser, memberNumber)
	// SenderLines does not know the bold header, so Sam's quoted signature is
	// in the window and only the member check keeps his number off Bob.
	contact := seedEnrichContact(t, e, "bob@window.example", "Works for me.\n\nBob Contact\n"+bobOwnNumber+"\n\n"+
		"*From:* Sam Seller <sam@seller.example>\n*Sent:* Monday\n\nSam Seller\n"+memberNumber)
	runEnrichPass(t, e, &signatureScriptBrain{fields: phoneReply(bobOwnNumber, memberNumber)})

	if got := liveNumbers(t, e, contact); !slices.Equal(got, []string{"+442079460000"}) {
		t.Fatalf("live numbers = %v, want Bob's own number and not the member's", got)
	}
}

func TestACardKeepsAMembersSwitchboard(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `INSERT INTO email_signature (owner_id, body, phone) VALUES ($1, '', $2)`, e.AdminUser, memberNumber)
	kim := seedEnrichContact(t, e, "kim@shared.example", "Hello")
	results, err := e.Contacts.ImportVCardsFromMessage(e.ConnectorOwnerCtx(e.AdminUser, "gmail"), []contacts.VCardEntry{{
		FullName: "Kim Shared",
		Emails:   []contacts.VCardChannel{{Value: "kim@shared.example", Kind: "work"}},
		Phones:   []contacts.VCardChannel{{Value: memberNumber, Kind: "work"}},
	}}, seedCapturedMail(t, e, "kim@shared.example", "my card"))
	if err != nil || len(results) != 1 || results[0].ContactID == nil {
		t.Fatalf("importing the card: results=%+v err=%v", results, err)
	}
	if results[0].ContactID.UUID != kim {
		t.Fatalf("the card made contact %v, want it to update Kim's record", results[0].ContactID)
	}
	if got := liveNumbers(t, e, kim); !slices.Equal(got, []string{"+31205550101"}) {
		t.Fatalf("live numbers = %v, want the switchboard the card states", got)
	}
}

func narrowedEvent(t *testing.T, activity ids.UUID) events.Envelope {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"changed_fields": map[string]any{"audience": "participants"}})
	if err != nil {
		t.Fatal(err)
	}
	return events.Envelope{
		Type: "activity.updated", Entity: events.EntityRef{Type: "activity", ID: activity}, Payload: payload,
		Trace: events.Trace{CorrelationID: ids.NewV7()},
	}
}

// signedAndNarrowed is Bob with a title and a number read off his own mail,
// which is then limited to its participants.
func signedAndNarrowed(t *testing.T, e *integration.Env) (contact, mail ids.UUID) {
	t.Helper()
	contact = seedEnrichContact(t, e, "bob@window.example", "Works.\n\nBob Contact\nCTO\n"+bobOwnNumber)
	mail = onlyMailOf(t, e, contact)
	applySignature(t, e, contact, mail, signatureField("title", "CTO"), phoneField(bobOwnNumber))
	e.WsExec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, mail)
	return contact, mail
}

const narrowedReason = "the mail it was read from was narrowed"

func TestNarrowingTakesTheSignatureTitleAndNumberOffTheContact(t *testing.T) {
	e := integration.Setup(t)
	contact, mail := signedAndNarrowed(t, e)

	if err := NewAudienceRescopeGen(e.Pool).HandleEvent(context.Background(), narrowedEvent(t, mail)); err != nil {
		t.Fatal(err)
	}
	if title := contactTitle(t, e, contact); title != nil {
		t.Errorf("title = %q, want it taken back with the narrowed mail", *title)
	}
	if got := liveNumbers(t, e, contact); len(got) != 0 {
		t.Errorf("live numbers = %v, want none", got)
	}
	audits := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND evidence->>'reason' = $2`, contact, narrowedReason)
	events := e.WsCount(t, `SELECT count(*) FROM event_outbox o JOIN audit_log a ON a.id = (o.envelope->'trace'->>'audit_log_id')::uuid
		 WHERE a.entity_id = $1 AND a.evidence->>'reason' = $2`, contact, narrowedReason)
	if audits != 1 || events != 1 {
		t.Errorf("%d audit rows and %d events for the retraction, want one each", audits, events)
	}
}

func TestARescopeOfAMailWidenedAgainLeavesItsSignatureFields(t *testing.T) {
	e := integration.Setup(t)
	contact, mail := signedAndNarrowed(t, e)
	e.WsExec(t, `UPDATE activity SET audience = 'workspace' WHERE id = $1`, mail)

	if err := NewAudienceRescopeGen(e.Pool).HandleEvent(context.Background(), narrowedEvent(t, mail)); err != nil {
		t.Fatal(err)
	}
	if title := contactTitle(t, e, contact); title == nil || *title != "CTO" {
		t.Errorf("title = %v, want CTO kept: the mail is open again", title)
	}
}

func TestANarrowingWaitsForTheSignatureWriterInsteadOfDeadlocking(t *testing.T) {
	e := integration.Setup(t)
	contact, mail := signedAndNarrowed(t, e)
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	writer, err := e.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := writer.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("releasing the writer: %v", err)
		}
	})
	var writerPID int
	if err := writer.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&writerPID); err != nil {
		t.Fatal(err)
	}
	// The signature writer's first lock: applySignature holds the contact.
	if _, err := writer.Exec(ctx, `SELECT id FROM contact WHERE id = $1 FOR UPDATE`, contact); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- NewAudienceRescopeGen(e.Pool).HandleEvent(context.Background(), narrowedEvent(t, mail))
	}()
	waitForBackendBlockedBy(t, writer, writerPID, done)

	// Then the source mail, which the rescope held while it ran, and the
	// evidence rows the writer rewrites.
	if _, err := writer.Exec(ctx, `SELECT id FROM activity WHERE id = $1 FOR SHARE`, mail); err != nil {
		t.Fatalf("the writer's lock on the source mail: %v", err)
	}
	if _, err := writer.Exec(ctx, `SELECT id FROM contact_profile_field WHERE contact_id = $1 FOR UPDATE`, contact); err != nil {
		t.Fatalf("the writer's lock on the evidence rows: %v", err)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := within(t, done); err != nil {
		t.Fatalf("the narrowing: %v", err)
	}
	if title := contactTitle(t, e, contact); title != nil {
		t.Errorf("title = %q after the writer let go, want it taken back", *title)
	}
}

func TestTheRepairTakesBackANameReadFromAQuote(t *testing.T) {
	e := integration.Setup(t)
	read := seedNamedContact(t, e, "mai@window.example", "Mai", "connector:gmail")
	edited := seedNamedContact(t, e, "mai.edited@window.example", "Mai", "connector:gmail")
	runNamePass(t, e, vietnameseReply)
	if row := readNameRow(t, e, read); row.Full != "Lê Thị Mai" {
		t.Fatalf("display = %q after the pass, want the printed name: the fixture wrote no name to take back", row.Full)
	}
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](edited),
		contacts.UpdateContactInput{FullName: new("Mai L.")}); err != nil {
		t.Fatal(err)
	}
	// Today's pass cannot read a quote, so it read Mai's own mail, and the
	// stored body now holds that signature under a quote header instead.
	e.WsExec(t, `UPDATE activity SET body = $1
		 WHERE id IN (SELECT activity_id FROM activity_link WHERE contact_id IN ($2, $3))`, "Cảm ơn.\n\nMai\n\n"+
		"From: Lan Seller <lan@seller.example>\nSent: Monday\n\nTrân trọng,\nLê Thị Mai\nGiám đốc", read, edited)

	runMachineAddressTask(principal.SystemActing(e.Admin(), "system:datatask-test"), t, e, signatureWindowDataTask())

	if row := readNameRow(t, e, read); row.First != nil || row.Last != nil || row.Full != "Mai" || row.Evidence != 0 {
		t.Errorf("first %s last %s display %q evidence %d, want the name the quote gave taken back to Mai",
			str(row.First), str(row.Last), row.Full, row.Evidence)
	}
	if row := readNameRow(t, e, edited); row.Full != "Mai L." || row.Evidence != 0 {
		t.Errorf("display %q evidence %d, want the display a human typed kept and the evidence gone", row.Full, row.Evidence)
	}
}

func TestTheRepairTakesBackFieldsWhoseMailIsGone(t *testing.T) {
	e := integration.Setup(t)
	gone := map[string][]string{
		"archived": {`UPDATE activity SET archived_at = occurred_at WHERE id = $1`},
		"restricted": {
			`INSERT INTO activity_retention_evidence (activity_id, basis, qualified_at, decided_by_name, reason)
			 SELECT id, 'controller_pin', occurred_at, 'Datenschutz', 'litigation hold' FROM activity WHERE id = $1`,
			`UPDATE activity SET restricted_at = occurred_at, archived_at = occurred_at,
			        restricted_reason = 'litigation hold', restricted_until = occurred_at + interval '1 year',
			        retention_class = 'commercial_correspondence', retention_class_at = occurred_at
			  WHERE id = $1`,
		},
		"erased": {`DELETE FROM activity WHERE id = $1`},
	}
	signed := map[string]ids.UUID{}
	for how, statements := range gone {
		contact := seedEnrichContact(t, e, how+"@window.example", "Works.\n\nBob Contact\nCTO")
		mail := onlyMailOf(t, e, contact)
		applySignature(t, e, contact, mail, signatureField("title", "CTO"))
		for _, sql := range statements {
			e.WsExec(t, sql, mail)
		}
		signed[how] = contact
	}

	runMachineAddressTask(principal.SystemActing(e.Admin(), "system:datatask-test"), t, e, signatureWindowDataTask())

	for how, contact := range signed {
		if title := contactTitle(t, e, contact); title != nil || len(signatureEvidence(t, e, contact)) != 0 {
			t.Errorf("%s mail: title %v and evidence %v remain, want both taken back", how, title, signatureEvidence(t, e, contact))
		}
	}
}
