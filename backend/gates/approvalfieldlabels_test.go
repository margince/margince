// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// An approval names a withheld field with the label the record's history panel
// gives it.
//
// The approval title and its email read agents.FieldLabel; the history panel
// reads history.field.* from the frontend catalog. Two labels for one field
// make a reader wonder whether "Value" in the email and "Amount" on the record
// are the same thing. A field the frontend does not label yet is the backend's
// alone to name. The TypeScript is read with a regexp, the trade
// frontendminorunits_test.go makes.

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// historyFieldLabel reads one history.field.* entry, whose value may sit on the
// next line when the formatter wraps it. Anchored at a line's start, so a
// comment quoting a key is not read as the key.
var historyFieldLabel = regexp.MustCompile(`(?m)^\s*"history\.field\.([a-z0-9_]+)"\s*:\s*("(?:[^"\\\n]|\\.)*")`)

// historyCatalog is the frontend catalog a shipped language is read from.
func historyCatalog(lang textlang.Lang) string {
	return "../frontend/src/i18n/" + string(lang) + ".ts"
}

func TestAnApprovalNamesAFieldAsTheHistoryPanelDoes(t *testing.T) {
	t.Parallel()
	for _, lang := range textlang.Shipped {
		for _, drift := range fieldLabelDrift(t, lang, readSource(t, historyCatalog(lang))) {
			t.Errorf("%s: %s", lang, drift)
		}
	}
}

// A changed frontend label, planted in the real catalog, must be seen.
func TestTheFieldLabelGateSeesAChangedLabel(t *testing.T) {
	t.Parallel()
	catalog := readSource(t, historyCatalog(textlang.English))
	planted := strings.Replace(catalog, `"history.field.owner_id": "Owner"`, `"history.field.owner_id": "Account lead"`, 1)
	if planted == catalog {
		t.Fatal("the catalog has no owner_id label to change, so the plant proves nothing")
	}
	if len(fieldLabelDrift(t, textlang.English, planted)) == 0 {
		t.Error("planted a changed owner_id label and the gate still agrees")
	}
}

// fieldLabelDrift compares every backend label with the frontend's label for
// the same field, where the frontend has one.
func fieldLabelDrift(t *testing.T, lang textlang.Lang, catalog string) []string {
	t.Helper()
	frontend := map[string]string{}
	for _, m := range historyFieldLabel.FindAllStringSubmatch(catalog, -1) {
		label, err := strconv.Unquote(m[2])
		if err != nil {
			t.Fatalf("history.field.%s holds %s, which this gate cannot read: %v", m[1], m[2], err)
		}
		frontend[m[1]] = label
	}
	var drift []string
	shared := 0
	for _, field := range agents.LabelledFields() {
		want, ok := frontend[field]
		if !ok {
			continue
		}
		shared++
		if got := agents.FieldLabel(lang, field); got != want {
			drift = append(drift, field+": an approval says "+strconv.Quote(got)+", the history panel says "+strconv.Quote(want))
		}
	}
	if shared == 0 {
		t.Fatalf("%s: no field is labelled on both sides, so this comparison would pass vacuously", lang)
	}
	return drift
}
