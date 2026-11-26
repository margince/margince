// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// stylebaseline.txt holds the English and Vietnamese findings the tree had
// when it was cut. It may only shrink:
// TestEveryStyleWaiverStillBreaksItsRule fails an entry whose sentence passes,
// and TestEveryStyleWaiverNamesASentenceInTheTree one whose sentence is gone.
//
//go:embed stylebaseline.txt
var baselineText string

const baselinePath = "backend/internal/shared/kernel/langcopy/langcopytest/stylebaseline.txt"

// recordVariable names a file. When it is set, every finding is appended there
// and none fails, so a re-cut sees the whole tree.
const recordVariable = "LANGCOPY_STYLE_RECORD"

// Waiver is one finding the baseline holds: a rule id, which leads with the
// language, and the sentence as the table holds it.
type Waiver struct{ Rule, Text string }

// StyleBaseline answers every waiver the baseline holds, in file order.
func StyleBaseline() ([]Waiver, error) { return parseBaseline(baselineText) }

func parseBaseline(text string) ([]Waiver, error) {
	var waivers []Waiver
	for number, line := range strings.Split(text, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rule, quoted, found := strings.Cut(line, "\t")
		sentence, err := strconv.Unquote(quoted)
		if !found || err != nil {
			return nil, fmt.Errorf("%s:%d is not a rule id, a tab and a quoted sentence: %q",
				baselinePath, number+1, line)
		}
		waivers = append(waivers, Waiver{Rule: rule, Text: sentence})
	}
	return waivers, nil
}

func (w Waiver) line() string { return w.Rule + "\t" + strconv.Quote(w.Text) }

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

// waived reports whether the baseline holds a finding, or records it when a
// re-cut is running.
func waived(t reporter, rule, text string) bool {
	t.Helper()
	if path := os.Getenv(recordVariable); path != "" {
		if err := record(path, Waiver{Rule: rule, Text: text}); err != nil {
			t.Fatalf("recording a style finding: %v", err)
		}
		return true
	}
	set, err := baselineSet()
	if err != nil {
		t.Fatalf("the style baseline cannot be read: %v", err)
	}
	return set[Waiver{Rule: rule, Text: text}]
}

var recording sync.Mutex

// record appends a finding and the package that found it. One write to a file
// opened for append keeps test binaries running side by side from interleaving.
func record(path string, w Waiver) error {
	recording.Lock()
	defer recording.Unlock()
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Clean(path), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = file.WriteString(w.line() + "\t" + strconv.Quote(dir) + "\n")
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}
