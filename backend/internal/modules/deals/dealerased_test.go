// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"encoding/json"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEraseAndMergeEventsCarryReferencesOnly(t *testing.T) {
	from, into := ids.NewV7(), ids.NewV7()
	cases := []struct {
		name    string
		payload events.Payload
		want    string
	}{
		{"deal.erased", crmcontracts.PublicEventDealErased{}, `{}`},
		{"company.erased", crmcontracts.PublicEventCompanyErased{}, `{}`},
		{"deal.merged", crmcontracts.PublicEventDealMerged{
			MergedFromId: openapi_types.UUID(from), MergedIntoId: openapi_types.UUID(into),
		}, `{"merged_from_id":"` + from.String() + `","merged_into_id":"` + into.String() + `"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.payload.EventType(); got != tc.name {
				t.Fatalf("event type %q, want %q", got, tc.name)
			}
			if _, err := events.StreamFor(tc.name); err != nil {
				t.Fatalf("%s is not in the event catalog: %v", tc.name, err)
			}
			raw, err := json.Marshal(tc.payload)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != tc.want {
				t.Fatalf("payload %s, want %s", raw, tc.want)
			}
		})
	}
}
