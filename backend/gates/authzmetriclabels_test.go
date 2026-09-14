// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// The outbound decision counter labels only closed vocabularies, and never the
// recipient.
//
// /metrics is process-global: it binds no workspace and resolves no principal,
// and it is unauthenticated unless a deployment sets a metrics token (an empty
// token is the default, see compose.gateMetrics). A label carrying the address
// a decision judged would publish who this installation writes to, across
// tenants, and would do so as a series a scraper keeps forever.
//
// It is also unbounded. A counter's cardinality is the product of its label
// values, so an address label mints one time series per recipient and the
// scrape grows with the address book.
//
// commsauthz.Decision carries Recipient, so the field is one struct literal
// away at all times. What stops it is that the counter is keyed by a type with
// no such field: DecisionCount names three closed vocabularies and nothing else.
//
// TWO ARMS, because the key is not the only way a label is born. The first
// reads the key type: a label the counter is grouped by must be one of the
// three. The second reads the RENDERER, because a second Fprintf in that file
// could emit a label from anything at all — a package variable, a passed
// argument — while DecisionCount stays untouched and the first arm stays green.
//
// WHAT NEITHER ARM SEES, stated because a prohibition that overclaims is worse
// than one that is narrow. Both read label NAMES; neither reads what a name is
// bound to, so `verdict=%q` filled with an address would pass. And the second
// arm reads literal format strings in one file, so a name assembled by
// concatenation, held in a constant, or written by a helper elsewhere is
// invisible to it. What makes those safe is the first arm plus the type
// system: the renderer's only input is a DecisionCount, whose three fields are
// closed vocabularies the database CHECKs also pin. These arms stop the easy
// way in — adding a field, adding a line — not a determined author.

import (
	"go/ast"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

const (
	// decisionCountFile declares the counter's key.
	decisionCountFile = "internal/modules/consent/decisioncounter.go"
	// decisionCountType is the key itself.
	decisionCountType = "DecisionCount"
	// metricsRendererFile writes the counter onto /metrics.
	metricsRendererFile = "internal/compose/authzmetrics.go"
)

// renderedLabels are the label names the renderer may emit. The same three the
// key declares, spelled as they appear in the exposition.
var renderedLabels = []string{"category", "mode", "verdict"}

// decisionCountFields are the three closed vocabularies a decision may be
// counted by, and the whole set.
//
// Spelled here rather than derived, because this gate's subject IS the list: a
// gate reading the struct to learn what the struct may contain would ratify
// whatever it found, which is the shape that passes while a recipient label is
// added.
var decisionCountFields = []string{"Verdict", "Category", "Mode"}

// forbiddenLabelSubstrings name what a label must never carry. Matched on the
// field name because a field is what becomes a label.
var forbiddenLabelSubstrings = []string{
	"recipient", "address", "email", "subject", "contact", "lead", "actor", "reason",
}

func TestTheDecisionCounterLabelsOnlyClosedVocabularies(t *testing.T) {
	t.Parallel()

	fields := structFieldNames(t, decisionCountFile, decisionCountType)
	if len(fields) == 0 {
		t.Fatalf("%s declares no %s fields — the subject this gate reads has moved, and a gate "+
			"that found nothing to judge reports the counter safe", decisionCountFile, decisionCountType)
	}

	for _, field := range fields {
		if slices.Contains(decisionCountFields, field) {
			continue
		}
		t.Errorf("%s.%s is a new counter label.\n\n"+
			"Every field here becomes a /metrics label, on an endpoint that binds no workspace "+
			"and authenticates nobody. A label must be a CLOSED vocabulary and must not name a "+
			"contact: add it to decisionCountFields only once it is both.",
			decisionCountType, field)
		for _, forbidden := range forbiddenLabelSubstrings {
			if strings.Contains(strings.ToLower(field), forbidden) {
				t.Errorf("%s.%s names %q. A recipient's address as a label publishes who this "+
					"installation writes to, to an unauthenticated scraper, and mints one time "+
					"series per contact.", decisionCountType, field, forbidden)
			}
		}
	}
}

// The renderer is where a label is actually written, so the second arm reads it.
//
// A gate holding only the key type would pass over a file that prints
// `{address=%q}` beside the counter it is meant to guard: DecisionCount would
// not have changed, and the label set it names would still be three. So this
// parses every label name the file emits and holds THOSE to the same three.
func TestTheRenderedLabelsAreOnlyTheDeclaredOnes(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile(metricsRendererFile)
	if err != nil {
		t.Fatalf("reading %s: %v — the renderer this gate reads has moved, and a gate that "+
			"found nothing to judge reports the labels safe", metricsRendererFile, err)
	}
	emitted := renderedLabelNames(string(source))
	if len(emitted) == 0 {
		t.Fatalf("%s emits no labelled metric line — this gate reads the rendering, and finding "+
			"none means it is reading the wrong file", metricsRendererFile)
	}
	for _, label := range emitted {
		if slices.Contains(renderedLabels, label) {
			continue
		}
		t.Errorf("%s renders a %q label.\n\n"+
			"Every label reaches an endpoint that binds no workspace and authenticates nobody. "+
			"A label naming a contact publishes who this installation writes to and mints one "+
			"time series each. Add it to renderedLabels only once it is a closed vocabulary "+
			"that names nobody.", metricsRendererFile, label)
	}
}

// labelInExposition matches one label name in a Prometheus line: the `name=`
// that opens a label pair inside the braces.
var labelInExposition = regexp.MustCompile(`[{,]([a-z_][a-z0-9_]*)=`)

// renderedLabelNames reads the label names a renderer emits, from the format
// strings it writes them with.
func renderedLabelNames(source string) []string {
	var out []string
	for _, match := range labelInExposition.FindAllStringSubmatch(source, -1) {
		if !slices.Contains(out, match[1]) {
			out = append(out, match[1])
		}
	}
	sort.Strings(out)
	return out
}

// A REMOVED label needs no arm here. The renderer names each field
// (internal/compose/authzmetrics.go), so dropping one stops the compiler before
// any test runs — a louder failure than this file could produce, and one nobody
// can skip. An arm asserting it would be a second, weaker copy of what the type
// system already holds.

// structFieldNames returns one named struct's field names.
func structFieldNames(t *testing.T, path, typeName string) []string {
	t.Helper()
	file := parseGo(t, path)
	var fields []string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, isSpec := n.(*ast.TypeSpec)
		if !isSpec || spec.Name.Name != typeName {
			return true
		}
		structType, isStruct := spec.Type.(*ast.StructType)
		if !isStruct {
			return false
		}
		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				fields = append(fields, name.Name)
			}
		}
		return false
	})
	return fields
}
