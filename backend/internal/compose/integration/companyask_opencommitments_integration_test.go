// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// "What is still open on this company?" with a task and a promise read out of a
// conversation, both written through the real writers.

import (
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type openWorkAccount struct {
	company  ids.CompanyID
	contact  ids.UUID
	taskID   ids.UUID
	activity ids.UUID
}

func seedOpenWork(t *testing.T, e *Env, contactOwner ids.UUID) openWorkAccount {
	t.Helper()
	ctx := e.Admin()
	companyID := e.SeedCompany(t, "Acme", &e.Rep1)
	contact := e.SeedContact(t, "Carol", &contactOwner)
	e.WsExec(t, `INSERT INTO relationship(kind,contact_id,company_id,source,captured_by) VALUES('employment',$1,$2,'manual','human:test')`, contact, companyID)

	taskSubject, due := "Call the CFO", briefClock.Add(24*time.Hour)
	task, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: "task", Subject: &taskSubject, DueAt: &due, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "company", EntityID: companyID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	subject, body, direction, at := "Re: audit", "I will send the security audit.", "outbound", briefClock.Add(-72*time.Hour)
	message, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	promiseDue := briefClock.Add(48 * time.Hour)
	e.RecordClaim(ctx, t, contacts.ClaimInput{
		ContactID: ids.From[ids.ContactKind](contact), Kind: "commitment_ours", Body: "Send the security audit",
		Quote: body, ActivityID: ids.UUID(message.Id), DueAt: &promiseDue, Source: "manual",
	})
	return openWorkAccount{
		company: ids.From[ids.CompanyKind](companyID), contact: contact,
		taskID: ids.UUID(task.Id), activity: ids.UUID(message.Id),
	}
}

func TestWhatIsOpenNamesThePromiseEvenWhenTheModelNamesOnlyTheTask(t *testing.T) {
	e := Setup(t)
	account := seedOpenWork(t, e, e.Rep1)
	lane := &countingLane{reply: `{"sentences":[{"text":"You have one open task.","evidence":[{"entity_type":"activity","entity_id":"` +
		account.taskID.String() + `"}]}]}`}
	svc := briefService(e, lane, "routing-1")

	answer, err := svc.Ask(e.As(e.Rep1, nil, briefReaderPerms), account.company, crmcontracts.CompanyQuestionWhatsOpen)
	if err != nil {
		t.Fatal(err)
	}
	if answer.GeneratedBy != crmcontracts.WrittenByDeterministic {
		t.Fatalf("generated_by = %q, want the floor: the reply left the promise out", answer.GeneratedBy)
	}
	var text string
	var chip string
	for _, sentence := range answer.Sentences {
		text += sentence.Text + "\n"
		for _, cite := range sentence.Evidence {
			if cite.EntityId.String() == account.activity.String() && cite.Name != nil {
				chip = *cite.Name
			}
		}
	}
	if !strings.Contains(text, "Send the security audit") || !strings.Contains(text, "Call the CFO") {
		t.Errorf("answer %q does not name both the task and the promise", text)
	}
	if strings.Contains(text, "Carol") {
		t.Errorf("answer %q names a person for a promise our side made", text)
	}
	if chip != "Send the security audit" {
		t.Errorf("the promise's chip is named %q, want the claim text", chip)
	}
}

func TestAReaderWhoCannotSeeTheContactIsToldTheListMayBeIncomplete(t *testing.T) {
	e := Setup(t)
	account := seedOpenWork(t, e, e.Rep3)
	e.MakeCapturePrivate(t, "contact", account.contact, e.Rep3)
	svc := briefService(e, nil, "")

	scoped := briefReaderPerms
	scoped.RowScope = principal.RowScopeTeam
	answer, err := svc.Ask(e.As(e.Rep1, []ids.UUID{e.Team1}, scoped), account.company, crmcontracts.CompanyQuestionWhatsOpen)
	if err != nil {
		t.Fatal(err)
	}
	var text string
	for _, sentence := range answer.Sentences {
		text += sentence.Text + "\n"
	}
	if strings.Contains(text, "Send the security audit") {
		t.Errorf("answer %q names a promise whose contact this reader cannot see", text)
	}
	if !strings.Contains(text, "may not show everything") {
		t.Errorf("answer %q does not say the list may be incomplete", text)
	}
}
