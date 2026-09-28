// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The offer-template form and the PDF renderer must spell a layout key the
// same way, or what an administrator types is stored and never printed.
//
// It happened: the form saved `header` and `footer`, the renderer read
// `header_text` and `footer_text`, and both sides passed their own tests
// because each test supplied its own spelling. layout is a free jsonb bag, so
// nothing in the contract could notice. This gate compares both directions —
// a key the form writes that the renderer ignores fails, and so does a key the
// renderer prints that no form can write.

import (
	"os"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
)

const offerTemplateForm = "../frontend/src/screens/offertemplates.tsx"

// tsLayoutLiteral captures the body of every `layout: { … }` object literal;
// the form spells one for create and one for the full-replace update.
var tsLayoutLiteral = regexp.MustCompile(`(?s)\blayout:\s*\{(.*?)\}`)

// tsLayoutKey reads a property name at the start of a line inside a literal,
// bare or quoted, so a quoted spelling cannot hide from the comparison.
var tsLayoutKey = regexp.MustCompile(`(?m)^\s*["']?([A-Za-z_][A-Za-z0-9_]*)["']?\s*:`)

// tsLayoutRead is the edit form's prefill, which reads the stored keys back
// off the template it is editing.
var tsLayoutRead = regexp.MustCompile(`\.layout\.([A-Za-z_][A-Za-z0-9_]*)`)

func TestTheOfferTemplateFormWritesTheKeysTheRendererPrints(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(offerTemplateForm)
	if err != nil {
		t.Fatalf("reading the offer-template form: %v", err)
	}
	form := tsComment.ReplaceAllString(string(source), " ")
	printed := slices.Sorted(slices.Values(deals.OfferTemplateLayoutTextKeys()))

	literals := tsLayoutLiteral.FindAllStringSubmatch(form, -1)
	if len(literals) == 0 {
		t.Fatalf("%s spells no `layout: { … }` literal — this gate is reading a shape that is gone", offerTemplateForm)
	}
	for i, literal := range literals {
		assertLayoutKeys(t, "layout literal "+strconv.Itoa(i+1), tsLayoutKey.FindAllStringSubmatch(literal[1], -1), printed)
	}
	assertLayoutKeys(t, "edit prefill", tsLayoutRead.FindAllStringSubmatch(form, -1), printed)
}

// assertLayoutKeys fails when one spelling site of the form disagrees with the
// renderer's key set in either direction.
func assertLayoutKeys(t *testing.T, site string, matches [][]string, printed []string) {
	t.Helper()
	var written []string
	for _, m := range matches {
		if !slices.Contains(written, m[1]) {
			written = append(written, m[1])
		}
	}
	slices.Sort(written)
	if !slices.Equal(written, printed) {
		t.Errorf("%s: the form's %s spells layout keys %q, the renderer prints %q — a key on one side only is typed and never printed, or printed and never typable",
			offerTemplateForm, site, written, printed)
	}
}
