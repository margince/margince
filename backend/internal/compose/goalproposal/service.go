// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package goalproposal

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/modelfailure"
	"github.com/margince/margince/backend/internal/compose/salesprogress"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// One model call may take this long.
const proposalDeadline = 20 * time.Second

// The codes a refused proposal answers with. The editor hides its ask on the
// first and says the second in the reader's language.
const (
	codeAINotConfigured = "ai_not_configured"
	codeNoSalesHistory  = "no_sales_history"
)

// Handlers serves the proposal the Sales goals editor asks for. It reads the
// seller's closed revenue as Progress does, asks the report_setup lane once
// and writes nothing.
type Handlers struct {
	Progress salesprogress.Reading
	// Lane is the report_setup lane. Unbound, the endpoint answers 409.
	Lane ai.Completer
	Now  func() time.Time
}

// WithLane binds the model lane the proposal asks.
func (h Handlers) WithLane(lane ai.Completer) Handlers {
	h.Lane = lane
	return h
}

// ProposeSalesGoalMonths implements POST /analytics/sales-goals/{owner_id}/{year}/proposed-months.
func (h Handlers) ProposeSalesGoalMonths(w http.ResponseWriter, r *http.Request, ownerID openapi_types.UUID, year int) {
	var body crmcontracts.SalesGoalProposalRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	out, err := h.propose(r.Context(), ids.UUID(ownerID), year, body)
	if err != nil {
		modelfailure.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// propose answers one seller's year. The grant to set their goals is asked
// first, so a reader who could not save the result spends no model call.
func (h Handlers) propose(ctx context.Context, owner ids.UUID, year int, req crmcontracts.SalesGoalProposalRequest) (crmcontracts.SalesGoalProposal, error) {
	if err := auth.RequireHumanOrItsAgent(ctx); err != nil {
		return crmcontracts.SalesGoalProposal{}, err
	}
	in, err := h.gather(ctx, owner, year, req)
	if err != nil {
		return crmcontracts.SalesGoalProposal{}, err
	}
	if h.Lane == nil {
		return crmcontracts.SalesGoalProposal{}, &httperr.DetailedError{
			Status: http.StatusConflict, Code: codeAINotConfigured,
			Detail: "a proposed plan needs an AI model — an administrator can bind one under Settings → AI; the months can still be entered by hand",
		}
	}
	if !in.HasHistory() {
		return crmcontracts.SalesGoalProposal{}, &httperr.DetailedError{
			Status: http.StatusConflict, Code: codeNoSalesHistory,
			Detail: "there is no closed revenue in this year or the last to read a pattern from — enter the months by hand",
		}
	}
	laneCtx, cancel := context.WithTimeout(ctx, proposalDeadline)
	defer cancel()
	res, err := ai.Ask(laneCtx, h.Lane, Request(in), func(reply string) error {
		_, err := Parse(reply, in)
		return err
	})
	if err != nil {
		return crmcontracts.SalesGoalProposal{}, err
	}
	proposal, err := Parse(res.Text, in)
	if err != nil {
		return crmcontracts.SalesGoalProposal{}, fmt.Errorf("%w: %w", ai.ErrOutputRejected, err)
	}
	out := crmcontracts.SalesGoalProposal{Months: proposal.Months, TotalMinor: proposal.Total(), Reason: proposal.Reason, GeneratedBy: crmcontracts.WrittenByModel}
	if res.ServedModel != "" {
		out.ModelUsed = &res.ServedModel
	}
	return out, nil
}

// gather reads what the model reads in one snapshot: the plan year's closed
// revenue and the year before's, once the reader may set this seller's goals.
func (h Handlers) gather(ctx context.Context, owner ids.UUID, year int, req crmcontracts.SalesGoalProposalRequest) (Input, error) {
	if req.YearlyTotalMinor != nil && *req.YearlyTotalMinor <= 0 {
		return Input{}, httperr.Validation("yearly_total_minor", "out_of_range", "enter a yearly goal above zero, or leave it out")
	}
	in := Input{Year: year, Today: h.Now().UTC().Format(time.DateOnly), Lang: identity.BaseLanguageForPrompt(ctx, h.Progress.DB.Pool())}
	err := h.Progress.DB.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		if err := h.Progress.Reports.RequireGoalEdit(ctx, tx, owner); err != nil {
			return err
		}
		for _, each := range []int{year, year - 1} {
			closed, err := h.Progress.ClosedByMonth(ctx, tx, owner, each)
			if err != nil {
				return err
			}
			in.Currency = closed.Currency
			in.Closed = append(in.Closed, yearIn(closed))
		}
		return nil
	})
	if err != nil {
		return in, err
	}
	if req.YearlyTotalMinor != nil {
		in.YearlyGoalMinor = *req.YearlyTotalMinor
		in.YearlyGoal = new(values.WholeMajorUnits(*req.YearlyTotalMinor, in.Currency))
	}
	return in, nil
}

// yearIn is a year as the model reads it: whole major units, the fraction
// dropped, since a pattern needs no cent.
func yearIn(closed salesprogress.ClosedYear) YearIn {
	out := YearIn{Year: closed.Year, Months: make([]*int64, len(closed.Months))}
	for index, month := range closed.Months {
		if month != nil {
			out.Months[index] = new(values.WholeMajorUnits(*month, closed.Currency))
		}
	}
	return out
}
