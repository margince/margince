// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AnswerReportingQuestionCard implements GET /analytics/reports/{id}/questions/{card_id}/answer.
func (h analyticsQueryHandlers) AnswerReportingQuestionCard(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id, cardID string, params crmcontracts.AnswerReportingQuestionCardParams,
) {
	ctx := r.Context()
	var (
		query      analyticsquery.Query
		restricted bool
		answer     AnalyticsAnswer
		binding    *crmcontracts.ReportingQuestionBinding
	)
	if err := h.db.Tx(ctx, func(tx pgx.Tx) error {
		card, scope, err := h.reportMetrics.QuestionCardIn(ctx, tx, ids.UUID(id), params.Revision, cardID)
		if err != nil {
			return err
		}
		binding = card.Binding
		query, restricted, err = resolveCard(ctx, tx, card, scope)
		if err != nil || restricted {
			return err
		}
		answer, err = RunAnalyticsQuery(ctx, tx, query, h.floor)
		return err
	}); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if restricted {
		httperr.WriteJSON(w, http.StatusOK, crmcontracts.ReportingQuestionAnswer{Status: crmcontracts.ReportingQuestionAnswerStatusRestricted, Binding: binding})
		return
	}
	labelled := h.labelledAnswer(ctx, query, answer)
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ReportingQuestionAnswer{
		Status: crmcontracts.ReportingQuestionAnswerStatusAnswered, Binding: binding, Answer: &labelled,
	})
}

// ExplainReportingQuestionCard implements POST /analytics/reports/{id}/questions/{card_id}/explain.
// A reader the card is restricted for is answered as for a card that is not
// there.
func (h analyticsQueryHandlers) ExplainReportingQuestionCard(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id, cardID string, params crmcontracts.ExplainReportingQuestionCardParams,
) {
	var body crmcontracts.ReportRunCell
	if !httperr.Decode(w, r, &body) {
		return
	}
	ctx := r.Context()
	var out AnalyticsExplanation
	if err := h.db.Tx(ctx, func(tx pgx.Tx) error {
		card, scope, err := h.reportMetrics.QuestionCardIn(ctx, tx, ids.UUID(id), params.Revision, cardID)
		if err != nil {
			return err
		}
		query, restricted, err := resolveCard(ctx, tx, card, scope)
		if err != nil {
			return err
		}
		if restricted {
			return apperrors.ErrNotFound
		}
		explain := analyticsquery.Explain{Query: query}
		if body.Group != nil {
			explain.Group = *body.Group
		}
		out, err = ExplainAnalyticsCell(ctx, tx, explain, h.floor)
		return err
	}); err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, h.labelledExplanation(ctx, out))
}
