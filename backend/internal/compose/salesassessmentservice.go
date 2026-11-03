// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/salesassessment"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The assessment's bounds. One model call may take this long, the next
// decision chooses among this many deals, and a written assessment is kept
// this long.
const (
	assessmentDeadline   = 20 * time.Second
	assessmentCandidates = 8
	assessmentDecisions  = 20
	assessmentKeepFor    = time.Hour
	assessmentKeepAtMost = 256
)

// salesAssessments writes the Sales review's assessment. The model reads the
// evaluation's measured cells, the deals behind the open pipeline and the
// recorded decisions. The reply is grounded, then kept per reader until the
// figures or the decisions move.
type salesAssessments struct {
	reports        *reporting.Service
	decisions      salesDecisions
	db             *database.DB
	lane           completer
	routingVersion string
	now            func() time.Time

	mu       sync.Mutex
	kept     map[string]keptAssessment
	inflight map[string]chan struct{}
}

type keptAssessment struct {
	at  time.Time
	out crmcontracts.SalesAssessment
}

func newSalesAssessments(reports *reporting.Service, decisions salesDecisions, db *database.DB) *salesAssessments {
	return &salesAssessments{reports: reports, decisions: decisions, db: db, now: time.Now, kept: map[string]keptAssessment{}, inflight: map[string]chan struct{}{}}
}

func (s *salesAssessments) withLane(lane completer, routingVersion string) *salesAssessments {
	s.lane, s.routingVersion = lane, routingVersion
	return s
}

// Assess answers the assessment of one selection for the reader. The
// evaluation is read under their own grants first, so a selection they may
// not read refuses before any model is asked. Refresh forces a new call.
func (s *salesAssessments) Assess(ctx context.Context, selection crmcontracts.ReportingSelection, refresh bool) (crmcontracts.SalesAssessment, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.SalesAssessment{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return crmcontracts.SalesAssessment{}, auth.RequireHuman(ctx)
	}
	evaluation, err := s.reports.Evaluate(ctx, selection)
	if err != nil {
		return crmcontracts.SalesAssessment{}, err
	}
	in, err := s.gather(ctx, selection, evaluation)
	if err != nil {
		return crmcontracts.SalesAssessment{}, err
	}
	lang := identity.BaseLanguageForPrompt(ctx, s.db.Pool())
	key := assessmentKey(actor.UserID.String(), lang, s.routingVersion, selection, in)
	if !refresh {
		if out, found := s.cached(key); found {
			return out, nil
		}
	}
	release, join := s.claim(key)
	if join != nil {
		<-join
		if out, found := s.cached(key); found {
			return out, nil
		}
	}
	defer release()
	out := s.write(ctx, evaluation, in, lang)
	if out.GeneratedBy == crmcontracts.WrittenByModel {
		s.keep(key, out)
	}
	return out, nil
}

// claim takes the key for this reader's call, or hands back the channel a
// call in flight closes when it is done. So concurrent readers of one key
// join one model call.
func (s *salesAssessments) claim(key string) (release func(), join chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pending, found := s.inflight[key]; found {
		return func() {}, pending
	}
	done := make(chan struct{})
	s.inflight[key] = done
	return func() {
		s.mu.Lock()
		delete(s.inflight, key)
		s.mu.Unlock()
		close(done)
	}, nil
}

func (s *salesAssessments) cached(key string) (crmcontracts.SalesAssessment, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept, found := s.kept[key]
	if !found || s.now().Sub(kept.at) > assessmentKeepFor {
		delete(s.kept, key)
		return crmcontracts.SalesAssessment{}, false
	}
	return kept.out, true
}

// keep stores a written assessment and lets the stale and the oldest go, so
// the map stays bounded whatever a day of reviews asked.
func (s *salesAssessments) keep(key string, out crmcontracts.SalesAssessment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for other, kept := range s.kept {
		if now.Sub(kept.at) > assessmentKeepFor {
			delete(s.kept, other)
		}
	}
	for len(s.kept) >= assessmentKeepAtMost {
		oldest, at := "", now
		for other, kept := range s.kept {
			if kept.at.Before(at) {
				oldest, at = other, kept.at
			}
		}
		delete(s.kept, oldest)
	}
	s.kept[key] = keptAssessment{at: now, out: out}
}

