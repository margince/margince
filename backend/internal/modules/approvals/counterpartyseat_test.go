// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCounterpartyReviewRequiresItsRecordedImportOwner(t *testing.T) {
	owner := ids.MustParse("01a05500-0000-7000-8000-000000000001")
	other := ids.MustParse("01a05500-0000-7000-8000-000000000002")
	for _, tc := range []struct {
		name, proposal string
		reader         ids.UUID
		withheld       bool
	}{
		{"importer", `{"owner_id":"` + owner.String() + `"}`, owner, false},
		{"other admin", `{"owner_id":"` + owner.String() + `"}`, other, true},
		{"no owner", `{}`, owner, true},
		{"null owner", `{"owner_id":null}`, owner, true},
		{"zero owner", `{"owner_id":"00000000-0000-0000-0000-000000000000"}`, owner, true},
		{"invalid owner", `{"owner_id":"not-a-user"}`, owner, true},
		{"invalid proposal", `{`, owner, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proposal := row{Kind: "capture_counterparty", ProposedChange: []byte(tc.proposal)}
			if got := withheldFromOtherSeats(principal.Principal{UserID: tc.reader}, proposal); got != tc.withheld {
				t.Fatalf("withheld = %v, want %v", got, tc.withheld)
			}
		})
	}
}
