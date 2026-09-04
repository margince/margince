// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reportdoc

// What a report is allowed to say, and the one thing it may never say: a
// number of its own.

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// aRun is a syntactically valid run id. Whether it EXISTS is the caller's
// question, not the grammar's — validation is structural on purpose.
var aRun = ids.NewV7().String()

func cell() Cell { return Cell{RunID: aRun, Column: "n"} }

// A figure comes from a handle, and a document of handles is well formed.
func TestADocumentWhoseFiguresAllComeFromHandlesIsAccepted(t *testing.T) {
	runs, err := Validate(Document{Blocks: []Block{
		{Kind: KindTitle, Text: "Pipeline this quarter"},
		{Kind: KindStatStrip, Cells: []Cell{cell()}},
		{Kind: KindMethodology, Text: "Open deals, expected close inside the quarter."},
	}})
	if err != nil {
		t.Fatalf("a document citing only handles was refused: %v", err)
	}
	if len(runs) != 1 {
		t.Errorf("the document cites one run and Validate reported %d", len(runs))
	}
}

// THE RULE. A literal beside a valid handle is refused, and the refusal says
// why this case is worse than a literal alone.
//
// The plan states it directly: "a test supplies a correct-looking literal
// alongside a handle and expects refusal". Correct-looking is the point — a
// wrong number would be caught by a reader, and a plausible one would not.
func TestALiteralIsRefusedEvenBesideAValidHandle(t *testing.T) {
	plausible := 42.0
	_, err := Validate(Document{Blocks: []Block{
		{Kind: KindTitle, Text: "Pipeline"},
		{Kind: KindStatStrip, Cells: []Cell{cell()}, Value: &plausible},
	}})
	if err == nil {
		t.Fatal("a block carrying both a literal and a handle was accepted: the literal is " +
			"what renders, so the page would show a number the database never computed")
	}
	if !IsInvalid(err) {
		t.Fatalf("the refusal is %v and should be a grammar refusal", err)
	}
	if !strings.Contains(err.Error(), "BOTH") {
		t.Errorf("the refusal does not tell the composer that carrying both is the "+
			"problem: %s", err)
	}
}

// And a literal alone is refused too, with the plainer reason.
func TestALiteralWithNoHandleIsRefused(t *testing.T) {
	n := 42.0
	_, err := Validate(Document{Blocks: []Block{
		{Kind: KindStatStrip, Value: &n},
	}})
	if !IsInvalid(err) {
		t.Fatalf("a block carrying a bare literal was accepted: %v", err)
	}
}

// A refusal maps like every other malformed input.
func TestAGrammarRefusalIsAnInvalidArgument(t *testing.T) {
	n := 1.0
	_, err := Validate(Document{Blocks: []Block{{Kind: KindStatStrip, Value: &n}}})
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Errorf("a grammar refusal is %v and should unwrap to the invalid-argument "+
			"sentinel, so the HTTP layer maps it like every other bad input", err)
	}
}

// An unknown block is refused rather than dropped.
func TestAnUnknownBlockIsRefused(t *testing.T) {
	_, err := Validate(Document{Blocks: []Block{{Kind: "freeform_html", Text: "<b>hi</b>"}}})
	if !IsInvalid(err) {
		t.Fatalf("an unknown block kind was accepted: %v", err)
	}
}

// A figure block with no figure would render an empty frame.
func TestAFigureBlockThatNamesNoCellIsRefused(t *testing.T) {
	for _, k := range []Kind{KindStatStrip, KindBar, KindWaterfall, KindRankedList, KindRecordTable} {
		if _, err := Validate(Document{Blocks: []Block{{Kind: k}}}); !IsInvalid(err) {
			t.Errorf("%s with no cell was accepted: an empty frame beside real numbers "+
				"reads as a figure of zero", k)
		}
	}
}

// A cell on a block that renders none is a number nobody would see.
func TestACellOnABlockThatRendersNoFiguresIsRefused(t *testing.T) {
	if _, err := Validate(Document{Blocks: []Block{
		{Kind: KindTitle, Text: "Q3", Cells: []Cell{cell()}},
	}}); !IsInvalid(err) {
		t.Error("a title carrying a cell was accepted: the figure would be silently unshown")
	}
}

// A callout states its kind.
func TestAnUntypedCalloutIsRefused(t *testing.T) {
	if _, err := Validate(Document{Blocks: []Block{
		{Kind: KindCallout, Text: "Some deals lack a close date."},
	}}); !IsInvalid(err) {
		t.Error("an untyped callout was accepted: it renders as prose, and a measured " +
			"absence then reads the same as one nobody looked for")
	}
	if _, err := Validate(Document{Blocks: []Block{
		{Kind: KindCallout, Text: "x", Severity: "catastrophe"},
	}}); !IsInvalid(err) {
		t.Error("a callout with an unknown severity was accepted")
	}
}

// Severity belongs to a callout and nowhere else.
func TestSeverityOnANonCalloutIsRefused(t *testing.T) {
	if _, err := Validate(Document{Blocks: []Block{
		{Kind: KindTitle, Text: "Q3", Severity: SeverityWarning},
	}}); !IsInvalid(err) {
		t.Error("a severity on a title was accepted")
	}
}

// A cell names a run and a column.
func TestACellWithoutARunOrAColumnIsRefused(t *testing.T) {
	if _, err := Validate(Document{Blocks: []Block{
		{Kind: KindStatStrip, Cells: []Cell{{RunID: "not-a-uuid", Column: "n"}}},
	}}); !IsInvalid(err) {
		t.Error("a cell naming no readable run was accepted")
	}
	if _, err := Validate(Document{Blocks: []Block{
		{Kind: KindStatStrip, Cells: []Cell{{RunID: aRun, Column: "  "}}},
	}}); !IsInvalid(err) {
		t.Error("a cell naming no column was accepted: a renderer would pick one")
	}
}

// One run cited twice is reported once, in citation order.
func TestTheCitedRunsAreDistinctAndOrdered(t *testing.T) {
	second := ids.NewV7().String()
	runs, err := Validate(Document{Blocks: []Block{
		{Kind: KindStatStrip, Cells: []Cell{cell(), {RunID: second, Column: "n"}}},
		{Kind: KindBar, Cells: []Cell{cell()}},
	}})
	if err != nil {
		t.Fatalf("a document citing two runs was refused: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("two distinct runs were cited and Validate reported %d", len(runs))
	}
	if runs[0].String() != aRun || runs[1].String() != second {
		t.Error("the cited runs came back out of citation order, so two identical " +
			"documents would produce different refusal messages")
	}
}

// An empty document is refused, and so is an oversized one.
func TestADocumentMustHaveBlocksAndNotTooMany(t *testing.T) {
	if _, err := Validate(Document{}); !IsInvalid(err) {
		t.Error("a document with no blocks was accepted: it renders as an empty page")
	}
	many := make([]Block, MaxBlocks+1)
	for i := range many {
		many[i] = Block{Kind: KindTitle, Text: "x"}
	}
	if _, err := Validate(Document{Blocks: many}); !IsInvalid(err) {
		t.Errorf("a document of %d blocks was accepted", len(many))
	}
}

// A document of pure prose cites nothing, and that is not an error.
func TestAProseOnlyDocumentIsAcceptedAndCitesNothing(t *testing.T) {
	runs, err := Validate(Document{Blocks: []Block{
		{Kind: KindTitle, Text: "Nothing closed this week"},
		{Kind: KindSummary, Text: "No deal moved stage."},
	}})
	if err != nil {
		t.Fatalf("a prose-only report was refused: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("a report with no figures cited %d runs", len(runs))
	}
}
