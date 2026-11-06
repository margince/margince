// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Inbound says the other side wrote it, which the row's voice answers where it
// names a side. A colleague's mail can arrive inbound, and reading the
// transport alone credits it to the contact.
//
// Threaded() then puts "Re:" on a draft that replies to our own colleague.
func TestAColleaguesInboundMailIsNotTheContactsWord(t *testing.T) {
	inbound := crmcontracts.ActivityDirectionInbound
	colleague := crmcontracts.ActivityAuthorKindColleague
	contact := crmcontracts.ActivityAuthorKindContactRecord

	for _, tc := range []struct {
		name string
		kind *crmcontracts.ActivityAuthorKind
		want bool
	}{
		{"a colleague wrote it", &colleague, false},
		{"the contact wrote it", &contact, true},
		{"nobody named a side", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			act := crmcontracts.Activity{
				Id:        openapi_types.UUID(ids.New[ids.ActivityKind]().UUID),
				Kind:      crmcontracts.ActivityKindEmail,
				Direction: &inbound,
			}
			if tc.kind != nil {
				act.Voice = &crmcontracts.ActivityVoice{
					Author: &crmcontracts.ActivityAuthor{Kind: *tc.kind},
				}
			}
			if got := foldAbout(act, time.Now(), time.UTC).Inbound; got != tc.want {
				t.Errorf("Inbound = %v, want %v for an inbound row %s", got, tc.want, tc.name)
			}
		})
	}
}
