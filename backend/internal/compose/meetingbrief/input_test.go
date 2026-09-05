// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// WithCounterpart folds the lead attendee's own 360 read into the brief's
// input. A row outside this caller's audience must never reach Recent at
// all: Recent is marshalled straight into the model's prompt, and a citation
// to a withheld conversation sends the reader to a record they cannot open,
// so the one place this is enforced is here, at the fold, rather than
// trusted downstream in every section that reads Recent.

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestWithCounterpartExcludesAWithheldActivity(t *testing.T) {
	withheld := crmcontracts.ActivityContentStateWithheld
	available := crmcontracts.ActivityContentStateAvailable
	view := crmcontracts.Person360{
		Activities: &struct {
			Data []crmcontracts.Activity `json:"data"`
			Page crmcontracts.PageInfo   `json:"page"`
		}{
			Data: []crmcontracts.Activity{
				{
					Id:           openapi_types.UUID(ids.NewV7()),
					Kind:         crmcontracts.ActivityKindEmail,
					OccurredAt:   at(9),
					ContentState: &withheld,
				},
				{
					Id:           openapi_types.UUID(ids.NewV7()),
					Kind:         crmcontracts.ActivityKindEmail,
					OccurredAt:   at(7),
					Subject:      ptr("Contract redlines"),
					ContentState: &available,
				},
				// content_state carries only two known values today, but the
				// exclusion is spelled as an allow-list precisely so a third
				// one nobody has written a case for yet is excluded too — this
				// row carries no content_state at all, the shape a future
				// unnamed state (or a stale caller) would also produce.
				{
					Id:         openapi_types.UUID(ids.NewV7()),
					Kind:       crmcontracts.ActivityKindEmail,
					OccurredAt: at(6),
					Subject:    ptr("No content_state at all"),
				},
			},
		},
	}
	in := &Input{}
	WithCounterpart(in, view)
	if len(in.Recent) != 1 {
		t.Fatalf("recent = %d, want 1 — only the row explicitly marked available may reach Recent", len(in.Recent))
	}
	if in.Recent[0].Subject != "Contract redlines" {
		t.Errorf("recent[0] = %+v, want the readable conversation, not the withheld or unmarked one", in.Recent[0])
	}
}

// A withheld row must not cost a readable one its slot: recentCap bounds what
// the reader actually sees, and a withheld row contributes nothing to see. A
// count-only check would pass even if the withheld row displaced the LAST
// readable one rather than being skipped outright, so this also names which
// one made it in.
func TestAWithheldActivityDoesNotSpendARecentSlot(t *testing.T) {
	data := make([]crmcontracts.Activity, 0, recentCap+1)
	withheld := crmcontracts.ActivityContentStateWithheld
	available := crmcontracts.ActivityContentStateAvailable
	data = append(data, crmcontracts.Activity{
		Id: openapi_types.UUID(ids.NewV7()), Kind: crmcontracts.ActivityKindEmail,
		OccurredAt: at(20), ContentState: &withheld,
	})
	lastReadableID := openapi_types.UUID(ids.NewV7())
	for i := range recentCap {
		id := openapi_types.UUID(ids.NewV7())
		if i == recentCap-1 {
			id = lastReadableID
		}
		data = append(data, crmcontracts.Activity{
			Id: id, Kind: crmcontracts.ActivityKindEmail, OccurredAt: at(10 - i),
			ContentState: &available,
		})
	}
	view := crmcontracts.Person360{
		Activities: &struct {
			Data []crmcontracts.Activity `json:"data"`
			Page crmcontracts.PageInfo   `json:"page"`
		}{Data: data},
	}
	in := &Input{}
	WithCounterpart(in, view)
	if len(in.Recent) != recentCap {
		t.Fatalf("recent = %d, want %d readable conversations, none of them spent on the withheld one", len(in.Recent), recentCap)
	}
	if got, want := in.Recent[recentCap-1].ID, ids.UUID(lastReadableID).String(); got != want {
		t.Errorf("last recent id = %s, want %s — the withheld row displaced the last readable one instead of being skipped", got, want)
	}
}
