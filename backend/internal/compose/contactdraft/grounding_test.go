// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// A draft names every record its input was folded from: the recipient, the
// deal, the project, each exchange, the conversation each claim was read from
// and the next meeting, so a kept draft can re-prove each one.
func TestTheGroundingNamesEveryRecordTheInputHolds(t *testing.T) {
	deal, project, meeting := ids.NewV7(), ids.NewV7(), ids.NewV7()
	exchange := activity(true, "Scope", new("Can you send the scope?"))
	said := claim(crmcontracts.ConversationClaimKindOpenQuestion, "send the scope", crmcontracts.ConversationClaimStatusOpen, nil)
	claims := []crmcontracts.ConversationClaim{said}
	view := crmcontracts.Contact360{
		Commercial: &crmcontracts.Contact360Commercial{Deal: &crmcontracts.Contact360CommercialDeal{
			DealId: openapi_types.UUID(deal), Title: "Rollout",
		}},
		Projects: &[]crmcontracts.Company360Project{{ProjectId: openapi_types.UUID(project), Name: "ERP"}},
		Claims:   &claims,
		NextMeeting: &crmcontracts.Contact360NextMeeting{
			ActivityId: openapi_types.UUID(meeting), StartsAt: draftedAt.Add(72 * time.Hour),
			Participants: participants(recipientID),
		},
	}
	view.Contact.Id = recipientID
	view.Activities = &struct {
		Data []crmcontracts.Activity `json:"data"`
		Page crmcontracts.PageInfo   `json:"page"`
	}{Data: []crmcontracts.Activity{exchange}}
	scoped := ids.From[ids.ProjectKind](project)

	in := FromView(view, Request{ProjectID: &scoped, Envelope: envelopeAt(textlang.English, convstate.BandWeeks)})
	got, err := in.Grounding(datasource.EntityContact)
	if err != nil {
		t.Fatalf("grounding: %v", err)
	}

	want := []datasource.EntityRef{
		{Type: datasource.EntityContact, ID: ids.UUID(recipientID)},
		{Type: datasource.EntityDeal, ID: deal},
		{Type: datasource.EntityProject, ID: project},
		{Type: datasource.EntityActivity, ID: ids.UUID(exchange.Id)},
		{Type: datasource.EntityActivity, ID: ids.UUID(said.SourceActivityId)},
		{Type: datasource.EntityActivity, ID: meeting},
	}
	if !slices.Equal(got, want) {
		t.Errorf("grounding = %v\nwant       %v", got, want)
	}
}

// The ids that name a record for grounding never reach the model: a project
// and a meeting are not records a reason may cite.
func TestTheGroundingIdsStayOutOfThePrompt(t *testing.T) {
	in := Input{
		Project: &ProjectIn{ID: ids.NewV7().String(), Name: "ERP"},
		Meeting: &MeetingIn{ActivityID: ids.NewV7().String(), StartsAt: draftedAt.Format(time.RFC3339)},
	}
	prompt, err := json.Marshal(in.Fenced())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, id := range []string{in.Project.ID, in.Meeting.ActivityID} {
		if strings.Contains(string(prompt), id) {
			t.Errorf("the prompt carries grounding id %s: %s", id, prompt)
		}
	}
}
