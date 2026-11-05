// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

import (
	"testing"

	contracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestBlockedCopyCatalog(t *testing.T) { langcopytest.Census(t, blockedWords) }

func TestPermissionReasonsFollowCardLanguage(t *testing.T) {
	for _, tc := range []struct{ language, log, task, claim string }{
		{"en", "You do not have permission to log activities", "You do not have permission to complete tasks", "You do not have permission to settle commitments"},
		{"de", "Sie haben keine Berechtigung, Aktivitäten zu erfassen", "Sie haben keine Berechtigung, Aufgaben abzuschließen", "Sie haben keine Berechtigung, Zusagen abzuschließen"},
		{"vi", "Bạn không có quyền ghi lại hoạt động", "Bạn không có quyền hoàn thành nhiệm vụ", "Bạn không có quyền hoàn tất cam kết"},
	} {
		for _, promise := range []string{"", "task", "claim"} {
			moment := card()
			want := tc.log
			if promise != "" {
				moment.MayBeDone = &contracts.ContactMomentMayBeDone{PromiseType: contracts.ContactMomentMayBeDonePromiseType(promise)}
				moment.RecommendedAction.Kind = contracts.ContactMomentActionKindCompleteTask
				want = tc.task
				if promise == "claim" {
					want = tc.claim
				}
			}
			Withhold(reader(principal.ObjectGrant{Read: true}), tc.language, &moment)
			got := moment.RecommendedAction
			if got.State != contracts.ContactMomentActionStateBlocked || got.Destination != nil || got.BlockedReason == nil || *got.BlockedReason != want {
				t.Fatalf("%s/%s: %+v", tc.language, promise, got)
			}
		}
	}
}
