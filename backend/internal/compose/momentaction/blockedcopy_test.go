// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

import (
	"context"
	"testing"

	contracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestBlockedCopyCatalog(t *testing.T) { langcopytest.Census(t, blockedWords) }

// roleReadsOnly is the sentence a reader whose role only reads gets, per
// language, as the record pages word it.
var roleReadsOnly = map[string]string{
	"en": "Your role can only read. Ask an administrator for another role.",
	"de": "Deine Rolle erlaubt nur das Lesen. Frage einen Admin nach einer anderen Rolle.",
	"vi": "Vai trò của bạn chỉ có quyền xem. Liên hệ quản trị viên để được cấp vai trò khác.",
}

// promised turns the card's recommended action into Done on a task or a claim.
func promised(promise string) contracts.ContactMoment {
	moment := card()
	moment.MayBeDone = &contracts.ContactMomentMayBeDone{PromiseType: contracts.ContactMomentMayBeDonePromiseType(promise)}
	moment.RecommendedAction.Kind = contracts.ContactMomentActionKindCompleteTask
	return moment
}

func TestARoleThatOnlyReadsHearsTheRoleSentenceOnEveryVerb(t *testing.T) {
	booking := card()
	booking.RecommendedAction.Kind = contracts.ContactMomentActionKindScheduleMeeting
	readers := map[string]context.Context{
		"no activity.create": reader(principal.ObjectGrant{Read: true}),
		"a read seat":        seated(principal.SeatRead, principal.ObjectGrant{Read: true, Create: true, Update: true}),
	}
	for language, want := range roleReadsOnly {
		for who, ctx := range readers {
			for name, moment := range map[string]contracts.ContactMoment{
				"log": card(), "book": booking, "task done": promised("task"), "claim done": promised("claim"),
			} {
				Withhold(ctx, language, &moment)
				got := moment.RecommendedAction
				if got.State != contracts.ContactMomentActionStateBlocked || got.BlockedReason == nil || *got.BlockedReason != want {
					t.Errorf("%s, %s, %s: %+v, want blocked saying %q", language, who, name, got, want)
				}
			}
		}
	}
}

func TestAReaderWhoMayWriteActivitiesHearsWhichGrantIsMissing(t *testing.T) {
	logsOnly := reader(principal.ObjectGrant{Read: true, Create: true})
	for _, tc := range []struct{ language, task, claim string }{
		{"en", "You do not have permission to complete tasks", "You do not have permission to settle commitments"},
		{"de", "Sie haben keine Berechtigung, Aufgaben abzuschließen", "Sie haben keine Berechtigung, Zusagen abzuschließen"},
		{"vi", "Bạn không có quyền hoàn thành nhiệm vụ", "Bạn không có quyền hoàn tất cam kết"},
	} {
		for promise, want := range map[string]string{"task": tc.task, "claim": tc.claim} {
			moment := promised(promise)
			Withhold(logsOnly, tc.language, &moment)
			if got := moment.RecommendedAction.BlockedReason; got == nil || *got != want {
				t.Errorf("%s/%s: blocked_reason %v, want %q", tc.language, promise, got, want)
			}
		}
	}
}
