// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"fmt"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The last conversation line names who wrote it, which the row's voice answers
// where it names a side. A colleague's mail arrives inbound, so the transport
// alone hands the counterpart a message we sent.
func TestTheLastConversationLineNamesTheVoiceNotTheTransport(t *testing.T) {
	said := briefCopyFor("en")
	for _, tc := range []struct {
		name string
		act  ActIn
		want string
	}{
		{"a colleague's inbound mail is ours", ActIn{
			Subject: "Prices", Direction: "inbound", WroteOurSide: true, SideKnown: true,
		}, said.Say(sectionWords.WeWrote)},
		{"the counterpart's mail is theirs", ActIn{
			Subject: "Prices", Direction: "inbound", WroteOurSide: false, SideKnown: true,
		}, said.Say(sectionWords.TheyWrote)},
		{"a row naming no side is neither", ActIn{
			Subject: "Prices", SideKnown: false,
		}, said.Say(sectionWords.LastCaptured)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := said.lastConversationLine(tc.act)
			if want := fmt.Sprintf(tc.want, said.subjectOrKind(tc.act)); got != want {
				t.Errorf("line = %q, want %q", got, want)
			}
		})
	}
}

// Through the production fold: a colleague's inbound mail on the contact's page
// reaches the line as ours.
func TestAColleaguesInboundMailFoldsIntoAnOurSideLastLine(t *testing.T) {
	available := crmcontracts.ActivityContentStateAvailable
	inbound := crmcontracts.ActivityDirectionInbound
	view := crmcontracts.Contact360{
		Activities: &struct {
			Data []crmcontracts.Activity `json:"data"`
			Page crmcontracts.PageInfo   `json:"page"`
		}{
			Data: []crmcontracts.Activity{{
				Id:           openapi_types.UUID(ids.NewV7()),
				Kind:         crmcontracts.ActivityKindEmail,
				Subject:      new("Prices"),
				Direction:    &inbound,
				ContentState: &available,
				Voice: &crmcontracts.ActivityVoice{Author: &crmcontracts.ActivityAuthor{
					Kind: crmcontracts.ActivityAuthorKindColleague,
				}},
			}},
		},
	}
	in := &Input{}
	WithCounterpart(in, view)
	if len(in.Recent) != 1 {
		t.Fatalf("recent = %d, want 1", len(in.Recent))
	}
	said := briefCopyFor("en")
	got := said.lastConversationLine(in.Recent[0])
	if want := fmt.Sprintf(said.Say(sectionWords.WeWrote), "Prices"); got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}
