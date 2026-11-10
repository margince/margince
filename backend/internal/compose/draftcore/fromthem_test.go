// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/draftcore"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestFromThemReadsTheVoiceBeforeTheTransport(t *testing.T) {
	inbound, outbound := crmcontracts.ActivityDirectionInbound, crmcontracts.ActivityDirectionOutbound
	written := func(kind crmcontracts.ActivityAuthorKind) *crmcontracts.ActivityVoice {
		return &crmcontracts.ActivityVoice{Author: &crmcontracts.ActivityAuthor{Kind: kind}}
	}
	for _, tc := range []struct {
		name string
		row  crmcontracts.Activity
		want bool
	}{
		{"a note sent nowhere", crmcontracts.Activity{}, false},
		{"our outbound mail", crmcontracts.Activity{Direction: &outbound}, false},
		{"their inbound mail", crmcontracts.Activity{Direction: &inbound}, true},
		{"a colleague's inbound mail", crmcontracts.Activity{
			Direction: &inbound, Voice: written(crmcontracts.ActivityAuthorKindColleague),
		}, false},
		{"the contact's inbound mail", crmcontracts.Activity{
			Direction: &inbound, Voice: written(crmcontracts.ActivityAuthorKindContactRecord),
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := draftcore.FromThem(tc.row); got != tc.want {
				t.Errorf("FromThem = %v, want %v", got, tc.want)
			}
		})
	}
}
