// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A lead's refusal owes the receipt its rules require, sent to the lead's own
// address and filed under the lead, as a contact's is.

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const leadReceiptAddress = "lead-refusal@example.test"

// refusingLead seeds a lead in a country whose rules owe a receipt. A nil
// email is a lead the product holds no address for.
func (l *confirmLaneEnv) refusingLead(t *testing.T, email *string) ids.UUID {
	t.Helper()
	company := l.e.SeedCompany(t, "Receipt Company", nil)
	l.e.WsExec(t, `UPDATE company SET address_country = upper($2) WHERE id = $1`, company, string(receiptOwed))
	lead, _, err := l.e.Contacts.CreateLead(l.e.Admin(), contacts.CreateLeadInput{
		FullName: new("Lead Refusal"), Email: email, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	l.e.WsExec(t, `UPDATE lead SET company_id = $2 WHERE id = $1`, lead.Id, company)
	return ids.UUID(lead.Id)
}

// pressLeadLink records the lead's own unsubscribe press.
func (l *confirmLaneEnv) pressLeadLink(t *testing.T, lead ids.UUID, address string) {
	t.Helper()
	if err := InstallationDB(l.e.Pool).Tx(l.ctx, func(tx pgx.Tx) error {
		stopped, err := l.store.StopForCredentialTx(l.ctx, tx, consent.WithdrawalRef{
			LeadID: ids.From[ids.LeadKind](lead), Address: address, Scope: consent.WithdrawalScopeAllMarketing,
		})
		if err == nil && !stopped {
			t.Error("the press recorded no stop")
		}
		return err
	}); err != nil {
		t.Fatalf("pressing the lead's link: %v", err)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM communication_suppression WHERE lead_id = $1 AND revoked_at IS NULL`, lead); n != 1 {
		t.Fatalf("%d live stops on the lead, want the refusal recorded whatever happened to its receipt", n)
	}
}

// leadNotices answers the live missed-receipt notices about one lead.
func (l *confirmLaneEnv) leadNotices(t *testing.T, lead ids.UUID) []string {
	t.Helper()
	rows, err := l.e.Pool.Query(context.Background(), `
		SELECT body FROM notice
		 WHERE kind = 'optout_receipt_missed' AND target_type = 'lead' AND target_id = $1 AND retracted_at IS NULL`, lead)
	if err != nil {
		t.Fatal(err)
	}
	bodies, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return bodies
}

// leadReceiptDue reads the deadline of the receipt filed under the lead, and
// fails when none is.
func (l *confirmLaneEnv) leadReceiptDue(t *testing.T, lead ids.UUID) time.Time {
	t.Helper()
	var due time.Time
	if err := l.e.Pool.QueryRow(context.Background(), `
		SELECT o.delivery_due_at FROM comms_outbound o
		  JOIN activity_link al ON al.activity_id = o.activity_id
		 WHERE o.template_key = 'optout_acknowledgement' AND al.entity_type = 'lead' AND al.lead_id = $1
		   AND o.recipients ? $2 AND o.delivery_due_at IS NOT NULL`, lead, leadReceiptAddress).Scan(&due); err != nil {
		t.Fatalf("no receipt to the lead's address filed under the lead: %v", err)
	}
	return due
}

func TestALeadsPressGetsTheReceiptItsRulesOwe(t *testing.T) {
	l := receiptLane(t, receiptOwed)
	lead := l.refusingLead(t, new(leadReceiptAddress))
	l.pressLeadLink(t, lead, leadReceiptAddress)

	l.leadReceiptDue(t, lead)
	if bodies := l.leadNotices(t, lead); len(bodies) != 0 {
		t.Fatalf("the lead's receipt was staged and the operators were told it was missed: %q", bodies)
	}
}

func TestALeadWithNoAddressRaisesTheNoAddressNotice(t *testing.T) {
	l := receiptLane(t, receiptOwed)
	lead := l.refusingLead(t, nil)
	l.pressLeadLink(t, lead, "")

	if n := l.e.WsCount(t, `SELECT count(*) FROM comms_outbound WHERE template_key = 'optout_acknowledgement'`); n != 0 {
		t.Fatalf("%d receipts staged for a lead with no address", n)
	}
	bodies := l.leadNotices(t, lead)
	if len(bodies) == 0 || !strings.Contains(bodies[0], "no email address") {
		t.Fatalf("a lead's receipt with nowhere to go raised %q", bodies)
	}
}

func TestAnOverdueLeadReceiptIsRaisedOnTheLeadAndSettledWhenSent(t *testing.T) {
	l := receiptLane(t, receiptOwed)
	lead := l.refusingLead(t, new(leadReceiptAddress))
	l.pressLeadLink(t, lead, leadReceiptAddress)
	due := l.leadReceiptDue(t, lead)
	vault := keyvault.NewMemory()
	if err := DriveControllerPayloadSweepForTest(context.Background(), l.e.Pool, vault, due.Add(time.Minute)); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	bodies := l.leadNotices(t, lead)
	if len(bodies) == 0 || !strings.Contains(bodies[0], "has not been delivered") {
		t.Fatalf("an overdue lead receipt raised %q on the lead", bodies)
	}

	l.recordReceiptSent(t)
	if err := DriveControllerPayloadSweepForTest(context.Background(), l.e.Pool, vault, due.Add(2*time.Minute)); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if bodies := l.leadNotices(t, lead); len(bodies) != 0 {
		t.Fatalf("the lead's receipt was sent and %d notices still stand", len(bodies))
	}
}

// A lead erased with its contact before the deadline keeps no receipt to send
// and raises no notice.
func TestALeadErasedBeforeItsReceiptIsSentGetsNeitherReceiptNorNotice(t *testing.T) {
	l := receiptLane(t, receiptOwed)
	lead := l.refusingLead(t, new(leadReceiptAddress))
	l.e.WsExec(t, `UPDATE lead SET from_contact_id = $2 WHERE id = $1`, lead, l.contact)
	l.pressLeadLink(t, lead, leadReceiptAddress)
	due := l.leadReceiptDue(t, lead)

	if err := ComposedEraser(l.e.Pool, nil, nil).EraseContact(l.e.Admin(), l.contact, "Art. 17 request"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}
	if n := l.e.WsCount(t, `
		SELECT count(*) FROM comms_outbound
		 WHERE template_key = 'optout_acknowledgement' AND cardinality(redacted_fields) = 0`); n != 0 {
		t.Fatalf("%d receipts to the erased lead are still sendable", n)
	}
	if err := DriveControllerPayloadSweepForTest(context.Background(), l.e.Pool, keyvault.NewMemory(), due.Add(time.Minute)); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM notice WHERE kind = 'optout_receipt_missed'`); n != 0 {
		t.Fatalf("%d notices raised about a lead erased before its deadline", n)
	}
}

// A link refusal is answered at the address it was made from. The lead may
// have changed its address since the link was minted.
func TestALinkRefusalIsAnsweredAtTheAddressItWasMadeFrom(t *testing.T) {
	l := receiptLane(t, receiptOwed)
	lead := l.refusingLead(t, new(leadReceiptAddress))
	l.e.WsExec(t, `UPDATE lead SET email = 'lead-moved@example.test' WHERE id = $1`, lead)
	l.pressLeadLink(t, lead, leadReceiptAddress)

	l.leadReceiptDue(t, lead)
	if n := l.e.WsCount(t, `SELECT count(*) FROM comms_outbound
		WHERE template_key = 'optout_acknowledgement' AND recipients ? 'lead-moved@example.test'`); n != 0 {
		t.Fatalf("%d receipts went to the lead's new address, which made no refusal", n)
	}
}

// receiptDecisions answers the subject each phase's decision about the lead's
// receipt was taken on, and its verdict.
func (l *confirmLaneEnv) receiptDecisions(t *testing.T, phase string) []string {
	t.Helper()
	rows, err := l.e.Pool.Query(context.Background(), `
		SELECT coalesce(d.subject_kind, '') || ':' || coalesce(d.subject_id::text, '') || ':' || d.verdict
		  FROM communication_decision d
		  JOIN comms_outbound o ON o.id = d.delivery_id
		 WHERE o.template_key = 'optout_acknowledgement' AND d.phase = $1`, phase)
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// transmitLeadReceipt asks the engine's transmit phase about the staged receipt.
func (l *confirmLaneEnv) transmitLeadReceipt(t *testing.T) commsauthz.TransmitTicket {
	t.Helper()
	var delivery ids.UUID
	var subject, body string
	var html *string
	if err := l.e.Pool.QueryRow(context.Background(), `
		SELECT id, subject, body, html_body FROM comms_outbound WHERE template_key = 'optout_acknowledgement'`).
		Scan(&delivery, &subject, &body, &html); err != nil {
		t.Fatal(err)
	}
	req := commsauthz.TransmitRequest{
		DeliveryID: delivery, Attempt: 1, Subject: subject, Body: body,
		Recipients: []connector.Recipient{{Email: leadReceiptAddress}},
	}
	if html != nil {
		req.HTMLBody = *html
	}
	ticket, err := consentGateFor(l.e.Pool).AuthorizeTransmit(sendWorkerScope(l.ctx), req)
	if err != nil {
		t.Fatalf("authorizing the receipt's transmit: %v", err)
	}
	return ticket
}

// A lead's receipt to an address a contact also holds is decided on the lead.
// That holds at staging and at transmit, whatever the contact's record says.
func TestALeadReceiptToASharedAddressIsDecidedOnTheLead(t *testing.T) {
	for name, contactObjected := range map[string]bool{
		"the contact never objected": false,
		"the contact objected too":   true,
	} {
		t.Run(name, func(t *testing.T) {
			l := receiptLane(t, receiptOwed)
			l.e.WsExec(t, `UPDATE contact_email SET email = $2 WHERE contact_id = $1`, l.contact, leadReceiptAddress)
			if contactObjected {
				l.e.WsExec(t, `INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level)
					VALUES ($1, 'marketing_objection', 'test', 'human:test', 'subject')`, l.contact)
			}
			lead := l.refusingLead(t, new("lead-own@example.test"))
			l.e.WsExec(t, `UPDATE lead SET email = $2 WHERE id = $1`, lead, leadReceiptAddress)
			l.pressLeadLink(t, lead, leadReceiptAddress)
			l.leadReceiptDue(t, lead)

			want := "lead:" + lead.String() + ":allow"
			if got := l.receiptDecisions(t, "staging"); len(got) != 1 || got[0] != want {
				t.Fatalf("staging decided %q, want %q", got, want)
			}
			if ticket := l.transmitLeadReceipt(t); !ticket.Allowed {
				t.Fatalf("transmit refused the lead's receipt: %s", ticket.Reason)
			}
			if got := l.receiptDecisions(t, "transmit"); len(got) != 1 || got[0] != want {
				t.Fatalf("transmit decided %q, want %q", got, want)
			}
		})
	}
}

// Retention anonymizing a lead parks its waiting receipt and takes back the
// notice that the receipt is overdue.
func TestRetentionAnonymizingALeadParksItsReceiptAndRetractsItsNotice(t *testing.T) {
	l := receiptLane(t, receiptOwed)
	lead := l.refusingLead(t, new(leadReceiptAddress))
	l.pressLeadLink(t, lead, leadReceiptAddress)
	due := l.leadReceiptDue(t, lead)
	if err := DriveControllerPayloadSweepForTest(context.Background(), l.e.Pool, keyvault.NewMemory(), due.Add(time.Minute)); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if len(l.leadNotices(t, lead)) == 0 {
		t.Fatal("no overdue notice to take back; the test proves nothing")
	}
	l.e.WsExec(t, `INSERT INTO retention_policy (object_type, category, retain_days, action)
		VALUES ('lead', 'unconverted', 365, 'anonymize')`)
	l.e.WsExec(t, `UPDATE lead SET created_at = now() - interval '400 days', entered_at = now() - interval '400 days'
		WHERE id = $1`, lead)

	svc := NewRetentionServiceFor(l.e.DB(), nil, slog.Default())
	integration.SettleIntoInstall(t, l.e)
	if err := svc.EvaluateInstallation(integration.RetentionPassCtx(l.e.WS)); err != nil {
		t.Fatalf("EvaluateInstallation: %v", err)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM lead WHERE id = $1 AND full_name = 'Anonymized Lead'`, lead); n != 1 {
		t.Fatal("retention did not anonymize the lead; the test proves nothing")
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM comms_outbound
		WHERE template_key = 'optout_acknowledgement' AND (recipients <> '[]'::jsonb OR status = 'pending')`); n != 0 {
		t.Fatalf("%d receipts to the anonymized lead still carry its address or can still go", n)
	}
	if bodies := l.leadNotices(t, lead); len(bodies) != 0 {
		t.Fatalf("%d notices still name the anonymized lead", len(bodies))
	}
}

// A mail filed under a lead may only go to an address the lead holds. The pin
// narrows who a message is about and never reaches somebody new.
func TestAMailFiledUnderALeadIsRefusedToAnAddressTheLeadDoesNotHold(t *testing.T) {
	l := receiptLane(t, receiptOwed)
	lead := l.refusingLead(t, new(leadReceiptAddress))
	l.pressLeadLink(t, lead, leadReceiptAddress)
	rendered, category, err := consent.RenderControllerTemplate(consent.TemplateOptOutAcknowledgement, time.Time{}, "en")
	if err != nil {
		t.Fatal(err)
	}
	inserter, err := jobs.NewInserter(l.e.Pool, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	queue := NewControllerMailQueue(l.e.Pool, inserter, laneFromAddress)
	for address, allowed := range map[string]bool{leadReceiptAddress: true, "stranger@example.test": false} {
		err := InstallationDB(l.e.Pool).Tx(l.ctx, func(tx pgx.Tx) error {
			_, err := queue.PresentConfirmationTx(l.ctx, tx, consent.ConfirmationSend{
				LeadID: ids.From[ids.LeadKind](lead), Recipient: address, Category: category, Rendered: rendered,
			})
			return err
		})
		if allowed && err != nil {
			t.Errorf("the receipt to the lead's own address was refused: %v", err)
		}
		if !allowed && err == nil {
			t.Errorf("a mail filed under the lead was admitted to %s, an address the lead does not hold", address)
		}
	}
}