// write asks the model, or serves the deterministic floor: no sentences,
// and generated_by says so, so the page writes the summary it can.
func (s *salesAssessments) write(ctx context.Context, evaluation crmcontracts.ReportingEvaluation, in salesassessment.Input, lang string) crmcontracts.SalesAssessment {
	out := crmcontracts.SalesAssessment{
		EvaluationKey: evaluation.EvaluationKey,
		GeneratedAt:   s.now().UTC(),
		GeneratedBy:   crmcontracts.WrittenByDeterministic,
		Sentences:     []crmcontracts.SalesAssessmentSentence{},
		Cells:         assessmentCells(in),
		Decisions:     len(in.Decisions),
	}
	if s.lane == nil || len(in.Cells) == 0 {
		return out
	}
	laneCtx, cancel := context.WithTimeout(ctx, assessmentDeadline)
	defer cancel()
	res, err := ai.Ask(laneCtx, s.lane, salesassessment.Request(in, lang), func(reply string) error {
		_, err := salesassessment.Parse(reply, in)
		return err
	})
	if err != nil {
		slog.WarnContext(ctx, "the sales assessment fell back to the deterministic summary", "reason", err)
		return out
	}
	written, err := salesassessment.Parse(res.Text, in)
	if err != nil {
		slog.WarnContext(ctx, "the sales assessment fell back to the deterministic summary", "reason", err)
		return out
	}
	return foldAssessment(out, written, res.ServedModel)
}

func foldAssessment(out crmcontracts.SalesAssessment, written salesassessment.Written, served string) crmcontracts.SalesAssessment {
	out.GeneratedBy = crmcontracts.WrittenByModel
	for _, sentence := range written.Sentences {
		out.Sentences = append(out.Sentences, crmcontracts.SalesAssessmentSentence{Text: sentence.Text, Kind: crmcontracts.SalesAssessmentSentenceKind(sentence.Kind), Cells: sentence.Cells})
	}
	if written.Next != nil {
		deal, dealErr := ids.Parse(written.Next.DealID)
		owner, ownerErr := ids.Parse(written.Next.OwnerID)
		if dealErr == nil && ownerErr == nil {
			out.NextDecision = &crmcontracts.SalesAssessmentDecision{DealId: openapi_types.UUID(deal), OwnerId: openapi_types.UUID(owner), Text: written.Next.Text}
		}
	}
	confidence := crmcontracts.SalesAssessmentConfidence(written.Confidence)
	out.Confidence = &confidence
	if served != "" {
		out.ModelUsed = &served
	}
	return out
}

