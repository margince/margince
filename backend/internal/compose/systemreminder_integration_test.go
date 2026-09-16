// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A task the system minted is not a promise. The clock starters file
// check-in and renewal reminders as ordinary open tasks, and both record
// pages rank open tasks into a "You owe them" card — so without a
// provenance filter, the product's own nudge to the reader is presented
// as a promise the reader made to the account. The reminder stays a task
// on the list; it may not be a promise on the card, on either page.

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestASystemMintedReminderIsNeverTheOwedPromise(t *testing.T) {
	e := integration.Setup(t)
	contact := seedLinkedContact(t, e, "erinnert@kunde.example")
	company := seedEmployerOf(t, e, contact, "Erinnert GmbH")
	now := time.Now()

	// The human promise is not yet due; the system reminders are OVERDUE, so
	// unfiltered either wins both pages' ranking (the most recently slipped
	// promise outranks one still ahead) and the defect reads loudest. One
	// reminder per arm of the predicate: the workflow engine binds the bare
	// "system" (automation's systemActor), and jobs elsewhere bind their own
	// "system:<job>" — a filter that saw only one spelling would let the
	// other through.
	promised := logTaskDueAt(t, e, contact, "Send the signed contract",
		now.Add(24*time.Hour), now.Add(-48*time.Hour))
	logSystemReminderFor(t, e, contact, "system",
		"Check in — no activity since 2026-09-08", now.Add(-24*time.Hour))
	logSystemReminderFor(t, e, contact, "system:time-scan",
		"Renewal comes up on 2026-10-01", now.Add(-12*time.Hour))

	contactSvc := contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), time.Now, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), time.Now)
	companySvc := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		approvals.NewService(InstallationDB(e.Pool)), time.Now)

	contactPage, err := contactSvc.Assemble(e.Admin(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatalf("assembling the contact: %v", err)
	}
	companyPage, err := companySvc.Assemble(e.Admin(), ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatalf("assembling the account: %v", err)
	}

	// The reminders are still real work: they stay on both task lists.
	if contactPage.NextSteps == nil || len(contactPage.NextSteps.Data) != 3 {
		t.Fatalf("the contact's task list carries %v, want all three tasks — the filter is for "+
			"the promise card, not the list", contactPage.NextSteps)
	}
	if companyPage.NextSteps == nil || len(companyPage.NextSteps.Data) != 3 {
		t.Fatalf("the account's task list carries %v, want all three tasks", companyPage.NextSteps)
	}

	// But it is nobody's promise: both cards name the human-filed one.
	if contactPage.Moment == nil {
		t.Fatal("no moment on a contact who is owed a signed contract")
	}
	if got := contactPage.Moment.Headline; got != "You owe them: Send the signed contract" {
		t.Errorf("contact moment = %q — a system-minted reminder presented as a promise tells "+
			"the reader they promised a check-in nobody promised", got)
	}
	if companyPage.Moment == nil {
		t.Fatal("no moment on an account that is owed a signed contract")
	}
	if got := companyPage.Moment.Headline; got != "You owe them: Send the signed contract" {
		t.Errorf("account moment = %q, want the human-filed promise", got)
	}
	if got := companyPage.Moment.Evidence; len(got) != 1 ||
		ids.UUID(*got[0].Id) != promised {
		t.Errorf("the account card's evidence is %v, want the human-filed task %v", got, promised)
	}
}

