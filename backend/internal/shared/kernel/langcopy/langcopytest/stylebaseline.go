// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// stylebaseline.txt holds the findings the tree still has. A test fails an
// entry whose sentence passes or is gone, and the recut drops those alone.
//
//go:embed stylebaseline.txt
var baselineText string

const baselinePath = "backend/internal/shared/kernel/langcopy/langcopytest/stylebaseline.txt"

// Waiver is one finding the baseline holds: a rule id and the sentence. A rule
// that reads the English also keeps the English it was checked beside.
type Waiver struct{ Rule, Text, English string }

// StyleBaseline answers every waiver the baseline holds, in file order.
func StyleBaseline() ([]Waiver, error) { return parseBaseline(baselineText) }

func parseBaseline(text string) ([]Waiver, error) {
	var waivers []Waiver
	for number, line := range strings.Split(text, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		w, err := parseWaiver(line)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", baselinePath, number+1, err)
		}
		waivers = append(waivers, w)
	}
	return waivers, nil
}

func parseWaiver(line string) (Waiver, error) {
	fields := strings.Split(line, "\t")
	rule, known := ruleNamed(fields[0])
	want := 2
	if known && rule.readsEnglish {
		want = 3
	}
	quoted := make([]string, len(fields))
	for i, field := range fields[1:] {
		unquoted, err := strconv.Unquote(field)
		if err != nil {
			return Waiver{}, fmt.Errorf("%q is not a rule id and tab-separated quoted sentences", line)
		}
		quoted[i] = unquoted
	}
	if len(fields) != want {
		return Waiver{}, fmt.Errorf("%q has %d fields, and %s takes %d: the rule id, the sentence and, "+
			"for a rule that reads it, the English", line, len(fields), fields[0], want)
	}
	return Waiver{Rule: fields[0], Text: quoted[0], English: quoted[1]}, nil
}

func (w Waiver) line() string {
	line := w.Rule + "\t" + strconv.Quote(w.Text)
	if rule, known := ruleNamed(w.Rule); known && rule.readsEnglish {
		line += "\t" + strconv.Quote(w.English)
	}
	return line
}

var baselineSet = sync.OnceValues(func() (map[Waiver]bool, error) {
	waivers, err := StyleBaseline()
	if err != nil {
		return nil, err
	}
	set := make(map[Waiver]bool, len(waivers))
	for _, w := range waivers {
		set[w] = true
	}
	return set, nil
})

// waived reports whether the baseline holds a finding.
func waived(t reporter, rule styleRule, text, english string) bool {
	t.Helper()
	w := Waiver{Rule: rule.id, Text: text}
	if rule.readsEnglish {
		w.English = english
	}
	set, err := baselineSet()
	if err != nil {
		t.Fatalf("the style baseline cannot be read: %v", err)
	}
	return set[w]
}
