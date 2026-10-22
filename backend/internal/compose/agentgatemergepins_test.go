// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type mergePinningApprovals struct {
	pinningApprovals
	pins auth.ReleasedRecordPins
}

func (p mergePinningApprovals) RedeemRecords(context.Context, ids.ApprovalID, string, string) (auth.ReleasedRecordPins, error) {
	return p.pins, nil
}

func TestMergeRedemptionKeepsSourceAndSurvivorPinsDistinct(t *testing.T) {
	for _, supplied := range []string{"", "3"} {
		t.Run("source_if_match_"+supplied, func(t *testing.T) {
			source, survivor := ids.NewV7(), ids.NewV7()
			pins := auth.ReleasedRecordPins{
				Target:   &auth.ReleasedRecordPin{EntityType: "contact", EntityID: survivor, Version: 9},
				CoTarget: &auth.ReleasedRecordPin{EntityType: "contact", EntityID: source, Version: 3},
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/contacts/"+source.String()+"/merge", nil)
			req.Header.Set(approvalTokenHeader, ids.New[ids.ApprovalKind]().String())
			if supplied != "" {
				req.Header.Set(ifMatchHeader, supplied)
			}
			called := false
			next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				called = true
				if got := r.Header.Get(ifMatchHeader); got != supplied {
					t.Fatalf("source If-Match=%q want=%q", got, supplied)
				}
				for _, pin := range []*auth.ReleasedRecordPin{pins.Target, pins.CoTarget} {
					got, ok := auth.ReleasedRecordVersion(r.Context(), pin.EntityType, pin.EntityID)
					if !ok || got != pin.Version {
						t.Fatalf("released record=%s version=%d pinned=%t", pin.EntityID, got, ok)
					}
				}
			})
			policy := agentPolicy{Op: "mergeContact", Access: accessTool, Tool: "merge_records", RecordType: recordTypeContact}
			handled, ran := redeemIfPresented(httptest.NewRecorder(), req, next, mergePinningApprovals{pins: pins}, policy, []byte(`{"target_id":"`+survivor.String()+`"}`))
			if !handled || !ran || !called {
				t.Fatalf("handled=%t ran=%t called=%t", handled, ran, called)
			}
		})
	}
}
