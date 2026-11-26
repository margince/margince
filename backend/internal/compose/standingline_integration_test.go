// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/standingline"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// scriptedLine answers every check with one line citing the newest activity,
// which the request shows as a1, and counts the calls.
type scriptedLine struct {
	text  string
	calls int
	saw   string
}

func (s *scriptedLine) Complete(_ context.Context, req model.Request) (model.Response, error) {
	s.calls++
	s.saw = req.Messages[0].Content
	return model.Response{Text: `{"keep_previous":false,"sentences":[{"text":"` + s.text +
		`","evidence":[{"entity_type":"activity","entity_id":"a1"}]}]}`}, nil
}

// A mail on a contact writes the line the contact page shows. The same input
// is not asked about twice, and a reader who may not open the mail gets none.
func TestAMailOnAContactWritesTheLineItsPageShows(t *testing.T) {
	e := integration.Setup(t)
	const address = "anna@kunde.example"
	contact := seedLinkedContact(t, e, address)
	now := clocktest.Now(t).UTC()
	subject, body, direction := "Re: Rollout plan", "The plan works for us. Please send the contract draft by Friday.", "inbound"
	mail, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &now,
		Source: "manual", CounterpartyEmail: address,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	lane := &scriptedLine{text: "Anna confirmed the rollout plan and asked for the contract draft by Friday."}
	refresher := standingline.NewRefresher(e.Pool, activities.NewStore(InstallationDB(e.Pool)), lane,
		func() string { return "test" }, func() time.Time { return now })
	system := principal.SystemActing(e.Admin(), standingLineActor)

	for range 2 {
		if err := refresher.Refresh(system, ids.From[ids.ActivityKind](ids.UUID(mail.Id))); err != nil {
			t.Fatal(err)
		}
	}
	if lane.calls != 1 {
		t.Errorf("the model was asked %d times about one unchanged input, want once", lane.calls)
	}
	if !strings.Contains(lane.saw, "contract draft") {
		t.Errorf("the request never showed the mail's words: %s", lane.saw)
	}

	page, err := pageAsOf(e, now).Assemble(e.Admin(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatal(err)
	}
	if page.StandingLine == nil || page.StandingLine.Text != lane.text ||
		len(page.StandingLine.ActivityIds) != 1 || ids.UUID(page.StandingLine.ActivityIds[0]) != ids.UUID(mail.Id) {
		t.Fatalf("the page's line is %+v, want the written line citing the mail", page.StandingLine)
	}

	contactOnly := e.As(e.Rep3, []ids.UUID{e.Team2}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  map[string]principal.ObjectGrant{"contact": {Read: true}},
		RowScope: principal.RowScopeAll,
	})
	blind, err := pageAsOf(e, now).Assemble(contactOnly, ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatal(err)
	}
	if blind.StandingLine != nil {
		t.Errorf("a reader who may not open the mail was served %q", blind.StandingLine.Text)
	}
}
