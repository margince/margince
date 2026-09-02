// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The page that names a deletion date and the sweep that carries it out must
// read ONE window, or the product promises a date it does not keep.
//
// Two callers ask about the same window at different moments. The Senders page
// asks "when does this sender's mail go", so an owner can object; the sweep asks
// "what may I destroy now". A second spelling of the arithmetic is how the page
// would come to show a date the sweep had already passed — and the owner would
// have had no window at all, having been shown one.
//
// So the deadline is written once, in capture.PersonalPurgeDeadline, and this
// gate fails when a second copy appears. It looks for the shape rather than for
// the two known call sites: a hard-coded pair would go on passing while a third
// caller wrote its own.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// windowArithmetic matches the interval arithmetic the deadline is made of: the
// per-message clock added to a window chosen by authority. Any file computing
// this itself is a second answer to when a message dies.
//
// Deliberately matched on the greatest()/resolved_by_owner pair rather than on
// either alone — resolved_by_owner is read for other reasons (the page shows who
// decided), and greatest() over timestamps is ordinary.
var windowArithmetic = regexp.MustCompile(
	`greatest\(\s*a\.created_at\s*,\s*p\.resolved_at\s*\)[^;]{0,200}resolved_by_owner`)

// deadlineHome is the one file allowed to spell it.
const deadlineHome = "internal/modules/capture/purgepersonal.go"

func TestOnePlaceDecidesWhenPersonalMailDies(t *testing.T) {
	t.Parallel()
	var offenders []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel := filepath.ToSlash(path)
		if rel == deadlineHome {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if windowArithmetic.Match(body) {
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}
	if len(offenders) > 0 {
		t.Errorf("%d file(s) compute the personal-purge deadline themselves instead of calling "+
			"capture.PersonalPurgeDeadline:\n\t%s\n\n"+
			"The Senders page shows this date and the sweep acts on it. Two spellings drift, and the "+
			"drift is invisible until an owner is shown a deadline that has already passed.",
			len(offenders), strings.Join(offenders, "\n\t"))
	}
	// A gate that matches nothing has stopped being one: if the home file stops
	// spelling the arithmetic, the regex is stale and every other file passes
	// for the wrong reason.
	home, err := os.ReadFile(deadlineHome)
	if err != nil {
		t.Fatalf("reading %s: %v", deadlineHome, err)
	}
	if !windowArithmetic.Match(home) {
		t.Errorf("%s no longer matches windowArithmetic, so this gate now admits every file in the "+
			"tree. Update the pattern to whatever the deadline is spelled as now.", deadlineHome)
	}
}
