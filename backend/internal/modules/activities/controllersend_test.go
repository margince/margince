// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOnlyARelaySendIsMarkedAsOne(t *testing.T) {
	relay, own, withheld := ids.NewV7(), ids.NewV7(), ids.NewV7()
	from := "notices@crm.example.test"
	page := []crmcontracts.Activity{
		{Id: openapi_types.UUID(relay), EmailSummary: &crmcontracts.EmailSummary{}},
		{Id: openapi_types.UUID(own), EmailSummary: &crmcontracts.EmailSummary{}},
		{Id: openapi_types.UUID(withheld), EmailSummary: &crmcontracts.EmailSummary{}},
	}
	applyEmailStates(page, map[ids.UUID]EmailRowState{
		relay: {Delivery: DeliveryState{Status: "sent", Controller: true, FromAddress: &from}},
		own:   {Delivery: DeliveryState{Status: "sent"}},
	})

	if mark := page[0].ControllerSend; mark == nil || mark.FromAddress == nil || *mark.FromAddress != from {
		t.Errorf("the relay send reads %+v, want controller_send from %q", mark, from)
	}
	if page[1].ControllerSend != nil {
		t.Error("the rep's own send is marked as the relay's")
	}
	if page[2].ControllerSend != nil {
		t.Error("a row whose delivery this reader may not see is marked anyway")
	}
}

func TestARelaySendWhoseRelayNamesNoFromKeepsItsLane(t *testing.T) {
	mark := ControllerSendOf(true, nil)
	if mark == nil || mark.FromAddress != nil {
		t.Errorf("a relay send with no recorded From reads %+v, want the lane with a null from_address", mark)
	}
}
