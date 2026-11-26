// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"slices"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
)

// A claim that names a task is owed in its own name only when the task is
// open and off the capped page. A task on the page stands for it, and a task
// that is done or hidden leaves nothing owed.
func TestAClaimIsOwedThroughItsTaskOrByItself(t *testing.T) {
	listed := openapi_types.UUID(ids.NewV7())
	claim := func(body string, task *openapi_types.UUID, open *bool) crmcontracts.ConversationClaim {
		return crmcontracts.ConversationClaim{
			Id: openapi_types.UUID(ids.NewV7()), Kind: crmcontracts.ConversationClaimKindCommitmentOurs,
			Status: crmcontracts.ConversationClaimStatusOpen, Body: body, TaskActivityId: task, TaskOpen: open,
		}
	}
	page := &crmcontracts.Contact360{
		NextSteps: &struct {
			Data []crmcontracts.Activity `json:"data"`
			Page crmcontracts.PageInfo   `json:"page"`
		}{Data: []crmcontracts.Activity{{Id: listed, Subject: new("Send the quote")}}, Page: crmcontracts.PageInfo{HasMore: true}},
		Claims: &[]crmcontracts.ConversationClaim{
			claim("no task", nil, nil),
			claim("task on the page", &listed, new(true)),
			claim("task past the cap", new(openapi_types.UUID(ids.NewV7())), new(true)),
			claim("task done or hidden", new(openapi_types.UUID(ids.NewV7())), new(false)),
			claim("task not asked about", new(openapi_types.UUID(ids.NewV7())), nil),
		},
	}

	var got []string
	for _, item := range owedPromises(page, nil) {
		if c, ok := item.Ref.(crmcontracts.ConversationClaim); ok && item.Source == owedwork.FromClaim {
			got = append(got, c.Body)
		}
	}

	if want := []string{"no task", "task past the cap"}; !slices.Equal(got, want) {
		t.Errorf("owed claims = %q, want %q", got, want)
	}
}

// The contact page ranks a task with only a date by its picked date on the
// company's calendar. The instant that date resolves to elsewhere does not rank it.
func TestADateOnlyTaskOnTheContactPageIsLateByItsPickedDate(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, 11, 17, 7, 59, 59, 0, time.UTC)
	page := &crmcontracts.Contact360{NextSteps: &struct {
		Data []crmcontracts.Activity `json:"data"`
		Page crmcontracts.PageInfo   `json:"page"`
	}{Data: []crmcontracts.Activity{{
		Id: openapi_types.UUID(ids.NewV7()), Subject: new("Send the contract"), DueAt: &due, DueHasTime: new(false),
		DueIntent: &crmcontracts.IntentTime{WallClock: "2026-11-16T23:59:59", Timezone: "America/Los_Angeles"},
	}}}}
	now := time.Date(2026, 11, 17, 7, 0, 0, 0, time.UTC)
	if _, late := owedwork.MostRecentlySlipped(owedPromises(page, berlin), now); !late {
		t.Error("a task picked for the 16th is not late at 08:00 Berlin on the 17th")
	}
}
