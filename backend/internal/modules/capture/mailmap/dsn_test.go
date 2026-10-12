// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// hardBounceFixture is the RFC 3464 shape the large mail systems send: a
// human-readable part, the machine-readable delivery-status part, and the
// original message's headers.
func hardBounceFixture() []byte {
	return crlf(
		"From: Mail Delivery Subsystem <mailer-daemon@mail.example.com>",
		"To: me@myco.com",
		"Subject: Undelivered Mail Returned to Sender",
		"Message-ID: <report-1@mail.example.com>",
		"Content-Type: multipart/report; report-type=delivery-status; boundary=\"BOUND\"",
		"",
		"--BOUND",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Your message could not be delivered.",
		"--BOUND",
		"Content-Type: message/delivery-status",
		"",
		"Reporting-MTA: dns; mail.example.com",
		"",
		"Final-Recipient: rfc822; gone@customer.example",
		"Action: failed",
		"Status: 5.1.1",
		"Diagnostic-Code: smtp; 550 5.1.1 user unknown",
		"",
		"--BOUND",
		"Content-Type: text/rfc822-headers",
		"",
		"From: me@myco.com",
		"To: gone@customer.example",
		"Message-ID: <sent-42@myco.com>",
		"Subject: The offer",
		"",
		"--BOUND--",
		"",
	)
}

func TestParseBounceReadsAHardBounce(t *testing.T) {
	report, ok := ParseBounce(hardBounceFixture())
	if !ok {
		t.Fatal("a permanent-failure DSN was not read as a bounce")
	}
	if report.MessageID != "sent-42@myco.com" {
		t.Errorf("MessageID = %q, want the ORIGINAL message's id sent-42@myco.com", report.MessageID)
	}
	if report.Kind != connector.BounceHard {
		t.Errorf("Kind = %q, want hard — 5.1.1 is a permanent refusal", report.Kind)
	}
	if report.Reason != "550 5.1.1 user unknown" {
		t.Errorf("Reason = %q, want the diagnostic text without its smtp; prefix", report.Reason)
	}
	if report.Recipient != "gone@customer.example" {
		t.Errorf("Recipient = %q, want the Final-Recipient address", report.Recipient)
	}
}

func TestParseBounceReadsATemporaryFailureAsSoft(t *testing.T) {
	raw := crlf(
		"From: mailer-daemon@mail.example.com",
		"To: me@myco.com",
		"Subject: Delivery problem",
		"Message-ID: <report-2@mail.example.com>",
		"In-Reply-To: <sent-43@myco.com>",
		"Content-Type: multipart/report; report-type=delivery-status; boundary=\"B2\"",
		"",
		"--B2",
		"Content-Type: message/delivery-status",
		"",
		"Reporting-MTA: dns; mail.example.com",
		"",
		"Final-Recipient: rfc822; away@customer.example",
		"Action: failed",
		"Status: 4.2.2",
		"Diagnostic-Code: smtp; 452 4.2.2 mailbox full",
		"",
		"--B2--",
		"",
	)
	report, ok := ParseBounce(raw)
	if !ok {
		t.Fatal("a temporary-failure DSN was not read as a bounce")
	}
	if report.Kind != connector.BounceSoft {
		t.Errorf("Kind = %q, want soft — 4.2.2 says nothing durable about the address", report.Kind)
	}
	if report.MessageID != "sent-43@myco.com" {
		t.Errorf("MessageID = %q, want the envelope In-Reply-To fallback sent-43@myco.com", report.MessageID)
	}
}

// A delay notice says the mail has NOT failed; recording it as a bounce would
// tell a rep their mail did not arrive while the system is still trying.
func TestParseBounceIgnoresADelayNotice(t *testing.T) {
	raw := crlf(
		"From: mailer-daemon@mail.example.com",
		"To: me@myco.com",
		"Subject: Delivery delayed",
		"Message-ID: <report-3@mail.example.com>",
		"In-Reply-To: <sent-44@myco.com>",
		"Content-Type: multipart/report; report-type=delivery-status; boundary=\"B3\"",
		"",
		"--B3",
		"Content-Type: message/delivery-status",
		"",
		"Reporting-MTA: dns; mail.example.com",
		"",
		"Action: delayed",
		"Status: 4.4.1",
		"",
		"--B3--",
		"",
	)
	if _, ok := ParseBounce(raw); ok {
		t.Error("a delay notice was read as a bounce")
	}
}

func TestParseBounceIgnoresOrdinaryMail(t *testing.T) {
	if _, ok := ParseBounce(inboundFixture()); ok {
		t.Error("an ordinary message was read as a bounce")
	}
}

