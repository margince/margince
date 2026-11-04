// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/salesassessment"
	"github.com/margince/margince/backend/internal/compose/weekly"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s *salesAssessments) upcomingFacts(ctx context.Context, evaluation crmcontracts.ReportingEvaluation) ([]salesassessment.CellIn, error) {
	cells := []salesassessment.CellIn{}
	if !weekly.MayReadMeetings(ctx) {
		return cells, nil
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		resolved, err := ResolveAnalyticsScope(ctx, tx, reportingRequested(evaluation.Context.Scope))
		if err != nil {
			return err
		}
		allowed, err := mayReadIndividualWork(ctx, tx, resolved)
		if err != nil {
			return err
		}
		if !allowed {
			return nil
		}
		window, err := upcomingWindow(ctx, tx, s.now().UTC())
		if err != nil {
			return err
		}
		count, err := readUpcomingMeetings(ctx, tx, reportingRequested(evaluation.Context.Scope), window)
		if err != nil {
			return err
		}
		zone, err := time.LoadLocation(evaluation.Context.Timezone)
		if err != nil {
			return err
		}
		words := reporting.ExplanationCopy(evaluation.Context)
		value := float64(count)
		cells = append(cells, assessmentCell("work:upcoming", words.Say(langcopy.Phrase{En: "Meetings booked in the next 14 days", De: "Gebuchte Termine in den nächsten 14 Tagen", Vi: "Cuộc họp đã đặt trong 14 ngày tới"}), &value, "count", assessmentWindow(crmcontracts.ReportingWindow{StartAt: window.StartAt, EndAt: window.EndAt}, zone), crmcontracts.ReportingCoverage{Status: "ok"}))
		return nil
	})
	return cells, err
}

func displayAssessmentCells(in *salesassessment.Input, evaluation crmcontracts.ReportingEvaluation) {
	language := string(reporting.ExplanationCopy(evaluation.Context).Lang())
	for i := range in.Cells {
		cell := &in.Cells[i]
		number := strings.TrimSpace(strings.TrimSuffix(cell.Value, " "+cell.Unit))
		if value, err := strconv.ParseFloat(number, 64); err == nil {
			cell.Display = salesassessment.DisplayQuantity(value, cell.Unit, language)
		}
	}
}

func assessmentWorthExplaining(in salesassessment.Input) bool {
	if len(in.Actions) > 0 || len(in.Decisions) > 0 {
		return true
	}
	for _, cell := range in.Cells {
		if cell.Coverage != "ok" && cell.Coverage != reportingPartial {
			continue
		}
		raw := strings.TrimSpace(strings.TrimSuffix(cell.Value, " "+cell.Unit))
		value, err := strconv.ParseFloat(raw, 64)
		if err == nil && value != 0 {
			return true
		}
	}
	return false
}

func mayReadIndividualWork(ctx context.Context, tx pgx.Tx, scope ResolvedScope) (bool, error) {
	actor, ok := principal.Actor(ctx)
	if ok && scope.Kind == ScopeKindOwner && scope.ID != nil && *scope.ID == actor.UserID {
		return true, nil
	}
	owners, err := readerMeasurableOwners(ctx, tx)
	if err != nil {
		return false, err
	}
	return !owners.IndividualOff, nil
}
