// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/modelfailure"
	"github.com/margince/margince/backend/internal/compose/searchplan"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

type searchVocabulary = searchplan.Vocabulary

// AskSearchData plans with vocabulary alone, then reads through the existing seams.
func (s Server) AskSearchData(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.AskSearchDataJSONRequestBody
	if !httperr.Decode(w, r, &body) {
		return
	}
	question, err := httperr.RequireNonBlank("question", body.Question)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := auth.RequireHumanOrItsAgent(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := requireSearchDataRead(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if s.lane == nil {
		httperr.WriteJSON(w, http.StatusOK, crmcontracts.SearchDataAnswer{Outcome: crmcontracts.SearchDataAnswerOutcomeUnsupported})
		return
	}
	out, err := s.answerSearchData(r.Context(), question)
	if err != nil {
		modelfailure.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (s Server) answerSearchData(ctx context.Context, question string) (crmcontracts.SearchDataAnswer, error) {
	vocabulary, err := s.searchDataVocabulary(ctx)
	if err != nil {
		return crmcontracts.SearchDataAnswer{}, err
	}
	laneCtx, cancel := context.WithTimeout(ctx, setupDeadline)
	defer cancel()
	response, err := ai.Ask(laneCtx, s.lane, searchplan.Request(question, vocabulary), func(reply string) error {
		_, err := searchplan.Read(reply, question, vocabulary)
		return err
	})
	if err != nil {
		return crmcontracts.SearchDataAnswer{}, err
	}
	plan, err := searchplan.Read(response.Text, question, vocabulary)
	if err != nil {
		return crmcontracts.SearchDataAnswer{}, fmt.Errorf("%w: %w", ai.ErrOutputRejected, err)
	}
	return s.executeSearchData(ctx, plan)
}

func (s Server) searchDataVocabulary(ctx context.Context) (string, error) {
	records, err := search.NewQuerySchemaResource(queryVocabulary(s.filterProposalHandlers.pool)).ReadResource(ctx, search.QuerySchemaURI)
	if err != nil {
		return "", err
	}
	analytics, err := analyticsVocabularyText(ctx, s.analyticsQueryHandlers.db)
	if err != nil {
		return "", err
	}
	var frame crmcontracts.AnalyticsContext
	if auth.Allows(ctx, string(recordTypeDeal), principal.ActionRead) || auth.Allows(ctx, "forecast", principal.ActionRead) {
		frame, err = s.analyticsContextHandlers.read(ctx)
	} else {
		err = s.analyticsQueryHandlers.db.Tx(ctx, func(tx pgx.Tx) error {
			settings, err := readReportFrame(ctx, tx)
			frame.Timezone, frame.BaseCurrency = settings.Timezone, settings.BaseCurrency
			return err
		})
	}
	if err != nil {
		return "", err
	}
	var catalog crmcontracts.ReportingCatalog
	if auth.Allows(ctx, "report_definition", principal.ActionRead) {
		catalog, err = s.reportMetrics.Catalog(ctx)
		if err != nil {
			return "", err
		}
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return "", httperr.Validation("actor", "missing", "sign in before asking a question")
	}
	encoded, err := json.Marshal(searchVocabulary{
		Now: s.filterProposalHandlers.now(), Context: frame, Metrics: catalog, SelfUserID: actor.UserID.String(),
		BaseCurrencyScale: values.MinorUnitScale(frame.BaseCurrency), MinorUnitExceptions: values.MinorUnitExceptions(),
		AnalyticsVocabulary: analytics, RecordVocabulary: json.RawMessage(records.Text),
	})
	if err != nil {
		return "", fmt.Errorf("encode search vocabulary: %w", err)
	}
	return string(encoded), nil
}

func (s Server) executeSearchData(ctx context.Context, plan searchplan.Plan) (crmcontracts.SearchDataAnswer, error) {
	out := crmcontracts.SearchDataAnswer{Outcome: crmcontracts.SearchDataAnswerOutcomeAnswered, Source: &plan.Arguments, Count: &plan.Count}
	switch plan.Engine {
	case searchplan.EngineReporting:
		evaluation, err := s.searchReporting(ctx, plan)
		if err != nil {
			return out, err
		}
		if !searchplan.SupportsMetric(evaluation, plan.Count) {
			return crmcontracts.SearchDataAnswer{Outcome: crmcontracts.SearchDataAnswerOutcomeUnsupported}, nil
		}
		out.Evaluation = &evaluation
	case searchplan.EngineAnalytics:
		var query crmcontracts.AnalyticsQuery
		if err := searchplan.DecodeArguments(plan.Arguments, &query); err != nil {
			return out, err
		}
		if query.Save != nil && *query.Save {
			return out, httperr.Validation("save", "read_only", "search questions cannot save queries")
		}
		question := queryFromWire(query)
		var answer AnalyticsAnswer
		if err := s.analyticsQueryHandlers.db.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
			var err error
			answer, err = RunAnalyticsQuery(ctx, tx, question, s.floor)
			return err
		}); err != nil {
			return out, err
		}
		wire := s.labelledAnswer(ctx, question, answer)
		out.Analytics = &wire
		out.AnalyticsQuery = &query
	case searchplan.EngineRecords:
		raw, err := s.toolRegistry.Invoke(ctx, "query_workspace", json.RawMessage(plan.Arguments))
		if err != nil {
			return out, err
		}
		payload, _, err := unwrapToolEnvelope(raw)
		if err != nil {
			return out, err
		}
		var records crmcontracts.WorkspaceQueryResult
		if err := json.Unmarshal(payload, &records); err != nil {
			return out, fmt.Errorf("decode search records: %w", err)
		}
		out.Records = &records
	default:
		return crmcontracts.SearchDataAnswer{Outcome: crmcontracts.SearchDataAnswerOutcomeUnsupported}, nil
	}
	return out, nil
}

func (s Server) searchReporting(ctx context.Context, plan searchplan.Plan) (crmcontracts.ReportingEvaluation, error) {
	var selection crmcontracts.ReportingSelection
	if err := searchplan.DecodeArguments(plan.Arguments, &selection); err != nil {
		return crmcontracts.ReportingEvaluation{}, err
	}
	if len(selection.Metrics) != 1 || (plan.Count && selection.Metrics[0] != reportingBookingsWon) {
		return crmcontracts.ReportingEvaluation{}, httperr.Validation("metrics", string(crmcontracts.SearchDataAnswerOutcomeUnsupported), "ask for one metric; record counts use Sales won")
	}
	return s.reportMetrics.Evaluate(ctx, selection)
}

func requireSearchDataRead(ctx context.Context) error {
	if err := auth.Require(ctx, "installation_settings", principal.ActionRead); err != nil {
		return err
	}
	checks := []auth.Check{}
	for _, object := range []string{string(recordTypeDeal), string(recordTypeContact), string(recordTypeCompany), string(recordTypeLead), string(recordTypeProject), "report_definition"} {
		checks = append(checks, auth.Check{Object: object, Actions: []principal.Action{principal.ActionRead}})
	}
	return auth.RequireOneOf(ctx, checks...)
}
