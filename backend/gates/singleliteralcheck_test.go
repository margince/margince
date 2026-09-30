// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// A CHECK that admits exactly one value is a discriminator or it is nothing.
//
// `DEFAULT x` plus `CHECK (col = x)` makes a column the same on every row that
// exists and every row that can exist. It cannot carry information, and the cost
// is not the byte: it reads to the next contributor as a dimension the product
// HAS, so they group a report by it or draw a control that switches on it, and
// find one bucket.
//
// The honest version of the shape is a real discriminator whose CHECK tracks
// what is implemented — written explicitly, read back, and widened by the change
// that adds the second value. The dishonest version has no writer at all: the
// column takes its default forever and the constraint guards a decision nobody
// is making. Two of each were in this schema, and only the pair below is the
// first kind.
//
// So the rule is not that the shape is wrong. It is that a column nothing varies
// has to say which of the two it is, beside the writer that makes it the first.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// heldToOneValue is `table.column`, which is what the reader has to go and look
// for a writer of.
type heldToOneValue string

// oneValueColumns are the columns a CHECK holds to a single literal on purpose,
// each naming the writer that makes it a discriminator rather than a default
// nobody moves.
var oneValueColumns = gatekit.Waive(map[heldToOneValue]string{
	"channel_connection.provider": "the channel provider, written explicitly by every connection insert and read back " +
		"to route a send. It carries no default, so a row that named nothing would be refused rather than quietly " +
		"becoming telegram, and the CHECK widens with the second provider implemented",
	"communication_review.kind": "the review kind, written as reviewKindSingle by the slice that creates one and read " +
		"on two paths. One review, one send, is the only shape this product performs today; the column is on the " +
		"contract so a second kind is a widening rather than a new field every client has to learn",
})

// singleLiteralCheck matches a constraint whose WHOLE body equals a bare column
// against one literal. Conditional forms — `x IS NULL OR col = 'lit'` — say
// something about a combination rather than holding the column to one value, and
// are not this shape.
var singleLiteralCheck = regexp.MustCompile(
	// The literal body accepts an EMPTY string and an escaped quote. A column
	// held to '' or to 'it''s' is held to one value exactly as much as one held
	// to a word, and a pattern that skipped them would under-report — the one
	// direction a census must not fail in.
	`^(?:public|ext)\.([a-z0-9_]+)\.[a-zA-Z0-9_]+ CHECK \(\(([a-z_][a-z0-9_]*) = '(?:[^']|'')*'::[a-z ]+\)\)$`)

func TestEveryColumnHeldToOneValueNamesItsWriter(t *testing.T) {
	t.Parallel()

	for _, record := range catalogRecords(t) {
		m := singleLiteralCheck.FindStringSubmatch(strings.TrimSpace(record))
		if m == nil {
			continue
		}
		// Asked about an OFFENDER: only a column the schema really holds to one
		// value reaches here, so an entry whose column was dropped or widened
		// stops matching and AssertAllMatched reports it.
		column := heldToOneValue(m[1] + "." + m[2])
		if oneValueColumns.Waived(t, column) {
			continue
		}
		t.Errorf("%s is held by a CHECK to one literal, so it is the same on every row that exists and "+
			"every row that can exist. Drop it if nothing writes it, or declare it in oneValueColumns with "+
			"the writer that makes it a discriminator", column)
	}
	oneValueColumns.AssertAllMatched(t)
}
