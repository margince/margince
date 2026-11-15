// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"slices"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEverySeatReachSkipsPagesWithoutTeamBadges(t *testing.T) {
	statuses := []crmcontracts.EmailAccessStatus{
		crmcontracts.EmailAccessStatusParticipants,
		crmcontracts.EmailAccessStatusSelected,
		crmcontracts.EmailAccessStatusWithheld,
		crmcontracts.EmailAccessStatusWorkspace,
	}
	page := []crmcontracts.Activity{{}}
	for _, status := range statuses {
		page = append(page, crmcontracts.Activity{EmailSummary: &crmcontracts.EmailSummary{DisplayStatus: status}})
	}
	for _, rows := range [][]crmcontracts.Activity{nil, {}, page} {
		if err := withEverySeatReach(context.Background(), nil, rows); err != nil {
			t.Fatal(err)
		}
	}
	for i, status := range statuses {
		if got := page[i+1].EmailSummary.DisplayStatus; got != status {
			t.Fatalf("badge=%s, want %s", got, status)
		}
	}
}

func TestEverySeatReachAsksOnlyAboutTheTeamRowsOfAMixedPage(t *testing.T) {
	statuses := []crmcontracts.EmailAccessStatus{
		crmcontracts.EmailAccessStatusTeam,
		crmcontracts.EmailAccessStatusParticipants,
		crmcontracts.EmailAccessStatusSelected,
		crmcontracts.EmailAccessStatusTeam,
		crmcontracts.EmailAccessStatusWithheld,
		crmcontracts.EmailAccessStatusWorkspace,
	}
	page := []crmcontracts.Activity{{Id: openapi_types.UUID(ids.NewV7())}}
	var want []ids.UUID
	for _, status := range statuses {
		id := ids.NewV7()
		page = append(page, crmcontracts.Activity{
			Id:           openapi_types.UUID(id),
			EmailSummary: &crmcontracts.EmailSummary{ActivityId: openapi_types.UUID(id), DisplayStatus: status},
		})
		if status == crmcontracts.EmailAccessStatusTeam {
			want = append(want, id)
		}
	}
	rows := teamBadgeRows(page)
	var got []ids.UUID
	for _, row := range rows {
		got = append(got, row.id)
		if ids.UUID(row.summary.ActivityId) != row.id {
			t.Fatalf("row %s pairs the badge of %s", row.id, row.summary.ActivityId)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("asked about %v, want only the Team rows %v", got, want)
	}
}
