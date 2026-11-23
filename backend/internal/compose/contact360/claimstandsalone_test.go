// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"encoding/json"
	"os"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// claimStandsAloneCase is one row of the table contactpromises.test.ts reads
// too, so the server ranking and the page count keep one rule.
type claimStandsAloneCase struct {
	Name        string   `json:"name"`
	Task        *string  `json:"task"`
	TaskOpen    *bool    `json:"task_open"`
	Listed      []string `json:"listed"`
	StandsAlone bool     `json:"stands_alone"`
}

func TestClaimStandsAloneFollowsTheSharedTable(t *testing.T) {
	raw, err := os.ReadFile("testdata/claimstandsalone.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []claimStandsAloneCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("the shared table holds no case")
	}
	// The table names tasks by short labels; each label maps to one uuid.
	uuidOf := map[string]openapi_types.UUID{}
	id := func(label string) openapi_types.UUID {
		if _, ok := uuidOf[label]; !ok {
			uuidOf[label] = openapi_types.UUID{byte(len(uuidOf) + 1)}
		}
		return uuidOf[label]
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			claim := crmcontracts.ConversationClaim{TaskOpen: tc.TaskOpen}
			if tc.Task != nil {
				claim.TaskActivityId = new(id(*tc.Task))
			}
			listed := map[openapi_types.UUID]bool{}
			for _, label := range tc.Listed {
				listed[id(label)] = true
			}
			if got := claimStandsAlone(claim, listed); got != tc.StandsAlone {
				t.Errorf("claimStandsAlone = %v, want %v", got, tc.StandsAlone)
			}
		})
	}
}
