// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnUnrelatedLaterEmailDoesNotSuggestTheTaskWasCompleted(t *testing.T) {
	e := integration.Setup(t)
	const address = "anna@kunde.example"
	contact := seedLinkedContact(t, e, address)
	contactID := ids.From[ids.ContactKind](contact)
	task := logTaskFor(t, e, contact, "Send revised installation plan", new(time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)))
	filed := readTask(t, e, task).OccurredAt
	svc := pageAsOf(e, filed.Add(3*time.Hour))
	before := momentOf(e.Admin(), t, svc, contactID)
	for _, delay := range []time.Duration{time.Hour, 2 * time.Hour} {
		sent := logEmailFor(e.Admin(), t, e, contact, address, "outbound", true, filed.Add(delay))
		got := momentOf(e.Admin(), t, svc, contactID)
		if got.MayBeDone != nil || got.ClaimKey != before.ClaimKey || got.Headline != before.Headline {
			t.Fatalf("an unrelated email changed the task into completion advice: %+v", got)
		}
		for _, evidence := range got.Evidence {
			if evidence.Id != nil && ids.UUID(*evidence.Id) == sent {
				t.Fatal("task evidence cites unrelated correspondence")
			}
		}
	}
	if done := readTask(t, e, task).IsDone; done != nil && *done {
		t.Fatal("the task was completed without its own completion action")
	}
}

func TestAnEmailNeverMakesAColleaguesTaskTheReadersToComplete(t *testing.T) {
	e := integration.Setup(t)
	perms := principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"contact":  {Read: true, Update: true},
			"activity": {Create: true, Read: true, Update: true},
		},
		RowScope: principal.RowScopeAll,
	}
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, perms)
	const address = "carla@kunde.example"
	contact := seedLinkedContact(t, e, address)
	task := logTaskFor(t, e, contact, "Send the pricing sheet", new(time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)))
	filed := readTask(t, e, task).OccurredAt
	sent := logEmailFor(author, t, e, contact, address, "outbound", true, filed.Add(time.Hour))
	if _, err := e.Activities.SetAudience(author, ids.From[ids.ActivityKind](sent), activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting the email to its participants: %v", err)
	}
	svc := pageAsOf(e, filed.Add(3*time.Hour))
	for _, reader := range []context.Context{author, colleague} {
		got := momentOf(reader, t, svc, ids.From[ids.ContactKind](contact))
		if got.MayBeDone != nil {
			t.Fatalf("a later email generated completion advice: %+v", got)
		}
		if got.Headline != "Your team owes them: Send the pricing sheet" {
			t.Fatalf("a reader was assigned the contact owner's task: %s", got.Headline)
		}
	}
}

// pageAsOf is the contact page service reading as of now, so emails dated
// relative to the task's own timestamps have all been sent.
func pageAsOf(e *integration.Env, now time.Time) *contact360.Service {
	return contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), time.Now, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), func() time.Time { return now })
}

func momentOf(as context.Context, t *testing.T, svc *contact360.Service, contactID ids.ContactID) crmcontracts.ContactMoment {
	t.Helper()
	page, err := svc.Assemble(as, contactID)
	if err != nil || page.Moment == nil {
		t.Fatalf("assembling contact360: %v (moment %v)", err, page.Moment)
	}
	return *page.Moment
}

// logEmailFor writes one email linked to the contact through the activity
// writer. attested is the provider's filing of it as sent by us.
func logEmailFor(as context.Context, t *testing.T, e *integration.Env, contact ids.UUID, address, direction string, attested bool, occurred time.Time) ids.UUID {
	t.Helper()
	subject := "Invoice payment instructions"
	row, _, err := e.Activities.LogActivity(as, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &direction, OccurredAt: &occurred, Source: "manual",
		CounterpartyEmail: address, CounterpartyOutboundAttested: attested,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("logging a %s email: %v", direction, err)
	}
	return ids.UUID(row.Id)
}

// readTask reads the task back through the activity store.
func readTask(t *testing.T, e *integration.Env, task ids.UUID) crmcontracts.Activity {
	t.Helper()
	row, err := e.Activities.GetActivity(e.Admin(), ids.From[ids.ActivityKind](task), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the task back: %v", err)
	}
	return row
}
