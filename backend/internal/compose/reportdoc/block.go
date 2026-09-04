// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reportdoc

// The closed block grammar: what a report document may contain.
//
// Closed on purpose. An open grammar — "any block with a type string" — moves
// the decision about what may render onto whoever composes the document, which
// is the model. A renderer that meets an unknown block has two bad options,
// draw it or drop it silently, and both are worse than refusing the document
// before anybody reads it.

import "fmt"

// Kind names one block type.
type Kind string

// The blocks a report may carry. Each is here because a report needs it; a
// kind nobody renders is a kind that cannot be validated against what it
// renders as.
const (
	// Prose and structure. These carry words, never figures.
	KindTitle       Kind = "title"
	KindSubtitle    Kind = "subtitle"
	KindScope       Kind = "scope"
	KindGeneratedAt Kind = "generated_at"
	KindSummary     Kind = "summary"
	KindMethodology Kind = "methodology"
	KindFollowUps   Kind = "follow_ups"

	// Figures. Every one of these dereferences a cell.
	KindStatStrip   Kind = "stat_strip"
	KindBar         Kind = "bar"
	KindWaterfall   Kind = "waterfall"
	KindRankedList  Kind = "ranked_list"
	KindRecordTable Kind = "record_table"

	// What the reader is owed when a number is incomplete or missing.
	KindCallout        Kind = "callout"
	KindEvidenceDrawer Kind = "evidence_drawer"
)

// Severity types a callout. A callout is how a document says something the
// numbers cannot: that a figure is partial, that a question was unanswerable,
// that a grouping is unsupported. Typed rather than free prose so a renderer
// can style a warning differently from a note, and so a reader can tell an
// absence that was measured from one nobody looked for.
type Severity string

// The severities a callout may carry.
const (
	SeverityNote        Severity = "note"
	SeverityWarning     Severity = "warning"
	SeverityPartial     Severity = "partial"
	SeverityUnknown     Severity = "unknown"
	SeverityUnsupported Severity = "unsupported"
)

// Cell names one figure: a saved run and, within it, one cell's group keys.
//
// This is the ONLY way a number reaches a report. The document says where the
// figure lives; what it IS resolves at read time, under the reader's own
// grants, which is why two readers of one report can legitimately see
// different figures and one of them can see a refusal.
type Cell struct {
	// RunID is the saved run. A string rather than a parsed uuid because this
	// is wire-shaped input and a malformed one is a refusal with a message,
	// not a parse panic.
	RunID string `json:"run_id"`
	// Group binds the cell's keys, one per grouping in the saved question.
	// Empty for an ungrouped run, which has one cell.
	Group []any `json:"group,omitempty"`
	// Column names which measure of the cell to show. A cell can carry several
	// and a block shows one.
	Column string `json:"column"`
}

// Block is one element of a report.
//
// One struct rather than an interface per kind: the wire shape is JSON from a
// composer, so the type has to survive a round trip through a map anyway, and
// a dozen tiny types would each need their own decoder. Which fields are legal
// is decided by Kind, in validate.go.
type Block struct {
	Kind Kind `json:"kind"`
	// Text is the composer's own words. Always allowed to be prose; never
	// allowed to be a figure the document sourced itself.
	Text string `json:"text,omitempty"`
	// Cells are the figures this block shows, in render order.
	Cells []Cell `json:"cells,omitempty"`
	// Severity types a callout and is meaningless elsewhere.
	Severity Severity `json:"severity,omitempty"`
	// Value is the trap this grammar exists to spring.
	//
	// A composer that puts a number here is telling the renderer to draw a
	// figure the database never computed. It is refused ALWAYS — including,
	// and especially, when Cells is populated beside it: a block carrying both
	// renders the literal, the two can disagree, and nothing downstream can
	// tell that the page is showing a made-up number. The field exists so the
	// refusal can name what it found rather than silently dropping it.
	Value *float64 `json:"value,omitempty"`
}

// Document is a whole report.
type Document struct {
	Blocks []Block `json:"blocks"`
}

// String makes a Kind printable in a refusal without a cast at each site.
func (k Kind) String() string { return string(k) }

// carriesFigures says whether a kind's purpose is to show numbers.
//
// Used to require at least one cell of the blocks that exist to display a
// figure: a stat strip with nothing to show is a composer error that would
// render as an empty box, and an empty box beside real numbers reads as a
// figure of zero.
func (k Kind) carriesFigures() bool {
	switch k {
	case KindStatStrip, KindBar, KindWaterfall, KindRankedList, KindRecordTable:
		return true
	default:
		return false
	}
}

// known says whether a kind is in the grammar.
//
// Derived from allKinds, which is also what the published catalog iterates, so
// a kind added in one place is accepted and described in one edit. A switch
// here beside that list would be two copies of the grammar, disagreeing the
// first time a block was added.
func (k Kind) known() bool {
	for _, known := range allKinds {
		if k == known {
			return true
		}
	}
	return false
}

// known says whether a severity is in the closed set. Derived from
// allSeverities for the same reason Kind.known is derived from allKinds.
func (s Severity) known() bool {
	for _, known := range allSeverities {
		if s == known {
			return true
		}
	}
	return false
}

// blockRef names a block in a refusal so a composer can find it.
func blockRef(i int, k Kind) string {
	return fmt.Sprintf("block %d (%s)", i, k)
}
