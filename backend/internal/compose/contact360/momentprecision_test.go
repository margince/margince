// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A moment's evidence carries the precision of the time it cites, so a reader
// takes no hour from a day entry's stored noon.
func TestAMomentsEvidenceCarriesThePrecisionOfItsTime(t *testing.T) {
	day := crmcontracts.OccurredPrecisionOccurredPrecisionDay
	prep := meeting(ahead(24))
	prep.OccurredPrecision = day
	for name, page := range map[string]*crmcontracts.Contact360{
		"meeting prep": {NextMeeting: prep},
		"gone quiet": {
			LastOutboundAt: new(at(9)), LastOutboundPrecision: &day,
			LastInboundAt: new(at(16)), LastLiveInboundAt: new(at(16)),
		},
	} {
		got := momentCopyFor("en").deriveMoment(readerCtx(), now, page)
		if len(got.Evidence) == 0 || got.Evidence[0].ObservedPrecision == nil || *got.Evidence[0].ObservedPrecision != day {
			t.Errorf("%s: the %q moment cites %+v, want its time read as a day", name, got.Rule, got.Evidence)
		}
	}
}
