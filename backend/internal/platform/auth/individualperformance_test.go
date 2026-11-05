// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAReaderAlwaysMeasuresThemselvesAndAColleagueOnlyWhileAllowed(t *testing.T) {
	me, colleague := ids.NewV7(), ids.NewV7()
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, UserID: me})
	for _, tc := range []struct {
		seat    ids.UUID
		allowed bool
		want    bool
	}{
		{me, false, true},
		{colleague, false, false},
		{ids.Nil, false, false},
		{colleague, true, true},
	} {
		if got := MayMeasureSeat(ctx, tc.allowed, tc.seat); got != tc.want {
			t.Errorf("MayMeasureSeat(allowed %v, self %v) = %v, want %v", tc.allowed, tc.seat == me, got, tc.want)
		}
	}
	if MayMeasureSeat(context.Background(), false, me) {
		t.Error("a context with no reader measured somebody with the switch off")
	}
}

// The switch closes every team's week and leaves a lead's reach for team
// totals alone.
func TestTheSwitchClosesTeamWeeksButNotTheTeamsALeadAnswersFor(t *testing.T) {
	lead := seatReading(principal.PrincipalHuman, principal.RowScopeTeam, holds{leads: true})
	if got := TeamWeekReachOf(WithIndividualPerformance(lead, true)); got != ReachesTeamsLed {
		t.Errorf("a lead's week reach with the switch on = %v, want teams led", got)
	}
	off := WithIndividualPerformance(lead, false)
	if got := TeamWeekReachOf(off); got != ReachesNoTeam {
		t.Errorf("a lead's week reach with the switch off = %v, want none", got)
	}
	if got := TeamLeadReachOf(off); got != ReachesTeamsLed {
		t.Errorf("a lead's team reach with the switch off = %v, want teams led", got)
	}
	if !errors.Is(IndividualPerformanceOffError{}, apperrors.ErrPermissionDenied) {
		t.Error("the individual-performance refusal is not a permission refusal")
	}
}
