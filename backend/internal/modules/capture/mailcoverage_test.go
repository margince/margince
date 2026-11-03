// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestMailCoverageNeverTreatsASyncOrImportAsACompleteInterval(t *testing.T) {
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	from, good := at.AddDate(0, -6, 0), at.Add(-time.Minute)
	mail := ConnectionView{Provider: "mail", Status: statusConnected, LastSuccessAt: &good}
	for _, tc := range []struct {
		name    string
		views   []ConnectionView
		state   crmcontracts.MailCaptureCoverageState
		success bool
	}{
		{"no mailbox", nil, crmcontracts.MailCaptureCoverageStateUnknown, false},
		{"calendar only", []ConnectionView{{Provider: "calendar", Status: statusConnected, LastSuccessAt: &good}}, crmcontracts.MailCaptureCoverageStateUnknown, false},
		{"successful mail sync", []ConnectionView{mail}, crmcontracts.MailCaptureCoverageStatePartial, true},
		{"completed history", []ConnectionView{{Provider: "mail", Status: statusConnected, LastSuccessAt: &good, Backfill: &BackfillRun{Status: "done"}}}, crmcontracts.MailCaptureCoverageStatePartial, true},
		{"capped history", []ConnectionView{{Provider: "mail", Status: statusConnected, LastSuccessAt: &good, Backfill: &BackfillRun{Status: "done", Failed: 1, EstimateIsFloor: true}}}, crmcontracts.MailCaptureCoverageStatePartial, true},
		{"a sibling never synced", []ConnectionView{mail, {Provider: "other_mail", Status: statusConnected}}, crmcontracts.MailCaptureCoverageStateUnknown, false},
		{"a sibling disconnected", []ConnectionView{mail, {Provider: "other_mail", Status: statusDisconnected}}, crmcontracts.MailCaptureCoverageStateUnavailable, false},
		{"a new failed attempt", []ConnectionView{{Provider: "mail", Status: statusConnected, LastSuccessAt: &good, LastSyncedAt: &at, LastErrorClass: new("unreachable")}}, crmcontracts.MailCaptureCoverageStateUnavailable, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mailCoverageOf(tc.views, []string{"mail", "other_mail"}, &from, at)
			if got.State != tc.state || (got.LastSuccessAt != nil) != tc.success || got.IntervalFrom == nil || !got.IntervalFrom.Equal(from) || !got.IntervalTo.Equal(at) {
				t.Fatalf("coverage = %+v", got)
			}
			if got.LastSuccessAt != nil && !got.LastSuccessAt.Equal(good) {
				t.Fatalf("last success = %s, want %s", got.LastSuccessAt, good)
			}
		})
	}
}

func TestMailCoverageRefusesAnUnattributedReaderBeforeReadingConnections(t *testing.T) {
	registry := NewRegistry(nil, nil, nil, nil)
	if _, err := registry.MailCoverage(context.Background(), []string{"mail"}, nil, time.Time{}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("unattributed coverage read = %v", err)
	}
}

func TestADelegatedUserIDDoesNotGrantMailCoverage(t *testing.T) {
	registry := NewRegistry(nil, nil, nil, nil)
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalAgent, UserID: ids.NewV7()})
	if _, err := registry.MailCoverage(ctx, []string{"mail"}, nil, time.Time{}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("delegated coverage read = %v", err)
	}
}
