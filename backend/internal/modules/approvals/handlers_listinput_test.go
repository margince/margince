// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestTheInboxClampsAnOutOfRangeLimitLikeEveryOtherList(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit *int
		want  int
	}{
		{"absent", nil, 50},
		{"zero", new(0), 1},
		{"negative", new(-1), 1},
		{"one", new(1), 1},
		{"in range", new(25), 25},
		{"max", new(200), 200},
		{"over", new(201), 200},
		{"far over", new(1000), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := crmcontracts.ListApprovalsParams{}
			if tc.limit != nil {
				l := crmcontracts.Limit(*tc.limit)
				params.Limit = &l
			}
			in, err := listInput(params)
			if err != nil {
				t.Fatalf("listInput: %v", err)
			}
			if in.Limit != tc.want {
				t.Fatalf("limit %v bound as %d, want %d", tc.limit, in.Limit, tc.want)
			}
		})
	}
}
