// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// ReportDraftRequest is a saved report's next charts, prepared for a human to
// publish. With no report id, it is a new report only its author sees. It has
// no audience, so no call through it changes who sees a report.
type ReportDraftRequest struct {
	ReportID    *ids.UUID                       `json:"report_id,omitempty"`
	Name        string                          `json:"name,omitempty"`
	Description *string                         `json:"description,omitempty"`
	Selection   crmcontracts.ReportingSelection `json:"selection"`
}

// ReportDrafter saves the draft through the reporting service the Analytics
// screen uses, so the tool and the screen accept the same selections.
type ReportDrafter func(context.Context, ReportDraftRequest) (crmcontracts.ReportingReport, error)

// RegisterReportDraftTool adds draft_report, which change_reports fronts.
func RegisterReportDraftTool(registry *Registry, draft ReportDrafter) {
	registry.Register(draftReport{draft: draft})
}

type draftReport struct{ draft ReportDrafter }

func (t draftReport) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: draftReportTool, Title: "Draft a saved report",
		Version:     toolVersionV1,
		Description: draftReportCopy.render(),
		Instead:     draftReportCopy.Instead,
		// Auto-executed because nothing another reader sees moves: a draft
		// waits for a human to publish it, and a new report is private.
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		Unlisted:     true,
		OpenAPIOp:    "saveReportingReportDraft",
		InputSchema:  reportDraftInput,
		OutputSchema: schemaFor[crmcontracts.ReportingReport](),
	}
}

func (t draftReport) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var request ReportDraftRequest
	if err := decodeArgs(in, &request); err != nil {
		return nil, err
	}
	report, err := t.draft(ctx, request)
	if err != nil {
		return nil, err
	}
	return json.Marshal(report)
}

const draftReportTool = "draft_report"

var reportDraftInput = describedSchema[ReportDraftRequest](pipelineIDProvenance, windowRule)
