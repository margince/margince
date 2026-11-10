// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Our last mail says we will wait for them, and our reminder to chase them is
// open. The record waits on them.
func TestWeWroteLastAndWaitReadsAsAwaitingTheirReply(t *testing.T) {
	page := &crmcontracts.Contact360{
		LastOutboundAt: new(at(2)), LastInboundAt: new(at(6)), LastLiveInboundAt: new(at(6)),
		NextSteps: timelineOf(crmcontracts.Activity{
			Id: openapi_types.UUID(ids.NewV7()), Kind: "task",
			Subject: new("Follow up if they have not answered"), OccurredAt: at(1), DueAt: new(ahead(72)),
		}),
	}
	got := momentCopyFor("en").deriveMoment(readerCtx(), now, page)
	if got.Rule != crmcontracts.ContactMomentRuleAwaitingReply {
		t.Fatalf("rule = %q, want awaiting_reply", got.Rule)
	}
	if len(got.Evidence) != 1 || got.Evidence[0].ObservedAt == nil || !got.Evidence[0].ObservedAt.Equal(at(2)) {
		t.Fatalf("evidence = %+v, want our last message", got.Evidence)
	}
	if got.WhyNow != "Your last message went out 2 days ago and is the newest on the record. The rule flags a missing reply after 7 days." {
		t.Errorf("why now = %q", got.WhyNow)
	}
}

func TestTheWaitDoesNotHideWhatWeOweOrTheirAnswer(t *testing.T) {
	cases := map[string]struct {
		page *crmcontracts.Contact360
		want crmcontracts.ContactMomentRule
	}{
		"a promise in our latest mail is ours to keep": {
			page: &crmcontracts.Contact360{
				LastOutboundAt: new(at(2)),
				Claims:         &[]crmcontracts.ConversationClaim{ourClaim(at(2), new(ahead(48)))},
			},
			want: crmcontracts.ContactMomentRuleOpenPromise,
		},
		"an unrelated mail of ours leaves an older promise owed": {
			page: &crmcontracts.Contact360{
				LastOutboundAt: new(at(2)),
				Claims:         &[]crmcontracts.ConversationClaim{ourClaim(at(10), new(ahead(240)))},
			},
			want: crmcontracts.ContactMomentRuleOpenPromise,
		},
		"a task written for a commitment is owed": {
			page: &crmcontracts.Contact360{
				LastOutboundAt: new(at(2)),
				NextSteps: timelineOf(crmcontracts.Activity{
					Id: openapi_types.UUID(ids.NewV7()), Kind: "task", Subject: new("Send the listing details"),
					OccurredAt: at(3), DueAt: new(ahead(72)), SourceSystem: new(activities.CommitmentTaskSource),
				}),
			},
			want: crmcontracts.ContactMomentRuleOpenPromise,
		},
		"a promise past its date still wins": {
			page: &crmcontracts.Contact360{
				LastOutboundAt: new(at(2)),
				Claims:         &[]crmcontracts.ConversationClaim{ourClaim(at(10), new(at(3)))},
			},
			want: crmcontracts.ContactMomentRuleOverduePromise,
		},
		"they answered after our mail": {
			page: &crmcontracts.Contact360{
				LastOutboundAt: new(at(3)), LastInboundAt: new(at(1)), LastLiveInboundAt: new(at(1)),
				Claims: &[]crmcontracts.ConversationClaim{ourClaim(at(10), new(ahead(48)))},
			},
			want: crmcontracts.ContactMomentRuleOpenPromise,
		},
		"the last day before the rule still waits": {
			page: &crmcontracts.Contact360{LastOutboundAt: new(at(goneQuietAfterDays - 1))},
			want: crmcontracts.ContactMomentRuleAwaitingReply,
		},
		"a wait past the rule is a silence": {
			page: &crmcontracts.Contact360{LastOutboundAt: new(at(goneQuietAfterDays))},
			want: crmcontracts.ContactMomentRuleGoneQuiet,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := momentCopyFor("en").deriveMoment(readerCtx(), now, tc.page).Rule; got != tc.want {
				t.Fatalf("rule = %q, want %q", got, tc.want)
			}
		})
	}
}

func ourClaim(said time.Time, due *time.Time) crmcontracts.ConversationClaim {
	return crmcontracts.ConversationClaim{
		Id: openapi_types.UUID(ids.NewV7()), Kind: crmcontracts.ConversationClaimKindCommitmentOurs,
		Status: crmcontracts.ConversationClaimStatusOpen, Body: "Send the revised dispatch quote",
		SourceQuote: "We will send the revised quote.", SourceActivityId: openapi_types.UUID(ids.NewV7()),
		OccurredAt: &said, DueAt: due,
	}
}

// Their mail and ours at one stored instant. A day on either side makes ours
// the answer and the wait theirs. Two instants leave the move ours.
func TestATieWaitsOnThemOnlyForADayEntry(t *testing.T) {
	day, instant := crmcontracts.OccurredPrecisionOccurredPrecisionDay, crmcontracts.OccurredPrecisionOccurredPrecisionInstant
	for precision, awaiting := range map[crmcontracts.OccurredPrecision]bool{day: true, instant: false} {
		page := &crmcontracts.Contact360{
			LastOutboundAt: new(at(2)), LastOutboundPrecision: new(precision),
			LastInboundAt: new(at(2)), LastInboundPrecision: new(precision),
			LastLiveInboundAt: new(at(2)), LastLiveInboundPrecision: new(precision),
		}
		got := momentCopyFor("en").deriveMoment(readerCtx(), now, page)
		if (got.Rule == crmcontracts.ContactMomentRuleAwaitingReply) != awaiting {
			t.Errorf("%s tie: rule = %q, want awaiting_reply = %v", precision, got.Rule, awaiting)
		}
	}
}
