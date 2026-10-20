// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A stranger's Bcc or foreign-link mail ranks as routine work with its reason,
// even when the classifier read it as a request. The same mail from a sender
// the workspace has written to keeps the top band. Every message goes through
// the capture sink, the real parser, the waiting lane and the Worklist.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The Worklist's top band and its routine band, as the attention package
// numbers them.
const (
	worklistTopBand     = 1
	worklistRoutineBand = 6
)

// redirectedLink is a call-to-action wrapped in Google's link redirect, whose
// real target is a site that is neither the sender's nor ours.
const redirectedLink = `<p><a href="https://www.google.com/url?q=https://landing.example.org/doc/index.html&amp;sa=D">View the document</a></p>`

// requestFrom captures one mail and records the classifier's asks-us verdict on it.
func (o *owedEnv) requestFrom(t *testing.T, m owedMail) ids.UUID {
	t.Helper()
	m.messageID = "in-" + ids.NewV7().String() + "@mail.example"
	message := o.capture(t, m)
	judge := principal.WithActor(o.e.Admin(), o.classifier())
	applied, err := o.e.Activities.SetOwedVerdict(judge, message, activities.OwedVerdictAsksUs, "prompts-test", o.now)
	if err != nil || !applied {
		t.Fatalf("judging %s as a request: applied=%v err=%v", message, applied, err)
	}
	return message
}

// worklistRow reads the reader's Worklist and returns the waiting row for one message.
func (o *owedEnv) worklistRow(t *testing.T, message ids.UUID) crmcontracts.WorklistItem {
	t.Helper()
	feed := newAttentionService(o.e.Pool, approvals.NewService(o.e.DB()), func() time.Time { return o.now })
	page, err := feed.Worklist(o.reader(), "all", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatalf("reading the Worklist: %v", err)
	}
	for _, row := range page.Queue {
		if row.Id == message.String() {
			return row
		}
	}
	t.Fatalf("message %s is not on the Worklist; it must be demoted, never hidden", message)
	return crmcontracts.WorklistItem{}
}

func says(row crmcontracts.WorklistItem, kind crmcontracts.WorklistReasonKind) bool {
	for _, because := range row.Because {
		if because.Kind == kind {
			return true
		}
	}
	return false
}

// knownSender creates a contact the workspace wrote to a month ago.
func (o *owedEnv) knownSender(t *testing.T, address string) {
	t.Helper()
	o.contact(t, "Kim Known", address)
	o.weWrite(t, owedMail{to: address, subject: "Our offer", at: o.now.Add(-30 * 24 * time.Hour)})
}

func TestAStrangersBccRequestIsRoutineWhileAKnownSendersStaysOnTop(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Sam Sender", "sender@example.com")
	o.knownSender(t, "known@example.net")
	eightDaysAgo := o.now.Add(-8 * 24 * time.Hour)

	stranger := o.requestFrom(t, owedMail{
		from: "sender@example.com", to: "undisclosed-recipients:;",
		subject: "View the document", at: eightDaysAgo,
	})
	known := o.requestFrom(t, owedMail{
		from: "known@example.net", to: "undisclosed-recipients:;",
		subject: "Next steps", at: eightDaysAgo,
	})

	if n := o.count(t, `SELECT count(*) FROM activity WHERE id = $1
		AND bcc_only AND sender_was_stranger AND NOT foreign_link`, stranger); n != 1 {
		t.Fatal("capture did not record the stranger's mail as a Bcc copy from a stranger")
	}
	row := o.worklistRow(t, stranger)
	if row.Level != worklistRoutineBand || !says(row, "bcc_from_stranger") {
		t.Errorf("the stranger's Bcc request is at level %d with reasons %v, want routine with bcc_from_stranger",
			row.Level, row.Because)
	}
	row = o.worklistRow(t, known)
	if row.Level != worklistTopBand || says(row, "bcc_from_stranger") {
		t.Errorf("the known sender's Bcc request is at level %d with reasons %v, want the top band and no stranger reason",
			row.Level, row.Because)
	}
}

func TestAStrangersRequestWithAForeignLinkIsRoutineWhileAPlainOneStaysOnTop(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Sam Sender", "sender@example.com")
	o.contact(t, "Lee Linker", "lee@example.com")
	o.knownSender(t, "known@example.net")
	twoDaysAgo := o.now.Add(-2 * 24 * time.Hour)

	foreign := o.requestFrom(t, owedMail{
		from: "sender@example.com", to: o.seat,
		subject: "View the document", at: twoDaysAgo, html: redirectedLink,
	})
	ownSite := o.requestFrom(t, owedMail{
		from: "lee@example.com", to: o.seat,
		subject: "Our brochure", at: twoDaysAgo, html: `<a href="https://docs.example.com/brochure">Brochure</a>`,
	})
	known := o.requestFrom(t, owedMail{
		from: "known@example.net", to: o.seat,
		subject: "The document", at: twoDaysAgo, html: redirectedLink,
	})

	row := o.worklistRow(t, foreign)
	if row.Level != worklistRoutineBand || !says(row, "link_off_sender") || says(row, "bcc_from_stranger") {
		t.Errorf("the stranger's foreign-link request is at level %d with reasons %v, want routine with only link_off_sender",
			row.Level, row.Because)
	}
	for name, message := range map[string]ids.UUID{
		"a stranger's link to their own site":             ownSite,
		"a stranger's link to their company's other site": o.companySiteRequest(t, twoDaysAgo),
		"a known sender's foreign link":                   known,
	} {
		if row := o.worklistRow(t, message); row.Level != worklistTopBand {
			t.Errorf("%s left the top band: level %d, reasons %v", name, row.Level, row.Because)
		}
	}
}

// companySiteRequest is a stranger's request linking to the second domain of
// the company that holds the sender's own domain.
func (o *owedEnv) companySiteRequest(t *testing.T, at time.Time) ids.UUID {
	t.Helper()
	if _, err := o.e.Contacts.CreateCompany(o.e.Admin(), contacts.CreateCompanyInput{
		DisplayName: "Two Sites", Source: "manual",
		Domains: []contacts.CompanyDomainInput{
			{Domain: "twosites.example", IsPrimary: true}, {Domain: "twosites-docs.example"},
		},
	}); err != nil {
		t.Fatalf("creating the company: %v", err)
	}
	o.contact(t, "Ana Twosites", "ana@twosites.example")
	return o.requestFrom(t, owedMail{
		from: "ana@twosites.example", to: o.seat, subject: "Our paper",
		at: at, html: `<a href="https://twosites-docs.example/paper">Paper</a>`,
	})
}

// Mail captured before these facts existed carries NULL, and NULL is no
// evidence: such mail ranks as it did.
func TestMailWithNoCapturedFactsRanksAsBefore(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Sam Sender", "sender@example.com")
	stranger := o.requestFrom(t, owedMail{
		from: "sender@example.com", to: "undisclosed-recipients:;",
		subject: "View the document", at: o.now.Add(-2 * 24 * time.Hour),
	})
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET bcc_only = NULL, foreign_link = NULL, sender_was_stranger = NULL WHERE id = $1`, stranger)
		return err
	}); err != nil {
		t.Fatalf("clearing the captured facts: %v", err)
	}

	if row := o.worklistRow(t, stranger); row.Level != worklistTopBand {
		t.Errorf("mail with no captured facts is at level %d with reasons %v, want the top band", row.Level, row.Because)
	}
}
