// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Every arm of the rule, including the one that answers for nobody. Readers
// across compose take their side from this, so an arm read wrong there is a
// stamp or a prompt naming the wrong party.
func TestWhoHeldThePenFollowsTheVoiceWithinCorrespondence(t *testing.T) {
	t.Parallel()
	inbound := crmcontracts.ActivityDirectionInbound
	outbound := crmcontracts.ActivityDirectionOutbound
	voice := func(kind crmcontracts.ActivityAuthorKind) *crmcontracts.ActivityVoice {
		return &crmcontracts.ActivityVoice{Author: &crmcontracts.ActivityAuthor{Kind: kind}}
	}
	for _, c := range []struct {
		name        string
		row         crmcontracts.Activity
		ours, known bool
	}{
		{"a colleague's mail that arrived is ours", crmcontracts.Activity{
			Direction: &inbound, Voice: voice(crmcontracts.ActivityAuthorKindColleague),
		}, true, true},
		{"the contact's mail that left here is theirs", crmcontracts.Activity{
			Direction: &outbound, Voice: voice(crmcontracts.ActivityAuthorKindContactRecord),
		}, false, true},
		{"a machine names no side, so inbound is theirs", crmcontracts.Activity{
			Direction: &inbound, Voice: voice(crmcontracts.ActivityAuthorKindMachine),
		}, false, true},
		{"an unresolved sender leaves outbound ours", crmcontracts.Activity{
			Direction: &outbound, Voice: voice(crmcontracts.ActivityAuthorKindUnresolved),
		}, true, true},
		{"no voice at all reads the transport", crmcontracts.Activity{Direction: &inbound}, false, true},
		{"a logged row is nobody's word, whoever authored it", crmcontracts.Activity{
			Voice: voice(crmcontracts.ActivityAuthorKindColleague),
		}, false, false},
	} {
		ours, known := WroteItOurSide(c.row)
		if ours != c.ours || known != c.known {
			t.Errorf("%s: ours=%v known=%v, want %v and %v", c.name, ours, known, c.ours, c.known)
		}
	}
}
