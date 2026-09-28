// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// search_report_evidence (🟢): the records behind a saved run, searched for
// the evidence of a claim about them.
//
// The run is the cohort handle. Its id names one question, and the question
// names one set of records; the engine re-derives that set under this caller's
// live grants, exactly as the report drawer does, and searches nothing else.
// Every figure in the answer is over the records this caller can read — never
// over rows they cannot, whose presence the answer must not betray — and a
// share is stated only when every one of those was judged.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/retrieval"
)

// ReportEvidenceQuery is one search over a saved run's records.
type ReportEvidenceQuery struct {
	RunID ids.UUID
	// Cell is one cell's group key values; nil searches every cell the run's
	// answer serves.
	Cell  *[]any
	Text  string
	Limit int
}

// ReportEvidence is the engine's answer: every record of the set the caller
// reached, sorted into what the text matched, what it did not, and what could
// not be judged — plus what kept any of the set out of reach.
type ReportEvidence struct {
	Entity datasource.EntityType
	// Citations are the matched records, strongest first.
	Citations       []retrieval.Hit
	Counterexamples []ids.UUID
	Abstentions     []ids.UUID
	SemanticRanking bool
	// Withheld: the privacy floor withholds the cell, so nothing was searched.
	Withheld bool
	// PartlyWithheld: without a cell, the cells the floor withheld were left out.
	PartlyWithheld bool
	// Truncated: the readable set is larger than one drill-through reads.
	Truncated bool
}

// ReportEvidenceSearcher answers one search over a saved run's records.
type ReportEvidenceSearcher func(ctx context.Context, q ReportEvidenceQuery) (ReportEvidence, error)

// The notes this tool's coverage can carry, beside the shared ones.
const (
	CodeCellWithheld      = "cell_withheld"
	CodeCellsWithheld     = "cells_withheld"
	CodeCohortTruncated   = "cohort_truncated"
	CodeRecordsUnjudged   = "records_unjudged"
	CodePrevalenceRefused = "prevalence_refused"
	// CodeOverReadableRecords is on every answer: its figures count the
	// records this caller can read, which may be fewer than the run counted.
	CodeOverReadableRecords = "prevalence_over_readable_records"
)

// RegisterReportEvidenceTool adds search_report_evidence once a searcher
// exists, the conditional registration the other injected-engine tools take.
func RegisterReportEvidenceTool(r *Registry, p datasource.SystemOfRecordProvider, search ReportEvidenceSearcher) {
	if search == nil {
		return
	}
	r.Register(searchReportEvidence{hydrator: searchContext{p: p}, search: search})
}

type searchReportEvidence struct {
	// hydrator is search_context's read-back, used for its seam read alone.
	hydrator searchContext
	search   ReportEvidenceSearcher
}

