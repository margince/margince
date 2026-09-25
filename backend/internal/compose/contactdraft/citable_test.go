// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft

import (
	"encoding/json"
	"regexp"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

var summaryID = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// fullView is a 360 with every section the fold reads, so the census below
// sees every id the fold could carry.
func fullView() (crmcontracts.Contact360, ids.ProjectID) {
	project := ids.New[ids.ProjectKind]()
	inbound := crmcontracts.ActivityDirectionInbound
	activities := []crmcontracts.Activity{{
		Id: openapi_types.UUID(ids.NewV7()), Kind: crmcontracts.ActivityKindEmail,
		OccurredAt: draftedAt.Add(-48 * time.Hour), Direction: &inbound,
		Subject: strPtr("Pricing question"), Body: strPtr("Could you send the pricing?"),
	}}
	claims := []crmcontracts.ConversationClaim{
		claim(crmcontracts.ConversationClaimKindOpenQuestion, "the pricing", crmcontracts.ConversationClaimStatusOpen, nil),
	}
	projects := []crmcontracts.Company360Project{{ProjectId: openapi_types.UUID(project.UUID), Name: "Rollout"}}
	view := crmcontracts.Contact360{
		Claims:   &claims,
		Projects: &projects,
		Commercial: &crmcontracts.Contact360Commercial{Deal: &crmcontracts.Contact360CommercialDeal{
			DealId: openapi_types.UUID(ids.NewV7()), Title: "Rollout licence",
		}},
		NextMeeting: &crmcontracts.Contact360NextMeeting{
			StartsAt: draftedAt.Add(72 * time.Hour), Subject: strPtr("Review"), Participants: participants(recipientID),
		},
	}
	view.Activities = &struct {
		Data []crmcontracts.Activity `json:"data"`
		Page crmcontracts.PageInfo   `json:"page"`
	}{Data: activities}
	view.Contact.Id = recipientID
	view.Contact.FullName = "Priya Raman"
	return view, project
}

// Every id the summary shows the model is one a reason may cite. An id the
// filter refuses is an invitation to a citation that is silently dropped, so
// the census reads the encoded payload rather than a list of its fields.
func TestEveryIDTheSummaryCarriesIsCitable(t *testing.T) {
	view, project := fullView()
	in := FromView(view, Request{Envelope: envelopeAt(textlang.English, convstate.BandFresh), ProjectID: &project})
	if in.Project == nil || in.Deal == nil || len(in.Claims) == 0 || len(in.Recent) == 0 || in.Meeting == nil {
		t.Fatalf("the fixture must fold every section, got %+v", in)
	}
	payload, err := json.Marshal(in.Fenced())
	if err != nil {
		t.Fatalf("encoding the summary: %v", err)
	}
	citable := knownRecords(in)
	found := summaryID.FindAllString(string(payload), -1)
	if len(found) == 0 {
		t.Fatal("the summary carries no id at all, so this census reads nothing")
	}
	for _, id := range found {
		if _, held := citable[id]; !held {
			t.Errorf("the summary carries %s, which no citation the filter accepts can name", id)
		}
	}
}
