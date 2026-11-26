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

// The model is told who wrote each recent message, from the author projection
// the contact brief reads. The contact is never credited with
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

// A promise the claims read marks not current is no open obligation, as on the
// commitments card. A settled one still reads as settled.
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

// An open promise that looks kept is no open driver of the meeting. The prep
// says so once, in one plain line, and cites where the promise was made.
func TestAPromiseThatLooksKeptIsNotAnOpenDriverAndIsSaidOnce(t *testing.T) {
	claim := claimOf(crmcontracts.ConversationClaimKindCommitmentOurs, crmcontracts.ConversationClaimStatusOpen, false)
	claim.SourceActivityId = openapi_types.UUID(ids.NewV7())
	claim.LooksKept = &crmcontracts.ConversationClaimLooksKept{GuestName: "Riley Reed", MeetingAt: at(9), MeetingPrecision: "instant"}
	found := []crmcontracts.ConversationClaim{claim}

	if open := foldClaims("Pat", found); len(open) != 0 {
		t.Fatalf("a promise that looks kept still drives the prep: %+v", open)
	}
	kept := foldLooksKept("Pat", found)
	if len(kept) != 1 {
		t.Fatalf("looks kept = %d, want 1", len(kept))
	}
	lines := looksKeptLines(Input{Language: "en", LooksKept: kept})
	if len(lines) != 1 || !strings.Contains(lines[0].Text, "Looks kept") || !strings.Contains(lines[0].Text, "Riley Reed") {
		t.Fatalf("lines = %+v, want one plain looks-kept line naming the guest", lines)
	}
	if storedInstant.MatchString(lines[0].Text) {
		t.Errorf("text = %q, want the day in the installation's notation, not a stored instant", lines[0].Text)
	}
	if len(lines[0].Evidence) != 1 || lines[0].Evidence[0].EntityID != kept[0].SourceID {
		t.Errorf("evidence = %+v, want the conversation the promise was made in", lines[0].Evidence)
	}
}

// A promise Margince closed itself is said as closed by Margince, with the
// meeting when the reader may open it, and is no open driver.
func TestAPromiseTheProductClosedIsSaidAsSuchOnce(t *testing.T) {
	claim := claimOf(crmcontracts.ConversationClaimKindCommitmentOurs, crmcontracts.ConversationClaimStatusDone, false)
	claim.SourceActivityId = openapi_types.UUID(ids.NewV7())
	claim.AutoKept = &crmcontracts.ConversationClaimAutoKept{Evidence: &crmcontracts.ConversationClaimLooksKept{
		GuestName: "Riley Reed", MeetingAt: at(9), MeetingPrecision: "instant",
	}}
	withheld := claim
	withheld.AutoKept = &crmcontracts.ConversationClaimAutoKept{}
	for name, tc := range map[string]struct {
		claim crmcontracts.ConversationClaim
		want  string
	}{
		"with the meeting":    {claim, "Riley Reed"},
		"meeting not visible": {withheld, "Margince marked this kept"},
	} {
		t.Run(name, func(t *testing.T) {
			found := []crmcontracts.ConversationClaim{tc.claim}
			if open := foldClaims("Pat", found); len(open) != 0 {
				t.Fatalf("a closed promise still drives the prep: %+v", open)
			}
			lines := looksKeptLines(Input{Language: "en", LooksKept: foldLooksKept("Pat", found)})
			if len(lines) != 1 || !strings.Contains(lines[0].Text, "Margince marked this kept") || !strings.Contains(lines[0].Text, tc.want) {
				t.Fatalf("lines = %+v, want one line saying Margince marked it kept and %q", lines, tc.want)
			}
		})
	}
}