// A full page of overdue reminders must not crowd the one genuine promise
// out of the account card's bounded read: the exclusion runs in the
// statement, before the bound, or the card would answer "nothing is owed"
// to the account that owes the most. The same seeding drives the
// review_commitments sweep, which must tell the reader the same thing the
// card does.
func TestAPageOfRemindersDoesNotBuryTheOnePromise(t *testing.T) {
	e := integration.Setup(t)
	contact := seedLinkedContact(t, e, "verschuettet@kunde.example")
	company := seedEmployerOf(t, e, contact, "Verschuettet GmbH")
	now := time.Now()

	promised := logTaskDueAt(t, e, contact, "Send the signed contract",
		now.Add(24*time.Hour), now.Add(-48*time.Hour))
	// More overdue reminders than the section's page holds, every one of
	// them due before the promise, under the id the workflow engine binds.
	for i := range 28 {
		logSystemReminderFor(t, e, contact, "system",
			fmt.Sprintf("Check in — no activity since day %02d", i+1),
			now.Add(-time.Duration(i+1)*24*time.Hour))
	}

	companySvc := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		approvals.NewService(InstallationDB(e.Pool)), time.Now)
	companyPage, err := companySvc.Assemble(e.Admin(), ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatalf("assembling the account: %v", err)
	}
	if companyPage.Moment == nil {
		t.Fatal("no moment on an account that owes a signed contract")
	}
	if got := companyPage.Moment.Headline; got != "You owe them: Send the signed contract" {
		t.Errorf("account moment = %q — the promise fell out of the read's bound behind "+
			"a page of reminders", got)
	}
	if got := companyPage.Moment.Evidence; len(got) != 1 ||
		ids.UUID(*got[0].Id) != promised {
		t.Errorf("the card's evidence is %v, want the human-filed task %v", got, promised)
	}

	// The tool surface answers the same question and must give the same
	// answer: the promise, and not one reminder.
	sweep, err := commitmentLister(e.Pool)(e.Admin(), agents.CommitmentQuery{})
	if err != nil {
		t.Fatalf("sweeping commitments: %v", err)
	}
	var subjects []string
	for _, c := range sweep.Commitments {
		subjects = append(subjects, c.Subject)
		if strings.HasPrefix(c.Subject, "Check in — no activity since") {
			t.Errorf("review_commitments reports %q as an open commitment — the tool is "+
				"telling the reader they promised a check-in the clock minted", c.Subject)
		}
	}
	if !slices.Contains(subjects, "Send the signed contract") {
		t.Errorf("review_commitments answered %v, want the human-filed promise in it", subjects)
	}
}

// The promise read's bound is its own, wider than the section's page: the
// card ranks over the set and asks which promise slipped LAST, and on an
// account owing more than a page of tasks that row is exactly the one a cut
// at the page would drop.
func TestTheAccountCardSeesThePromiseThatSlippedLast(t *testing.T) {
	e := integration.Setup(t)
	contact := seedLinkedContact(t, e, "vielversprochen@kunde.example")
	company := seedEmployerOf(t, e, contact, "Vielversprochen GmbH")
	now := time.Now()

	// 26 overdue human promises. In due-date order the one that slipped
	// yesterday is row 26 — past the section's 25-row page.
	for i := range 25 {
		logTaskDueAt(t, e, contact, fmt.Sprintf("Old chore %02d", i+1),
			now.Add(-time.Duration(26-i)*24*time.Hour), now.Add(-60*24*time.Hour))
	}
	logTaskDueAt(t, e, contact, "Send the revised offer",
		now.Add(-24*time.Hour), now.Add(-60*24*time.Hour))

	companySvc := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		approvals.NewService(InstallationDB(e.Pool)), time.Now)
	companyPage, err := companySvc.Assemble(e.Admin(), ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatalf("assembling the account: %v", err)
	}
	if companyPage.Moment == nil {
		t.Fatal("no moment on an account owing twenty-six promises")
	}
	if got := companyPage.Moment.Headline; got != "You owe them: Send the revised offer" {
		t.Errorf("account moment = %q, want the promise that slipped last — a read cut at "+
			"the section's page names a stale chore instead of the one still worth rescuing", got)
	}
}

// logSystemReminderFor writes one open task the way the product's own runs
// do: through the real activity writer, under a system principal, so
// captured_by carries the same provenance a clock reminder carries in
// production. The caller names the id, because both spellings exist in
// production — the workflow engine binds the bare "system" and jobs bind
// "system:<job>" — and each test says which arm it is holding.
func logSystemReminderFor(t *testing.T, e *integration.Env, contact ids.UUID, actorID, subject string, due time.Time) {
	t.Helper()
	ctx := principal.WithActor(e.Admin(),
		principal.Principal{Type: principal.PrincipalSystem, ID: actorID})
	if _, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, Source: "system",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	}); err != nil {
		t.Fatalf("logging the system reminder: %v", err)
	}
}