// assessmentKey is what one written assessment answers for: this reader,
// this selection, everything the model read, this prompt and this model
// binding. The evaluation's own instant is left out, so the same figures
// read a minute later are the same assessment.
func assessmentKey(reader, lang, routingVersion string, selection crmcontracts.ReportingSelection, in salesassessment.Input) string {
	hash := sha256.New()
	for _, part := range []string{reader, lang, routingVersion, salesassessment.PromptVersion} {
		hash.Write([]byte(part))
		hash.Write([]byte{0})
	}
	for _, read := range []any{selection, in} {
		encoded, err := json.Marshal(read)
		if err != nil {
			encoded = []byte(fmt.Sprint(read))
		}
		hash.Write(encoded)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// gather turns the evaluation into what the model reads. That is the cells
// with their figures as text, the deals behind the open pipeline the reader
// may see, and the decisions recorded on them.
func (s *salesAssessments) gather(ctx context.Context, selection crmcontracts.ReportingSelection, evaluation crmcontracts.ReportingEvaluation) (salesassessment.Input, error) {
	in := salesassessment.Input{Frame: assessmentFrame(evaluation.Context)}
	for _, metric := range evaluation.Metrics {
		in.Cells = append(in.Cells, assessmentCell("metric:"+string(metric.Id), string(metric.Id), metric.Value, metric.Unit, evaluation.Context.Interval, metric.Coverage))
	}
	for _, chart := range evaluation.Charts {
		if chart.Kind != reportingBreakdown {
			continue
		}
		for _, point := range chart.Points {
			in.Cells = append(in.Cells, assessmentCell("point:"+string(chart.Metric)+":"+point.Key, string(chart.Metric)+" · "+point.Label, point.Value, chart.Unit, evaluation.Context.Interval, chart.Coverage))
		}
	}
	candidates, err := s.candidates(ctx, selection, evaluation)
	if err != nil {
		return in, err
	}
	in.Candidates = candidates
	in.Decisions, err = s.recordedDecisions(ctx, candidates)
	return in, err
}

func assessmentFrame(context crmcontracts.ReportingContext) salesassessment.FrameIn {
	frame := salesassessment.FrameIn{
		Period:   context.PeriodKind,
		Window:   context.Interval.StartAt.Format(time.DateOnly) + " to " + context.Interval.EndAt.Format(time.DateOnly),
		Currency: context.Currency,
		Today:    context.EvaluatedAt.Format(time.DateOnly),
	}
	if context.Scope.Label != nil {
		frame.Scope = *context.Scope.Label
	}
	if context.ProductLabel != nil {
		frame.Product = *context.ProductLabel
	}
	if context.InitiativeLabel != nil {
		frame.Initiative = *context.InitiativeLabel
	}
	if context.Country != nil {
		frame.Country = string(*context.Country)
	}
	if context.Industry != nil {
		frame.Industry = string(*context.Industry)
	}
	return frame
}

func assessmentCell(id, label string, value *float64, unit string, window crmcontracts.ReportingWindow, coverage crmcontracts.ReportingCoverage) salesassessment.CellIn {
	cell := salesassessment.CellIn{ID: id, Label: label, Unit: unit, Coverage: string(coverage.Status), Window: window.StartAt.Format(time.DateOnly) + " to " + window.EndAt.Format(time.DateOnly)}
	if coverage.Withheld {
		cell.Coverage = "withheld"
	}
	if value != nil {
		cell.Value = figureText(*value, unit)
	} else {
		cell.Value = "not measured"
	}
	return cell
}

// figureText writes a figure the way the page prints it: money in major
// units, a count or rate as it is.
func figureText(value float64, unit string) string {
	if unit != "" && unit != "count" && unit != "ratio" && unit != "percent" && unit != "days" {
		return strconv.FormatFloat(value/100, 'f', 0, 64) + " " + unit
	}
	return strconv.FormatFloat(value, 'f', -1, 64) + " " + unit
}

// candidates are the open pipeline's largest deals the reader may see, read
// through the same evidence door the chart's drill-through uses.
func (s *salesAssessments) candidates(ctx context.Context, selection crmcontracts.ReportingSelection, evaluation crmcontracts.ReportingEvaluation) ([]salesassessment.CandidateIn, error) {
	var hasOpen bool
	for _, metric := range evaluation.Metrics {
		if metric.Id == reportingOpenPipeline {
			hasOpen = metric.Coverage.Status != reportingFailed
		}
	}
	if !hasOpen {
		return nil, nil
	}
	expectation := reporting.EvidenceExpectation{Key: evaluation.EvaluationKey, At: evaluation.Context.EvaluatedAt, FrameworkRevision: evaluation.Context.FrameworkRevision}
	evidence, err := s.reports.Evidence(ctx, selection, reportingOpenPipeline, "", "", nil, nil, assessmentCandidates, expectation)
	if err != nil {
		return nil, fmt.Errorf("read the deals behind the open pipeline: %w", err)
	}
	out := make([]salesassessment.CandidateIn, 0, len(evidence.Rows))
	for _, row := range evidence.Rows {
		if row.Restricted || row.SourceId == nil || row.OwnerId == nil {
			continue
		}
		candidate := salesassessment.CandidateIn{ID: row.SourceId.String(), Name: row.Label, OwnerID: row.OwnerId.String()}
		if row.Value != nil {
			candidate.Amount = figureText(*row.Value, evaluation.Context.Currency)
		}
		out = append(out, candidate)
	}
	return out, nil
}

// recordedDecisions are the decisions on the candidate deals, newest first,
// each with its outcome when one was recorded. Read per deal, so a busy
// workspace's other decisions never push a candidate's own out of view.
func (s *salesAssessments) recordedDecisions(ctx context.Context, candidates []salesassessment.CandidateIn) ([]salesassessment.DecisionIn, error) {
	out := []salesassessment.DecisionIn{}
	for _, candidate := range candidates {
		dealID, err := ids.Parse(candidate.ID)
		if err != nil {
			return nil, fmt.Errorf("a candidate deal id: %w", err)
		}
		listed, err := s.decisions.List(ctx, &dealID, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, decisionsIn(listed, assessmentDecisions-len(out))...)
	}
	return out, nil
}

func decisionsIn(listed []crmcontracts.SalesDecision, room int) []salesassessment.DecisionIn {
	out := []salesassessment.DecisionIn{}
	for _, decision := range listed {
		if len(out) == room {
			break
		}
		in := salesassessment.DecisionIn{ID: decision.Id.String(), Text: decision.Decision, Deal: decision.DealId.String(), Status: string(decision.Status)}
		if decision.OwnerId != nil {
			in.OwnerID = decision.OwnerId.String()
		}
		if decision.ReviewOn != nil {
			in.ReviewOn = decision.ReviewOn.Format(time.DateOnly)
		}
		if decision.Result != nil {
			in.Result = *decision.Result
		}
		out = append(out, in)
	}
	return out
}

func assessmentCells(in salesassessment.Input) []crmcontracts.SalesAssessmentCell {
	out := make([]crmcontracts.SalesAssessmentCell, 0, len(in.Cells))
	for _, cell := range in.Cells {
		out = append(out, crmcontracts.SalesAssessmentCell{Id: cell.ID, Label: cell.Label, Value: cell.Value})
	}
	return out
}
