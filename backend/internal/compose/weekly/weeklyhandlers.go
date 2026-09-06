// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// The weekly review's HTTP surface: the archive index, and one week.
//
// Both are reads. There is no assemble endpoint — a retrospective is written
// when its week closes, by the job, and a rep pressing a button to build last
// week again would get a different answer than the one they read on Monday.

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// weekIndexCap bounds the archive index. A year of weeks is more than anybody
// scrolls, and the index is a list of doors rather than a report.
const weekIndexCap = 52

// Handlers serves the weekly review.
type Handlers struct{ engine *Engine }

// NewHandlers binds the handlers to the engine.
func NewHandlers(engine *Engine) Handlers { return Handlers{engine: engine} }

// ListWeeklyReviews implements (GET /weekly-reviews): the archive's index.
func (h Handlers) ListWeeklyReviews(w http.ResponseWriter, r *http.Request) {
	weeks, err := h.engine.ListWeeks(r.Context(), weekIndexCap)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	// Never null on the wire: a reader seeing `null` has to decide whether it
	// means "no weeks yet" or "the list was not read".
	out := make([]openapi_types.Date, 0, len(weeks))
	for _, week := range weeks {
		out = append(out, openapi_types.Date{Time: week})
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.WeeklyReviewIndex{Weeks: out})
}

// GetLatestWeeklyReview implements (GET /weekly-reviews/latest): one week.
func (h Handlers) GetLatestWeeklyReview(
	w http.ResponseWriter, r *http.Request, params crmcontracts.GetLatestWeeklyReviewParams,
) {
	var week *time.Time
	if params.Week != nil {
		day := params.Week.Time
		week = &day
	}
	review, err := h.engine.LatestReview(r.Context(), week)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, reviewToWire(review))
}

// nullableText serves empty prose as JSON null rather than "".
//
// The distinction is the contract's: null means no pass wrote one, and the
// screen tells that apart from "a pass ran and had nothing to say" through
// narrated_at. An empty string would be a third spelling of the same absence
// that neither field documents.
func nullableText(text string) *string {
	if text == "" {
		return nil
	}
	return &text
}

func reviewToWire(review Review) crmcontracts.WeeklyReview {
	deals := make([]crmcontracts.WeeklyReviewDeal, 0, len(review.Deals))
	for _, line := range review.Deals {
		deals = append(deals, dealLineToWire(line))
	}
	out := crmcontracts.WeeklyReview{
		Id:             openapi_types.UUID(review.ID),
		LocalWeekStart: openapi_types.Date{Time: review.LocalWeekStart},
		GeneratedAt:    review.GeneratedAt,
		AsOf:           review.AsOf,
		Narrative:      nullableText(review.Narrative),
		NarratedAt:     review.NarratedAt,
		Counts:         countsToWire(review.Counts),
		Pipeline:       pipelineToWire(review.Money),
		Deals:          deals,
	}
	if review.Prior != nil {
		out.Prior = &crmcontracts.WeeklyReviewPrior{
			LocalWeekStart: openapi_types.Date{Time: review.Prior.LocalWeekStart},
			Counts:         countsToWire(review.Prior.Counts),
			Pipeline:       pipelineToWire(review.Prior.Money),
		}
	}
	if len(review.Outlook) > 0 {
		outlook := make([]crmcontracts.WeeklyReviewOutlook, 0, len(review.Outlook))
		for _, horizon := range review.Outlook {
			outlook = append(outlook, outlookToWire(horizon))
		}
		out.Outlook = &outlook
	}
	if review.Scorecard != nil {
		out.Scorecard = scorecardToWire(*review.Scorecard)
	}
	return out
}

// scorecardToWire renders the week's judgement.
//
// Each block travels only when it is present. An absent block is omitted rather
// than sent as zeros, because a reader that received zeros would draw a rep who
// carried no leads as having failed at the funnel.
func scorecardToWire(card Scorecard) *crmcontracts.WeeklyReviewScorecard {
	out := &crmcontracts.WeeklyReviewScorecard{}
	if l := card.Lead; l != nil {
		out.Lead = &crmcontracts.WeeklyScorecardLeadBlock{
			Advanced: l.Advanced, Disqualified: l.Disqualified, Promoted: l.Promoted,
			AnsweredInTarget: l.AnsweredInTarget, Breached: l.Breached,
			MeetingsBooked: l.Booked, MeetingsHeld: l.Held, MeetingsNoShow: l.NoShow,
			MeetingsPartialHistory: l.PartialHistory,
		}
	}
	if d := card.Deal; d != nil {
		out.Deal = &crmcontracts.WeeklyScorecardDealBlock{
			Advances: d.Advances, Regressions: d.Regressions,
			MedianDaysInStage: d.MedianDaysInStage,
			WithNextStep:      d.WithNextStep, Open: d.Open,
			MultiThreaded: d.MultiThreaded, CloseDateSound: d.CloseDateSound,
			ForecastUp: d.ForecastUp, ForecastDown: d.ForecastDown,
		}
	}
	return out
}

