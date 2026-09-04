// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// `margince://schema/report-blocks` — the block grammar compose_analytics_report
// accepts, published as a document rather than recited into that tool's own
// description.
//
// The same move the report plan vocabulary made, for the same measured reason:
// fourteen block kinds with their fields is text every client holds for a whole
// session and every scheduled run re-sends on every step, to answer a question
// one call asks once. What stays in the tool is the rule a caller gets wrong —
// never write a number — because that one is not lookup-able, it is a habit.
//
// This is not schema deferral. Everything deciding whether a call is
// WELL-FORMED stays in the tool's own input schema; what moves is which KIND
// names that schema's open `kind` string admits, and it moves because the
// refusal path is loud: an unknown kind is refused by name with the whole set.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// ReportBlocksURI is the document's stable identity.
const ReportBlocksURI = "margince://schema/report-blocks"

// reportBlocksResourceName is the catalogue entry's name member.
const reportBlocksResourceName = "report_blocks"

// reportBlocksVersion identifies the document's SHAPE, not its content. A
// caller caching it needs to know when the shape changed; the grammar moves on
// its own schedule and is re-read either way.
const reportBlocksVersion = "1"

// reportBlocksNotation states the one rule a composer gets wrong, because it is
// the rule no field list can carry: a figure is never written, only cited.
const reportBlocksNotation = "A block carries STRUCTURE and WORDS; a figure is named, never written. " +
	"Every number cites a saved run and a cell inside it, and the server resolves that citation " +
	"under the reading caller's own authority. A block carrying a literal number is refused EVEN " +
	"WHEN a valid citation sits beside it: the literal is what would render, the two can " +
	"disagree, and no reader could tell the page shows a figure the database never computed."

// ReportBlocksReader is the seam the describe tool reads through.
type ReportBlocksReader interface {
	ReportBlocksDocument(ctx context.Context) (json.RawMessage, error)
}

// BlockGrammar is the grammar as the owning package describes it.
//
// An interface rather than the concrete type, because reportdoc lives in
// compose and this module may not import it — the composition root passes the
// description in, the same way the report catalog is passed to the vocabulary
// resource.
type BlockGrammar struct {
	Blocks     []BlockDescription `json:"blocks"`
	Severities []string           `json:"severities"`
}

// BlockDescription is one block kind, as a composer needs to know it.
type BlockDescription struct {
	Kind          string `json:"kind"`
	TakesCells    bool   `json:"takes_cells"`
	TakesText     bool   `json:"takes_text"`
	TakesSeverity bool   `json:"takes_severity"`
}

// ReportBlocksResource publishes the block grammar.
type ReportBlocksResource struct{ grammar BlockGrammar }

// NewReportBlocksResource binds the document to the grammar it describes.
func NewReportBlocksResource(grammar BlockGrammar) ReportBlocksResource {
	return ReportBlocksResource{grammar: grammar}
}

// Resources advertises the one document this provider publishes.
func (ReportBlocksResource) Resources(context.Context) []mcp.Resource {
	return []mcp.Resource{{
		URI:   ReportBlocksURI,
		Name:  reportBlocksResourceName,
		Title: "Report block grammar",
		// ScopeRead, matching compose_analytics_report: a caller admitted to
		// the tool is admitted to the grammar it refuses against.
		RequiredScope: principal.ScopeRead,
		MIMEType:      mimeApplicationJSON,
		// Says what it HOLDS, not when to fetch it. A description that orders a
		// read draws reads from runs with nothing to compose.
		Description: "The blocks a report may carry: each kind, whether it renders figures, " +
			"words or both, and the severities a callout may state. " +
			"compose_analytics_report names this document instead of carrying it.",
	}}
}

// ReadResource composes the document. An unknown URI answers ErrNotFound,
// matching every other read on this surface.
func (r ReportBlocksResource) ReadResource(ctx context.Context, uri string) (mcp.ResourceContents, error) {
	if uri != ReportBlocksURI {
		return mcp.ResourceContents{}, fmt.Errorf("agents: resource %q: %w", uri, apperrors.ErrNotFound)
	}
	body, err := r.ReportBlocksDocument(ctx)
	if err != nil {
		return mcp.ResourceContents{}, err
	}
	return mcp.ResourceContents{URI: uri, MIMEType: mimeApplicationJSON, Text: string(body)}, nil
}

// ReportBlocksDocument composes the published document.
//
// Held by: TestTheBlocksResourceAndTheSeamServeTheSameBytes
// (internal/modules/agents/reportblocksdoc_test.go)
//
// The resource read above and describe_report_blocks both call THIS, so the two
// doors serve one byte sequence rather than two renderings that agree today.
func (r ReportBlocksResource) ReportBlocksDocument(context.Context) (json.RawMessage, error) {
	body, err := json.Marshal(reportBlocksDoc{
		Version:    reportBlocksVersion,
		Notation:   reportBlocksNotation,
		Blocks:     r.grammar.Blocks,
		Severities: r.grammar.Severities,
	})
	if err != nil {
		return nil, fmt.Errorf("agents: rendering the report block grammar: %w", err)
	}
	return body, nil
}

var (
	_ mcp.ResourceProvider = ReportBlocksResource{}
	_ ReportBlocksReader   = ReportBlocksResource{}
)

// reportBlocksDoc is the published shape.
type reportBlocksDoc struct {
	Version    string             `json:"version"`
	Notation   string             `json:"notation"`
	Blocks     []BlockDescription `json:"blocks"`
	Severities []string           `json:"severities"`
}