type recordingBounceSink struct {
	reports []connector.BounceReport
	err     error
}

func (r *recordingBounceSink) RecordBounce(_ context.Context, report connector.BounceReport) error {
	r.reports = append(r.reports, report)
	return r.err
}

func TestRecordIfBounceHandsTheReportToTheSink(t *testing.T) {
	sink := &recordingBounceSink{}
	if err := RecordIfBounce(context.Background(), hardBounceFixture(), sink); err != nil {
		t.Fatalf("RecordIfBounce: %v", err)
	}
	if len(sink.reports) != 1 || sink.reports[0].MessageID != "sent-42@myco.com" {
		t.Errorf("sink got %+v, want the one hard-bounce report", sink.reports)
	}
	if err := RecordIfBounce(context.Background(), inboundFixture(), sink); err != nil {
		t.Fatalf("RecordIfBounce on ordinary mail: %v", err)
	}
	if len(sink.reports) != 1 {
		t.Error("ordinary mail reached the bounce sink")
	}
}

// A nil sink is a connector wired without bounce recording; the report is
// dropped exactly as before. A sink error propagates — losing a bounce
// silently is the invisibility this path exists to end.
func TestRecordIfBounceNilSinkAndSinkError(t *testing.T) {
	if err := RecordIfBounce(context.Background(), hardBounceFixture(), nil); err != nil {
		t.Fatalf("nil sink: %v", err)
	}
	refusal := errors.New("comms is unreachable")
	sink := &recordingBounceSink{err: refusal}
	if err := RecordIfBounce(context.Background(), hardBounceFixture(), sink); !errors.Is(err, refusal) {
		t.Errorf("sink error = %v, want it propagated", err)
	}
}

// bounceFromAnOrdinaryDomainFixture is the same RFC 3464 shape sent by a
// reporter whose From looks like anyone else's — which the RFC permits and real
// systems do, whatever the mailer-daemon convention suggests.
func bounceFromAnOrdinaryDomainFixture() []byte {
	return crlf(
		"From: Delivery Reports <reports@relay.partner.example>",
		"To: me@myco.com",
		"Subject: Delivery Status Notification (Failure)",
		"Message-ID: <report-9@relay.partner.example>",
		"Content-Type: multipart/report; report-type=delivery-status; boundary=\"B9\"",
		"",
		"--B9",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Your message could not be delivered.",
		"--B9",
		"Content-Type: message/delivery-status",
		"",
		"Reporting-MTA: dns; relay.partner.example",
		"",
		"Final-Recipient: rfc822; gone@customer.example",
		"Action: failed",
		"Status: 5.1.1",
		"",
		"--B9",
		"Content-Type: text/rfc822-headers",
		"",
		"From: me@myco.com",
		"To: gone@customer.example",
		"Message-ID: <sent-77@myco.com>",
		"",
		"--B9--",
		"",
	)
}

// A report is read by its SHAPE, not by who signed it. RFC 3464 puts no
// requirement on the From, so a reporter using its own domain was a bounce the
// workspace simply never learned about — the address stayed on the list and the
// next send failed the same way.
func TestParseBounceReadsAReportFromAnOrdinaryDomain(t *testing.T) {
	report, ok := ParseBounce(bounceFromAnOrdinaryDomainFixture())
	if !ok {
		t.Fatal("a well-formed delivery report was not read as one because of its From — " +
			"the sender convention is not what RFC 3464 defines a report by")
	}
	if report.Recipient != "gone@customer.example" {
		t.Errorf("the report names %q as the failed recipient", report.Recipient)
	}
	if report.Kind != connector.BounceHard {
		t.Errorf("Kind = %q, want hard — 5.1.1 is a permanent refusal", report.Kind)
	}
}

// And it reaches the drop branch, which is the half that carries it to the
// recorder: every connector asks SkipReason first, so a report it does not drop
// is one the bounce path never sees.
func TestADeliveryReportIsDroppedWhoeverSentIt(t *testing.T) {
	msg, err := Parse(bounceFromAnOrdinaryDomainFixture(), "me@myco.com")
	if err != nil {
		t.Fatalf("parsing the report: %v", err)
	}
	reason, drop := msg.SkipReason()
	if !drop {
		t.Fatal("a delivery report from an ordinary domain was captured as correspondence — " +
			"it lands on the timeline as a message from a contact, and never reaches the " +
			"bounce recorder the drop branch feeds")
	}
	if reason != "delivery report" {
		t.Errorf("it was dropped as %q; the shape is what recognised it", reason)
	}
}
