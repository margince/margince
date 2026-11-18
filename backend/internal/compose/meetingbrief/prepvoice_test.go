// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/contactbrief"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func voiced(kind crmcontracts.ActivityAuthorKind, role *crmcontracts.ActivityVoiceContactRole) crmcontracts.Activity {
	available := crmcontracts.ActivityContentStateAvailable
	inbound := crmcontracts.ActivityDirectionInbound
	return crmcontracts.Activity{
		Id: openapi_types.UUID(ids.NewV7()), Kind: crmcontracts.ActivityKindEmail,
		Subject: new("Partnership"), Direction: &inbound, ContentState: &available,
		Voice: &crmcontracts.ActivityVoice{
			Author:      &crmcontracts.ActivityAuthor{Kind: kind, IsReader: kind == crmcontracts.ActivityAuthorKindColleague},
			ContactRole: role,
		},
	}
}

// The model is told who wrote each recent message, from the same author
// projection the contact brief reads, so the contact is never credited with
// the reader's own proposal.
func TestRecentMessagesNameTheirSpeakerAndNotTheTransport(t *testing.T) {
	author := crmcontracts.ActivityVoiceContactRoleAuthor
	view := crmcontracts.Contact360{
		Activities: &struct {
			Data []crmcontracts.Activity `json:"data"`
			Page crmcontracts.PageInfo   `json:"page"`
		}{Data: []crmcontracts.Activity{
			voiced(crmcontracts.ActivityAuthorKindColleague, nil),
			voiced(crmcontracts.ActivityAuthorKindContactRecord, &author),
		}},
	}
	in := &Input{}
	WithCounterpart(in, view)
	if len(in.Recent) != 2 {
		t.Fatalf("recent = %d, want 2", len(in.Recent))
	}
	if got := in.Recent[0].Speaker; got == nil || got.Kind != contactbrief.SpeakerYou {
		t.Errorf("our own inbound-direction mail speaker = %+v, want you", got)
	}
	if got := in.Recent[1].Speaker; got == nil || got.Kind != contactbrief.SpeakerThem {
		t.Errorf("the contact's mail speaker = %+v, want them", got)
	}
}

// The plan prompt carries the excerpt's speaker, and both meeting prompts hold
// the attribution rule.
func TestThePlanPromptCarriesTheSpeakerAndBothPromptsHoldTheRule(t *testing.T) {
	excerpt := ExcerptIn{
		ActivityID: "a1", Subject: "Partnership", Direction: "inbound",
		Speaker: &contactbrief.Speaker{Kind: contactbrief.SpeakerYou},
	}
	message := promptMessageOf(excerpt, HistoryIn{})
	if message.Speaker == nil || message.Speaker.Kind != contactbrief.SpeakerYou {
		t.Fatalf("prompt message speaker = %+v, want you", message.Speaker)
	}
	encoded := encodePlanPrompt(PlanPrompt{Moments: []PromptMoment{{Messages: []PromptMessage{message}}}})
	if !strings.Contains(encoded, `"speaker":{"kind":"you"}`) {
		t.Errorf("plan prompt = %s, want the speaker object", encoded)
	}
	for name, system := range map[string]string{"plan": planSystem, "brief": briefSystem} {
		if !strings.Contains(system, speakerRule) {
			t.Errorf("%s prompt lacks the attribution rule", name)
		}
	}
}

func claimOf(kind crmcontracts.ConversationClaimKind, status crmcontracts.ConversationClaimStatus, review bool) crmcontracts.ConversationClaim {
	return crmcontracts.ConversationClaim{Kind: kind, Status: status, NeedsReview: review, Body: "invite Pat"}
}

// A promise the claims read marks not current is no open obligation, the same
// as on the commitments card; a settled one still reads as settled.
func TestPrepDropsAnOpenPromiseThatIsNotCurrent(t *testing.T) {
	ours, theirs := crmcontracts.ConversationClaimKindCommitmentOurs, crmcontracts.ConversationClaimKindCommitmentTheirs
	open, done := crmcontracts.ConversationClaimStatusOpen, crmcontracts.ConversationClaimStatusDone
	got := foldClaims("Pat", []crmcontracts.ConversationClaim{
		claimOf(ours, open, true),
		claimOf(theirs, open, true),
		claimOf(ours, done, true),
		claimOf(ours, open, false),
		claimOf(crmcontracts.ConversationClaimKindObjection, open, true),
	})
	var seen []string
	for _, claim := range got {
		seen = append(seen, claim.Kind+"/"+claim.Status)
	}
	want := "commitment_ours/done commitment_ours/open objection/open"
	if strings.Join(seen, " ") != want {
		t.Errorf("folded = %v, want %q", seen, want)
	}
}