// outlookToWire renders one horizon's frozen landing.
//
// The opening side is omitted rather than zeroed when no Monday snapshot
// existed: the two are different facts, and a zero opening draws a week that
// started from nothing and made everything.
func outlookToWire(horizon Outlook) crmcontracts.WeeklyReviewOutlook {
	out := crmcontracts.WeeklyReviewOutlook{
		PeriodKind:    crmcontracts.WeeklyReviewOutlookPeriodKind(horizon.PeriodKind),
		PeriodStart:   openapi_types.Date{Time: horizon.PeriodStart},
		PeriodEnd:     openapi_types.Date{Time: horizon.PeriodEnd},
		BaseCurrency:  horizon.BaseCurrency,
		WonMinor:      horizon.WonMinor,
		CommitMinor:   horizon.CommitMinor,
		BestCaseMinor: horizon.BestCaseMinor,
		WeightedMinor: horizon.WeightedMinor,
		Movement:      movementToWire(horizon.Movement),
	}
	if horizon.Opening.Known {
		opening := horizon.Opening.LandingMinor
		out.OpeningLandingMinor = &opening
	}
	if horizon.Closing.Known {
		closing := horizon.Closing.LandingMinor
		out.ClosingLandingMinor = &closing
	}
	if horizon.ForwardMeasure != "" {
		measure := crmcontracts.WeeklyReviewOutlookForwardMeasure(horizon.ForwardMeasure)
		out.ForwardMeasure = &measure
	}
	return out
}

// movementToWire renders the bars, each with the deals behind it.
func movementToWire(bars []Movement) []crmcontracts.WeeklyReviewMovement {
	out := make([]crmcontracts.WeeklyReviewMovement, 0, len(bars))
	for _, bar := range bars {
		wire := crmcontracts.WeeklyReviewMovement{
			Bar:        crmcontracts.WeeklyReviewMovementBar(bar.Bar),
			DeltaMinor: bar.DeltaMinor,
		}
		if len(bar.Drivers) > 0 {
			drivers := make([]crmcontracts.WeeklyReviewDriver, 0, len(bar.Drivers))
			for _, driver := range bar.Drivers {
				drivers = append(drivers, crmcontracts.WeeklyReviewDriver{
					DealId:     openapi_types.UUID(driver.DealID),
					DealLabel:  driver.DealLabel,
					DeltaMinor: driver.DeltaMinor,
				})
			}
			wire.Drivers = &drivers
		}
		out = append(out, wire)
	}
	return out
}

func countsToWire(c Counts) crmcontracts.WeeklyReviewCounts {
	return crmcontracts.WeeklyReviewCounts{
		TasksDue: c.TasksDue, TasksDone: c.TasksDone,
		TasksCarriedOver: c.TasksCarriedOver,
		DealsMoved:       c.DealsMoved, DealsWon: c.DealsWon, DealsLost: c.DealsLost,
		ProposalsAccepted: c.ProposalsAccepted, ProposalsRejected: c.ProposalsRejected,
		BriefItemsActed: c.BriefItemsActed, BriefItemsDismissed: c.BriefItemsDismissed,
		CommitmentsDue: c.CommitmentsDue, CommitmentsKept: c.CommitmentsKept,
		LeadsRouted: c.LeadsRouted, LeadsAnsweredInTarget: c.LeadsAnsweredInTarget,
		LeadsBreached: c.LeadsBreached,
		MeetingsHeld:  c.MeetingsHeld, MeetingsWithNextStep: c.MeetingsWithNextStep,
	}
}

// pipelineToWire renders the money, or nothing.
//
// Absent rather than a block of zeros when the week could not be converted: a
// reader cannot tell a zero that means "nothing happened" from one that means
// "we could not work it out", and only one of those is true.
func pipelineToWire(money Money) *crmcontracts.WeeklyReviewPipeline {
	if !money.Known {
		return nil
	}
	return &crmcontracts.WeeklyReviewPipeline{
		CreatedMinor: money.CreatedMinor,
		WonMinor:     money.WonMinor,
		LostMinor:    money.LostMinor,
		Currency:     money.Currency,
	}
}

func dealLineToWire(line DealLine) crmcontracts.WeeklyReviewDeal {
	out := crmcontracts.WeeklyReviewDeal{
		DealId:     openapi_types.UUID(line.DealID),
		Label:      line.Label,
		Outcome:    crmcontracts.WeeklyReviewDealOutcome(line.Outcome),
		OccurredAt: line.OccurredAt,
	}
	if line.ToStageLabel != "" {
		stage := line.ToStageLabel
		out.ToStageLabel = &stage
	}
	// Money is a pair or it is absent — a bare amount is a number nobody can
	// read, and this tree refuses to print one.
	if line.AmountMinor != nil && line.Currency != "" {
		amount, currency := *line.AmountMinor, line.Currency
		out.AmountMinorAtClose = &amount
		out.CurrencyAtClose = &currency
	}
	return out
}
