// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// ReportingRead selects a read operation over the same reporting engine as HTTP.
type ReportingRead struct {
	Scheduled         bool                              `json:"scheduled,omitempty"`
	Mode              string                            `json:"mode" enum:"catalog,evaluate,reports,report,editions,edition,evidence,compare"`
	Selection         crmcontracts.ReportingSelection   `json:"selection,omitempty"`
	ID                ids.UUID                          `json:"id,omitempty"`
	RightID           ids.UUID                          `json:"right_id,omitempty"`
	Reference         crmcontracts.ReportingEvidenceRef `json:"reference,omitempty"`
	EvaluationKey     string                            `json:"evaluation_key,omitempty"`
	EvaluatedAt       time.Time                         `json:"evaluated_at,omitempty"`
	FrameworkRevision int64                             `json:"framework_revision,omitempty"`
	Cursor            *string                           `json:"cursor,omitempty"`
	Limit             int                               `json:"limit,omitempty" minimum:"1" maximum:"100"`
}

// ReportingAnswer preserves typed results across the MCP boundary.
type ReportingAnswer struct {
	Catalog    *crmcontracts.ReportingCatalog     `json:"catalog,omitempty"`
	Evaluation *crmcontracts.ReportingEvaluation  `json:"evaluation,omitempty"`
	Reports    *crmcontracts.ReportingReportList  `json:"reports,omitempty"`
	Report     *crmcontracts.ReportingReport      `json:"report,omitempty"`
	Editions   *crmcontracts.ReportingEditionList `json:"editions,omitempty"`
	Edition    *crmcontracts.ReportingEdition     `json:"edition,omitempty"`
	Evidence   *crmcontracts.ReportingEvidence    `json:"evidence,omitempty"`
	Comparison *crmcontracts.ReportingComparison  `json:"comparison,omitempty"`
}

// ReportingReader injects the reporting service without a sibling-module dependency.
type ReportingReader func(context.Context, ReportingRead) (ReportingAnswer, error)

// RegisterReportingTool exposes the bounded reporting read surface to agents.
func RegisterReportingTool(registry *Registry, read ReportingReader) {
	registry.Register(readReporting{read: read})
}

type readReporting struct{ read ReportingReader }

var reportingReadCopy = toolCopy{
	Purpose: "Discover standard sales metrics, evaluate the same graphs as Analytics, or reopen saved report definitions and frozen editions.",
	Limits:  "Read only. Catalog returns permitted metric IDs and blocks. Evaluate requires a selection. Report and editions require a report id; edition requires an edition id. Evidence requires the evaluation receipt and selection, or an edition id, plus its exact reference. Compare requires two edition IDs. Always retain coverage and capture times when quoting numbers.",
	Instead: "Use list_pipelines to discover pipeline and stage IDs. Use run_analytics_query for custom groupings and compose_analytics_report for a document. Configure targets, report sharing and schedules in Analytics.",
}

func (t readReporting) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{Name: "read_reporting", Title: "Read sales reporting", Version: toolVersionV1, Description: reportingReadCopy.render(), Instead: reportingReadCopy.Instead, RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, OpenAPIOp: "evaluateReporting", InputSchema: reportingReadInput, OutputSchema: schema(`{"type":"object","properties":{"catalog":{"type":"object"},"evaluation":{"type":"object"},"reports":{"type":"object"},"report":{"type":"object"},"editions":{"type":"object"},"edition":{"type":"object"},"evidence":{"type":"object"},"comparison":{"type":"object"}},"additionalProperties":false}`)}
}

func (t readReporting) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var request ReportingRead
	if err := decodeArgs(in, &request); err != nil {
		return nil, err
	}
	if err := requireReportingIDs(request); err != nil {
		return nil, err
	}
	noteDerivedContent(ctx)
	answer, err := t.read(ctx, request)
	if err != nil {
		return nil, err
	}
	return json.Marshal(answer)
}

var (
	reportingReadInput   = describedSchema[ReportingRead](pipelineIDProvenance)
	reportingMetricInput = describedSchema[crmcontracts.ReportMetricReference](pipelineIDProvenance)
)

func describedSchema[T any](description string) json.RawMessage {
	shape, err := describeType(reflect.TypeFor[T]())
	if err != nil {
		//craft:ignore panic-in-domain Tool schemas are constructed at composition time; an unsupported static type must fail boot.
		panic(fmt.Sprintf("describe tool input: %v", err))
	}
	shape.Description = description
	raw, err := json.Marshal(shape)
	if err != nil {
		//craft:ignore panic-in-domain A schema encoding failure is a composition defect, before any request is admitted.
		panic(fmt.Sprintf("encode tool input: %v", err))
	}
	return raw
}
