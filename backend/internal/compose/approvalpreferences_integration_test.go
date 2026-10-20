// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestPersonalApprovalMutesAgreeAcrossHTTPToolsAndHomeCounts(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	svc := NewApprovalsService(e.DB())
	for range 3 {
		deal := e.SeedDeal(t, "Review date", pipeline, open, &e.AdminUser)
		proposal, err := json.Marshal(deals.CloseDateCorrection{DealID: ids.From[ids.DealKind](deal), ExpectedCloseDate: "2026-12-01", Basis: "Review the date"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: deals.CloseDateCorrectionKind, TargetType: "deal", TargetID: deal, ProposedChange: proposal, DiffHash: deal.String()}); err != nil {
			t.Fatal(err)
		}
	}
	for _, muted := range []bool{false, true, false} {
		request := httptest.NewRequest(http.MethodPatch, "/me/approval-preferences", strings.NewReader(fmt.Sprintf(`{"kind":"close_date_correction","muted":%t}`, muted))).WithContext(e.Admin())
		recorder := httptest.NewRecorder()
		approvals.NewHandlers(svc).UpdateApprovalPreference(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("set personal preference: %d %s", recorder.Code, recorder.Body.String())
		}
		request = httptest.NewRequest(http.MethodGet, "/me/approval-preferences", nil).WithContext(e.Admin())
		recorder = httptest.NewRecorder()
		approvals.NewHandlers(svc).ListApprovalPreferences(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("read personal preference: %d %s", recorder.Code, recorder.Body.String())
		}
		var preferences crmcontracts.ApprovalPreferences
		if err := json.Unmarshal(recorder.Body.Bytes(), &preferences); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, preference := range preferences.Data {
			if preference.Kind == deals.CloseDateCorrectionKind {
				found = true
				if preference.Muted != muted {
					t.Fatalf("preference=%+v, want muted=%t", preference, muted)
				}
			}
		}
		if !found {
			t.Fatal("personal preference missing from HTTP list")
		}
		want := 3
		if muted {
			want = 0
		}
		count, err := (attentionApprovals{svc: svc}).CountPending(e.Admin(), attention.ApprovalQuery{Status: "pending"})
		if err != nil || count != want {
			t.Fatalf("Home count %d, want %d: %v", count, want, err)
		}
		page, err := (approvalInbox{svc: svc}).ListApprovals(e.Admin(), agents.ApprovalQuery{Limit: 1})
		if err != nil || (len(page.Approvals) > 0) != !muted {
			t.Fatalf("tool queue parity: %v, %v", page, err)
		}
		if !muted && page.NextCursor == "" {
			t.Fatal("tool lost continuation behind first row")
		}
		for _, include := range []bool{false, true} {
			expected := want
			if include {
				expected = 3
			}
			total := 0
			var cursor *string
			for range 4 {
				request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/approvals?include_muted=%t", include), nil).WithContext(e.Admin())
				recorder := httptest.NewRecorder()
				approvals.NewHandlers(svc).ListApprovals(recorder, request, crmcontracts.ListApprovalsParams{IncludeMuted: &include, Limit: new(1), Cursor: cursor})
				if recorder.Code != http.StatusOK {
					t.Fatalf("HTTP status %d: %s", recorder.Code, recorder.Body.String())
				}
				var response crmcontracts.ApprovalListResponse
				if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				total += len(response.Data)
				if !response.Page.HasMore {
					break
				}
				if response.Page.NextCursor == nil || *response.Page.NextCursor == "" {
					t.Fatal("HTTP continuation is missing")
				}
				cursor = response.Page.NextCursor
			}
			if total != expected {
				t.Fatalf("HTTP paged count %d, want %d", total, expected)
			}
		}
	}
}
