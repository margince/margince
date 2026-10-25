// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/companybrief"
	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/contactbrief"
	"github.com/margince/margince/backend/internal/compose/draftcore"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reply captures one message on the thread rooted at root, through the mail
// parser and the production sink.
func (f *relationshipFixture) reply(t *testing.T, key, root, from, to string, daysAgo int, body ...string) {
	t.Helper()
	headers := []string{
		"From: " + from, "To: " + to, "Subject: Re: " + root,
		"Date: " + f.now.AddDate(0, 0, -daysAgo).Format(time.RFC1123Z),
		"Message-ID: <" + key + "@relationship.test>",
		"In-Reply-To: <" + root + "@relationship.test>", "References: <" + root + "@relationship.test>",
	}
	if len(body) == 0 {
		body = []string{"Content-Type: text/plain; charset=utf-8", "", "Words a human wrote about " + key + ".", ""}
	}
	captureMailVia(t, f.e, f.seat, "gmail", []byte(strings.Join(append(headers, body...), "\r\n")))
}

func (f *relationshipFixture) awaited(t *testing.T, activity ids.UUID) bool {
	t.Helper()
	f.e.WsExec(t, `UPDATE company SET lifecycle = 'customer' WHERE id = $1`, f.company)
	rows, _, err := activities.NewStore(f.e.DB()).AwaitingReplies(f.e.As(f.seat, nil, integration.AdminPerms), f.now)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ActivityID == activity {
			return true
		}
	}
	return false
}

// A question the reader sent stays awaited through a calendar answer and a
// delivery report on its thread. The customer's reply settles it. An
// invitation the reader sent awaits nothing.
func TestOnlyAHumansReplySettlesAnAwaitedQuestion(t *testing.T) {
	f := newRelationshipFixture(t)
	question := f.mail(t, "pricing-question", f.seatAddr, f.contactAddr, "", 4)
	invite := f.mail(t, "kickoff-invite", f.seatAddr, f.contactAddr, "", 4, calendarBody("REQUEST")...)
	f.reply(t, "pricing-accepted", "pricing-question", f.contactAddr, f.seatAddr, 2, calendarBody("REPLY")...)
	f.reply(t, "pricing-bounce", "pricing-question", "mailer-daemon@googlemail.com", f.seatAddr, 2)
	if !f.awaited(t, question) {
		t.Fatal("a calendar answer or a delivery report on the thread settled the reader's question")
	}
	if f.awaited(t, invite) {
		t.Fatal("an invitation the reader sent became a follow-up")
	}
	f.reply(t, "pricing-answer", "pricing-question", f.contactAddr, f.seatAddr, 1)
	if f.awaited(t, question) {
		t.Fatal("the customer's own reply did not settle the question")
	}
}

func (f *relationshipFixture) contactPages() *contact360.Service {
	clock := func() time.Time { return f.now }
	return contact360.NewService(f.e.Pool, f.e.Contacts, f.e.Deals, f.e.Projects,
		consent.NewStore(InstallationDB(f.e.Pool)),
		comms.NewStore(InstallationDB(f.e.Pool), clock, activities.NewStore(InstallationDB(f.e.Pool))),
		ai.NewFeedbackStore(InstallationDB(f.e.Pool)), clock)
}

func (f *relationshipFixture) companyPages() *company360.Service {
	return company360.NewService(f.e.Pool, f.e.Contacts, f.e.Deals, f.e.Projects,
		approvals.NewService(InstallationDB(f.e.Pool)), func() time.Time { return f.now })
}

func citesActivity(sentences []crmcontracts.CompanyBriefSentence, activity ids.UUID) bool {
	for _, sentence := range sentences {
		for _, evidence := range sentence.Evidence {
			if ids.UUID(evidence.EntityId) == activity {
				return true
			}
		}
	}
	return false
}

// More automatic mail than the page's timeline holds cannot push the newest
// human message out of the briefs and drafts. They read the rows a human
// wrote before the cap. The page keeps every record.
func TestAutomaticMailPastTheCapLeavesTheHumanMessageInTheBriefs(t *testing.T) {
	f := newRelationshipFixture(t)
	human := f.mail(t, "their-pricing-reply", f.contactAddr, f.seatAddr, "", 30)
	for i := range 26 {
		f.mail(t, "notice-"+string(rune('a'+i)), "Rep via Google Slides <drive-shares-dm-noreply@google.com>", f.contactAddr, f.seatAddr, 20)
	}
	ctx := f.e.Admin()
	contactID, companyID := ids.ContactID{UUID: f.contact}, ids.CompanyID{UUID: f.company}

	page, err := f.contactPages().Assemble(ctx, contactID)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range page.Activities.Data {
		if ids.UUID(row.Id) == human {
			t.Fatal("the fixture's notices do not fill the page's timeline; the cap is not exercised")
		}
	}
	brief, err := contactbrief.NewService(f.e.Pool, f.contactPages(), nil, "test", func() time.Time { return f.now }).Get(ctx, contactID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !citesActivity(brief.Sentences, human) {
		t.Errorf("the contact brief does not cite the human reply behind the notices: %+v", brief.Sentences)
	}
	company, err := companybrief.NewService(f.e.Pool, f.companyPages(), f.e.Contacts, nil, "test", func() time.Time { return f.now }).Get(ctx, companyID, true)
	if err != nil {
		t.Fatal(err)
	}
	var sentences []crmcontracts.CompanyBriefSentence
	for _, section := range company.Sections {
		sentences = append(sentences, section.Sentences...)
	}
	if !citesActivity(sentences, human) {
		t.Errorf("the company brief does not cite the human reply behind the notices: %+v", company.Sections)
	}
	draftView, err := f.companyPages().AssembleScoped(ctx, companyID, company360.AssembleOptions{WrittenByHumanOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(draftcore.LanguageEvidenceOf(draftView.Activities.Data, f.now), "\n"), "their-pricing-reply") {
		t.Error("the drafting language reads the notices instead of the human reply")
	}

	later := f.mail(t, "their-newer-reply", f.contactAddr, f.seatAddr, "", 1)
	brief, err = contactbrief.NewService(f.e.Pool, f.contactPages(), nil, "test", func() time.Time { return f.now }).Get(ctx, contactID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !citesActivity(brief.Sentences, later) {
		t.Errorf("the contact brief does not move to the newer human reply: %+v", brief.Sentences)
	}
}
