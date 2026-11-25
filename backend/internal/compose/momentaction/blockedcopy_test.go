// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

import (
	"context"
	"testing"

	contracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestBlockedCopyCatalog(t *testing.T) { langcopytest.Census(t, blockedWords) }

var spoken = []string{"en", "de", "vi"}

// promised turns the card's recommended action into Done on a task or a claim.
func promised(promise string) contracts.ContactMoment {
	moment := card()
	moment.MayBeDone = &contracts.ContactMomentMayBeDone{PromiseType: contracts.ContactMomentMayBeDonePromiseType(promise)}
	moment.RecommendedAction.Kind = contracts.ContactMomentActionKindCompleteTask
	return moment
}

func booked() contracts.ContactMoment {
	booking := card()
	booking.RecommendedAction.Kind = contracts.ContactMomentActionKindScheduleMeeting
	return booking
}

func TestTheSeatAndARoleThatOnlyReadsEachHaveTheirSentenceOnEveryVerb(t *testing.T) {
	for _, reader := range []struct {
		who  string
		ctx  context.Context
		want langcopy.Phrase
	}{
		{"a read seat whose role writes", seated(principal.SeatRead, principal.ObjectGrant{Read: true, Create: true, Update: true}), SeatReadsOnly()},
		{"a read seat whose role only reads", seated(principal.SeatRead, principal.ObjectGrant{Read: true}), SeatReadsOnly()},
		{"a role that only reads", reader(principal.ObjectGrant{Read: true}), RoleReadsOnly()},
	} {
		for _, language := range spoken {
			want := langcopy.For(language).Say(reader.want)
			for name, moment := range map[string]contracts.ContactMoment{
				"log": card(), "book": booked(), "task done": promised("task"), "claim done": promised("claim"),
			} {
				Withhold(reader.ctx, language, "contact", &moment)
				got := moment.RecommendedAction
				if got.State != contracts.ContactMomentActionStateBlocked || got.BlockedReason == nil || *got.BlockedReason != want {
					t.Errorf("%s, %s, %s: %+v, want blocked saying %q", language, reader.who, name, got, want)
				}
			}
		}
	}
}

func TestARoleThatUpdatesTheRecordButWritesNoActivityHearsTheActionSentence(t *testing.T) {
	for _, language := range spoken {
		want := langcopy.For(language).Say(ActionRefused())
		for name, moment := range map[string]contracts.ContactMoment{"log": card(), "book": booked()} {
			for _, record := range []string{"contact", "company"} {
				Withhold(customRole(), language, record, &moment)
				if got := moment.RecommendedAction.BlockedReason; got == nil || *got != want {
					t.Errorf("%s, %s on a %s: blocked_reason %v, want %q", language, name, record, got, want)
				}
			}
		}
	}
}

// On the account card the claim's contact is not the record the card sits on.
// A role that may change the company hears which grant is missing.
func TestAReaderWhoMayWriteTheRecordHearsWhichGrantIsMissing(t *testing.T) {
	logsOnly := holding(principal.SeatFull, map[string]principal.ObjectGrant{
		"activity": {Read: true, Create: true}, "contact": {Read: true}, "company": {Read: true, Update: true},
	})
	for _, language := range spoken {
		said := langcopy.For(language)
		for promise, want := range map[string]string{"task": said.Say(blockedWords.Complete), "claim": said.Say(blockedWords.Settle)} {
			moment := promised(promise)
			Withhold(logsOnly, language, "company", &moment)
			if got := moment.RecommendedAction.BlockedReason; got == nil || *got != want {
				t.Errorf("%s/%s: blocked_reason %v, want %q", language, promise, got, want)
			}
		}
	}
}