func (t searchReportEvidence) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "search_report_evidence", Title: "Search the evidence behind a saved run",
		Version:       toolVersionV1,
		Description:   searchReportEvidenceCopy.render(),
		Instead:       searchReportEvidenceCopy.Instead,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		InputSchema: schema(`{"type":"object","required":["run_id","query"],"properties":{
			"run_id":{"type":"string","format":"uuid","description":"A saved run: run_analytics_query with save answers one."},
			"cell":{"type":"array","items":{},"description":"One cell's group key values, in the run's group_by order. Omit to search every record the run measured."},
			"query":{"type":"string","maxLength":1000,"description":"The words the evidence would carry."},
			"limit":{"type":"integer","minimum":1,"maximum":25,"description":"How many citations, counterexamples and abstentions to return, each."}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[SearchReportEvidenceResult](),
	}
}

// CoverageClasses: a set searched whole, or a part of one. Never
// ranked_semantic — the ranking orders the citations, the classification
// decides them.
func (t searchReportEvidence) CoverageClasses() []string {
	return []string{CoverageCompleteExact, CoveragePartialDegraded}
}

// SearchReportEvidenceResult is what search_report_evidence answers.
type SearchReportEvidenceResult struct {
	Citations       []SearchContextHit `json:"citations"`
	Counterexamples []wireRecord       `json:"counterexamples"`
	// Abstentions name records the text could not be judged against.
	Abstentions []EvidenceAbstention `json:"abstentions"`
	Tally       EvidenceTally        `json:"tally"`
	// Coverage is complete_exact only when every readable record of the set
	// was judged. Not omitempty, for search_context's reason.
	Coverage string `json:"coverage"`
	// Prevalence is null unless coverage is complete_exact; a note says why.
	Prevalence *EvidencePrevalence `json:"prevalence"`
	Notes      []QueryNote         `json:"notes"`
}

// EvidenceAbstention is a record that matched neither way.
type EvidenceAbstention struct {
	RecordType string   `json:"record_type"`
	ID         ids.UUID `json:"id"`
}

// EvidenceTally counts the readable records, by verdict.
type EvidenceTally struct {
	Matched   int `json:"matched"`
	Unmatched int `json:"unmatched"`
	Unjudged  int `json:"unjudged"`
}

// EvidencePrevalence is the share of the readable records the text matched;
// Of counts those records.
type EvidencePrevalence struct {
	Matched int     `json:"matched"`
	Of      int     `json:"of"`
	Share   float64 `json:"share"`
}

func (t searchReportEvidence) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		RunID ids.UUID `json:"run_id"`
		Cell  *[]any   `json:"cell"`
		Query string   `json:"query"`
		Limit int      `json:"limit"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" {
		return nil, &BadArgsError{Field: "query", Cause: errors.New(
			"`query` is required and takes the words the evidence would carry")}
	}
	if n := len([]rune(args.Query)); n > contextSearchMaxQueryRunes {
		return nil, &BadArgsError{Field: "query", Cause: fmt.Errorf(
			"`query` takes at most %d characters and this one carries %d", contextSearchMaxQueryRunes, n)}
	}
	limit := contextSearchLimit(args.Limit)
	found, err := t.search(ctx, ReportEvidenceQuery{
		RunID: args.RunID, Cell: args.Cell, Text: args.Query, Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	result, err := t.answer(ctx, found, limit)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// answer reads the listed records back and decides coverage and prevalence.
//
// Each listed record is read again because the world moves between the search
// and the read: one whose content this caller may no longer read is dropped
// with its verdict and its excerpt, and the tally loses it too.
func (t searchReportEvidence) answer(ctx context.Context, found ReportEvidence, limit int) (SearchReportEvidenceResult, error) {
	result := SearchReportEvidenceResult{
		Citations:       make([]SearchContextHit, 0, min(limit, len(found.Citations))),
		Counterexamples: make([]wireRecord, 0, min(limit, len(found.Counterexamples))),
		Abstentions:     make([]EvidenceAbstention, 0, min(limit, len(found.Abstentions))),
		Tally: EvidenceTally{
			Matched: len(found.Citations), Unmatched: len(found.Counterexamples), Unjudged: len(found.Abstentions),
		},
		Notes: gapNotes(found),
	}
	dropped := false
	for _, hit := range found.Citations[:min(limit, len(found.Citations))] {
		record, readable, err := t.hydrator.read(ctx, hit.Ref)
		if err != nil {
			return SearchReportEvidenceResult{}, err
		}
		if !readable {
			dropped, result.Tally.Matched = true, result.Tally.Matched-1
			continue
		}
		result.Citations = append(result.Citations, SearchContextHit{
			Record: newWireRecord(ctx, record), Score: hit.Score, Excerpts: excerptsOf(hit.Evidence),
		})
	}
	for _, id := range found.Counterexamples[:min(limit, len(found.Counterexamples))] {
		record, readable, err := t.hydrator.read(ctx, datasource.EntityRef{Type: found.Entity, ID: id})
		if err != nil {
			return SearchReportEvidenceResult{}, err
		}
		if !readable {
			dropped, result.Tally.Unmatched = true, result.Tally.Unmatched-1
			continue
		}
		result.Counterexamples = append(result.Counterexamples, newWireRecord(ctx, record))
	}
	for _, id := range found.Abstentions[:min(limit, len(found.Abstentions))] {
		record, readable, err := t.hydrator.read(ctx, datasource.EntityRef{Type: found.Entity, ID: id})
		if err != nil {
			return SearchReportEvidenceResult{}, err
		}
		if !readable {
			dropped, result.Tally.Unjudged = true, result.Tally.Unjudged-1
			continue
		}
		// Only the id is served, but it is a read like any other and is
		// charged and sourced as one.
		noteRecord(ctx, record)
		result.Abstentions = append(result.Abstentions, EvidenceAbstention{RecordType: string(found.Entity), ID: id})
	}
	if dropped {
		result.Notes = append(result.Notes, QueryNote{
			Code:   CodeRowUnreadable,
			Detail: "at least one record could not be read back when the answer was assembled and is not counted",
		})
	}
	return withPrevalence(result, searchedWhole(found, dropped)), nil
}

// searchedWhole says every readable record of the set was judged — the one
// condition under which a share of them may be stated.
func searchedWhole(found ReportEvidence, dropped bool) bool {
	return !found.Withheld && !found.PartlyWithheld && !found.Truncated &&
		len(found.Abstentions) == 0 && !dropped
}

// gapNotes names what the answer counts over, and every reason the readable
// set was not searched whole.
func gapNotes(found ReportEvidence) []QueryNote {
	notes := []QueryNote{{
		Code: CodeOverReadableRecords,
		Detail: "every figure here counts the records you can read today; the run may have " +
			"counted records you cannot, and nothing here says whether it did",
	}}
	add := func(on bool, code, detail string) {
		if on {
			notes = append(notes, QueryNote{Code: code, Detail: detail})
		}
	}
	add(found.Withheld, CodeCellWithheld,
		"the privacy floor withholds this cell, so none of its records were searched")
	add(found.PartlyWithheld, CodeCellsWithheld,
		"the privacy floor withholds some of this run's cells, and their records were not searched")
	add(found.Truncated, CodeCohortTruncated,
		"the set is larger than one search reads, and only its first records were searched")
	add(len(found.Abstentions) > 0, CodeRecordsUnjudged,
		"some records carry no text the search can judge; they are listed as abstentions")
	add(!found.SemanticRanking && len(found.Citations) > 0, CodeSemanticRankingDegraded,
		"the citations are ordered by word overlap, not meaning; which records match is unaffected")
	return notes
}

// withPrevalence states the share only over a readable set searched whole.
func withPrevalence(result SearchReportEvidenceResult, whole bool) SearchReportEvidenceResult {
	judged := result.Tally.Matched + result.Tally.Unmatched
	if !whole || judged == 0 {
		result.Coverage = CoveragePartialDegraded
		result.Notes = append(result.Notes, QueryNote{
			Code: CodePrevalenceRefused,
			Detail: "no share is stated: the notes above name the readable records that were not " +
				"judged, and a share of part of them is not a share of all of them",
		})
		return result
	}
	result.Coverage = CoverageCompleteExact
	result.Prevalence = &EvidencePrevalence{
		Matched: result.Tally.Matched, Of: judged,
		Share: float64(result.Tally.Matched) / float64(judged),
	}
	return result
}
