// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The two privacy lanes are the only ones the worklist's warning list stays
// quiet about, and the reason is the reader's ROLE: a seat with no privacy
// inbox would otherwise be told "part of your day is hidden" every morning of
// its working life, for a room it is never meant to enter.
//
// That silence is the whole subject here, because it is the one place this
// surface deliberately declines to report a hole in the day. It has to be no
// wider than its reason: a privacy admin whose seat is mis-set — a missing
// contact grant, a non-human principal on a queue that admits no machine —
// gets the SAME refusal from the lane, and for them it is news. Swallowing it
// leaves them reading a clean page over a case queue they cannot open.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// privacyAdmin is a reader who holds the privacy inbox, so a refused privacy
// lane cannot be explained by the grant.
func privacyAdmin() principal.Principal {
	return principal.Principal{
		Type:   principal.PrincipalHuman,
		UserID: ids.MustParse("01a05500-0000-7000-8000-0000000000fd"),
		Permissions: principal.Permissions{
			RowScope: principal.RowScopeAll,
			Objects:  map[string]principal.ObjectGrant{"privacy_request": {Read: true}},
		},
	}
}

func TestAPrivacyLaneRefusedDespiteTheGrantIsNamedAsUnavailable(t *testing.T) {
	for _, lane := range []crmcontracts.AttentionLanesOmitted{laneDSR, laneNoticeCase} {
		t.Run(string(lane), func(t *testing.T) {
			omitted := []crmcontracts.AttentionLanesOmitted{lane}
			day := crmcontracts.Attention{AsOf: rankInstant, LanesOmitted: &omitted}

			got := unavailable(principal.WithActor(t.Context(), privacyAdmin()), day)

			if len(got) != 1 || got[0].Source != string(lane) {
				t.Fatalf("a holder of the privacy inbox was told nothing about a lane it may read: %+v", got)
			}
			if got[0].Reason != crmcontracts.WorklistSourceUnavailableReasonWithheld {
				t.Errorf("reason = %q, want withheld", got[0].Reason)
			}
		})
	}
}
