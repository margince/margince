// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestACoverageLineCarriesTheNewestEventInEachDirection(t *testing.T) {
	lena, buyer := ids.NewV7(), ids.NewV7()
	wrote := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	met := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	out := wireCoverage(DealCoverage{
		DealID: ids.NewV7(), LinesReadable: true,
		OurSide: []ColleagueEdge{{UserID: lena, ContactID: buyer, LastOutboundAt: &wrote, LastMetAt: &met}},
	}, nil, nil)

	if out.Lines == nil || len(*out.Lines) != 1 {
		t.Fatalf("lines = %v, want one pair", out.Lines)
	}
	line := (*out.Lines)[0]
	if ids.UUID(line.UserId) != lena || ids.UUID(line.ContactId) != buyer {
		t.Errorf("line names %v and %v, want the colleague and the seat", line.UserId, line.ContactId)
	}
	if line.LastInboundAt != nil || !line.LastOutboundAt.Equal(wrote) || !line.LastMetAt.Equal(met) {
		t.Errorf("line dates = %v %v %v", line.LastInboundAt, line.LastOutboundAt, line.LastMetAt)
	}
}

func TestAPairWithNoExchangeHasNoLine(t *testing.T) {
	out := wireCoverage(DealCoverage{
		DealID: ids.NewV7(), LinesReadable: true,
		OurSide: []ColleagueEdge{{UserID: ids.NewV7(), ContactID: ids.NewV7()}},
	}, nil, nil)

	if out.Lines == nil || len(*out.Lines) != 0 {
		t.Errorf("lines = %v, want present and empty", out.Lines)
	}
}

func TestLinesAreAbsentNotEmptyWithoutTheActivityGrant(t *testing.T) {
	at := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	out := wireCoverage(DealCoverage{
		DealID:  ids.NewV7(),
		OurSide: []ColleagueEdge{{UserID: ids.NewV7(), ContactID: ids.NewV7(), LastInboundAt: &at}},
	}, nil, nil)

	if out.Lines != nil {
		t.Errorf("lines = %v, want absent so a seat is not called unreached", *out.Lines)
	}
}
