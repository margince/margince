// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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
