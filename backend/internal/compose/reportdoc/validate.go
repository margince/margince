// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reportdoc

// Deciding whether a document may be rendered.
//
// Every rule here answers one question: can a reader trust that the figures on
// this page came from the database? A document that fails is refused whole
// rather than rendered with the offending block dropped, because a report
// missing a block it was composed with says something different from the
// report that was composed.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// InvalidError is a document refused, naming the block and what was wrong.
//
// It unwraps to ErrInvalidArgument, so the HTTP layer maps it the way it maps
// every other malformed input, and a composer reading the message can find the
// block by index.
type InvalidError struct {
	Where  string
	Reason string
}

func (e *InvalidError) Error() string { return e.Where + ": " + e.Reason }

// Unwrap ties this to the fixed sentinel registry rather than to a status code
// spelled here. A refusal that picked its own code would drift from every
// other invalid input the moment the registry changed.
func (e *InvalidError) Unwrap() error { return apperrors.ErrInvalidArgument }

// MaxBlocks bounds a document.
//
// Not a performance limit: a report is something a person reads, and a
// thousand-block document is a dump wearing a report's name. The bound is here
// so the refusal says so rather than a renderer discovering it.
const MaxBlocks = 200

// Validate refuses a document that a reader could not trust.
//
// Returns the distinct run ids the document cites, so the caller can check
// they exist and are readable in one pass instead of once per block. An empty
// slice is a legitimate answer: a document of pure prose cites nothing, and
// nothing about that is wrong — it simply has no figures to get wrong.
func Validate(doc Document) ([]ids.UUID, error) {
	if len(doc.Blocks) == 0 {
		return nil, &InvalidError{
			Where:  "document",
			Reason: "a report with no blocks renders as an empty page",
		}
	}
	if len(doc.Blocks) > MaxBlocks {
		return nil, &InvalidError{
			Where: "document",
			Reason: fmt.Sprintf("a report carries at most %d blocks and this one has %d",
				MaxBlocks, len(doc.Blocks)),
		}
	}

	// Ordered, not a bare set: the caller resolves these and reports which
	// failed, and a map's iteration order would make that message differ
	// between two identical documents.
	var runIDs []ids.UUID
	seen := map[ids.UUID]bool{}

	for i, b := range doc.Blocks {
		where := blockRef(i, b.Kind)
		if !b.Kind.known() {
			return nil, &InvalidError{
				Where: where,
				// The whole set, so a caller that guessed pays one refusal
				// rather than a lookup — which is what lets the tool
				// description name the vocabulary document without ordering a
				// read of it.
				Reason: "no such block kind. A renderer meeting one either draws something " +
					"nobody specified or drops it silently, and a report missing a block it " +
					"was composed with says something different from the one composed. The " +
					"kinds are: " + strings.Join(KindNames(), ", "),
			}
		}
		if err := checkNoLiteral(b, where); err != nil {
			return nil, err
		}
		if err := checkSeverity(b, where); err != nil {
			return nil, err
		}
		if err := checkFigures(b, where); err != nil {
			return nil, err
		}
		for j, c := range b.Cells {
			id, err := checkCell(c, fmt.Sprintf("%s cell %d", where, j))
			if err != nil {
				return nil, err
			}
			if !seen[id] {
				seen[id] = true
				runIDs = append(runIDs, id)
			}
		}
	}
	return runIDs, nil
}

// checkNoLiteral is the rule the package exists for.
//
// A literal is refused whether or not a handle sits beside it, and the
// both-present case is the one that matters: it renders the literal, the two
// can disagree, and no reader can tell the page is showing a number the
// database never computed. Refusing only the literal-alone case would admit
// exactly that.
func checkNoLiteral(b Block, where string) error {
	if b.Value == nil {
		return nil
	}
	reason := "a block may not carry a number of its own — every figure is dereferenced " +
		"from a saved run's cell, so that what a reader sees is what the database computed"
	if len(b.Cells) > 0 {
		reason = "this block carries BOTH a literal number and a cell handle. That is worse " +
			"than a literal alone: the literal is what renders, the two can disagree, and " +
			"nothing downstream can tell the page is showing a figure the database never " +
			"computed. Remove the literal and keep the handle"
	}
	return &InvalidError{Where: where, Reason: reason}
}

// checkSeverity keeps the typed-callout vocabulary closed.
//
// A callout is how a document reports what the numbers cannot — a partial
// figure, an unanswerable question, an unsupported grouping. An untyped one
// would render as ordinary prose, which is how a measured absence becomes
// indistinguishable from one nobody looked for.
func checkSeverity(b Block, where string) error {
	if b.Kind != KindCallout {
		if b.Severity != "" {
			return &InvalidError{
				Where:  where,
				Reason: "only a callout carries a severity",
			}
		}
		return nil
	}
	if b.Severity == "" {
		return &InvalidError{
			Where: where,
			Reason: "a callout states its kind — an untyped one renders as prose, and a " +
				"measured absence then reads the same as one nobody looked for",
		}
	}
	if !b.Severity.known() {
		return &InvalidError{
			Where:  where,
			Reason: fmt.Sprintf("no such severity %q", b.Severity),
		}
	}
	return nil
}

// checkFigures requires a figure block to have a figure, and forbids one on a
// block that renders none.
//
// The second half is not tidiness. A cell on a title is a number the composer
// meant to show somewhere, silently not shown — and a report missing a figure
// somebody put in it is a report that reads as complete while saying less.
func checkFigures(b Block, where string) error {
	if b.Kind.carriesFigures() {
		if len(b.Cells) == 0 {
			return &InvalidError{
				Where: where,
				Reason: "this block exists to show a figure and names none — it would render " +
					"as an empty frame, which beside real numbers reads as a figure of zero",
			}
		}
		return nil
	}
	if len(b.Cells) > 0 && b.Kind != KindSummary && b.Kind != KindEvidenceDrawer {
		return &InvalidError{
			Where: where,
			Reason: "this block renders no figures, so a cell on it is a number the composer " +
				"meant to show and nobody would see",
		}
	}
	return nil
}

// checkCell validates one handle and returns the run it names.
func checkCell(c Cell, where string) (ids.UUID, error) {
	id, err := ids.Parse(c.RunID)
	if err != nil {
		return ids.UUID{}, &InvalidError{
			Where:  where,
			Reason: "names no readable run id",
		}
	}
	if strings.TrimSpace(c.Column) == "" {
		return ids.UUID{}, &InvalidError{
			Where: where,
			Reason: "names no column — a cell can carry several measures and a block shows one, " +
				"so which is not a detail a renderer may pick",
		}
	}
	return id, nil
}

// IsInvalid says whether an error is a grammar refusal.
//
// Exported because the caller distinguishes "this document is malformed" from
// "this run does not exist", and the two are different answers to a composer.
func IsInvalid(err error) bool {
	var invalid *InvalidError
	return errors.As(err, &invalid)
}
