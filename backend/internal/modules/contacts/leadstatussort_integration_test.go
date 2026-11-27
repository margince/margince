// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"slices"
	"testing"
)

// Sorting the lead list by status follows the ladder a lead climbs, not the
// alphabet of its keys, on every page the cursor walks.
func TestTheLeadListSortsStatusInLadderOrder(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	ctx := asStageAdmin(e, stageAdminGrants())
	for _, status := range []LeadStatus{LeadStatusEngaged, LeadStatusContacted, LeadStatusNew} {
		name := "Ladder " + string(status)
		if _, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: &name, Source: "manual", Status: string(status)}); err != nil {
			t.Fatalf("creating the %s lead: %v", status, err)
		}
	}

	for _, tc := range []struct {
		sort string
		want []string
	}{
		{"status", []string{"Ladder new", "Ladder contacted", "Ladder engaged"}},
		{"-status", []string{"Ladder engaged", "Ladder contacted", "Ladder new"}},
	} {
		t.Run(tc.sort, func(t *testing.T) {
			sort, limit := tc.sort, 1
			var got []string
			var cursor *string
			for range 4 {
				leads, page, err := e.store.ListLeads(ctx, ListLeadsInput{Sort: &sort, Limit: &limit, Cursor: cursor})
				if err != nil {
					t.Fatalf("listing by %s: %v", sort, err)
				}
				for _, l := range leads {
					got = append(got, *l.FullName)
				}
				if !page.HasMore {
					break
				}
				cursor = &page.NextCursor
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("sorted by %s: %v, want %v", sort, got, tc.want)
			}
		})
	}
}
