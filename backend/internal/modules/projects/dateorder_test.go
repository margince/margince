// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"errors"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestAPatchIsHeldToTheDatePairItLeavesOnTheRow(t *testing.T) {
	day := func(s string) *time.Time {
		parsed, err := time.Parse(time.DateOnly, s)
		if err != nil {
			t.Fatal(err)
		}
		return &parsed
	}
	stored := crmcontracts.Project{
		StartedAt:     &openapi_types.Date{Time: *day("2026-03-01")},
		TargetEndDate: &openapi_types.Date{Time: *day("2026-06-30")},
	}
	reversed := crmcontracts.Project{
		StartedAt:     &openapi_types.Date{Time: *day("2026-06-30")},
		TargetEndDate: &openapi_types.Date{Time: *day("2026-03-01")},
	}
	for name, tc := range map[string]struct {
		current crmcontracts.Project
		in      UpdateProjectInput
		field   string
	}{
		"a target end before the stored start": {stored, UpdateProjectInput{TargetEndDate: day("2026-02-28")}, "target_end_date"},
		"a start after the stored target end":  {stored, UpdateProjectInput{StartedAt: day("2026-07-01")}, "started_at"},
		"both sent, reversed":                  {stored, UpdateProjectInput{StartedAt: day("2026-05-01"), TargetEndDate: day("2026-04-01")}, "target_end_date"},
		"the same day on both ends":            {stored, UpdateProjectInput{TargetEndDate: day("2026-03-01")}, ""},
		"the start cleared":                    {stored, UpdateProjectInput{Clear: []string{"started_at"}, TargetEndDate: day("2026-01-01")}, ""},
		"neither date moved on a reversed row": {reversed, UpdateProjectInput{Name: new("Renamed")}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			err := refuseMovedTargetEnd(tc.current, tc.in)
			var refusal *ProjectDateRangeError
			switch {
			case tc.field == "" && err != nil:
				t.Fatalf("refused %v, want accepted", err)
			case tc.field != "" && (!errors.As(err, &refusal) || refusal.Field != tc.field):
				t.Fatalf("answered %v, want a date range refusal on %s", err, tc.field)
			}
		})
	}
}
