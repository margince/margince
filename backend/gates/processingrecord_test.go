// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The Art. 30 processing record names the code that enforces each entry, and
// this fails when that code is not there any more.
//
// A processing record is a promise about what the software does. Every activity
// in it carries a `Durchsetzung` row naming the file that carries out the
// promise — that is what makes the document checkable rather than aspirational,
// and it is the only part of it a test can hold. A renamed or deleted file turns
// the promise into a citation of nothing, and nothing else in the tree notices:
// the document is prose, the code compiles, and the discrepancy surfaces at an
// audit.
//
// What this does NOT check, said plainly rather than left to be assumed: whether
// the named file still does what the row claims. No test can read that. The
// obligation this holds is narrower — that a reader following the citation
// arrives somewhere — and it is worth holding because the failure it catches
// (a path that resolves to nothing) is the one that happens by accident.
//
// Deriving the subjects from the document rather than listing them here is the
// point. A list would be a second copy of the record, and a new activity added
// to the record with a bad path would pass a gate that only knew the old ones.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// processingRecords are the Art. 30 documents this rule covers. A record in a
// language not listed here is not exempt — it is unwritten, and adding one adds
// it here.
var processingRecords = []string{
	"../docs/compliance/de/verarbeitungsverzeichnis-und-dsfa.md",
}

// citedGoFile matches a backtick-quoted Go path in the record's prose, which is
// how every Durchsetzung row spells the code it points at.
//
// Anchored on the `.go` suffix rather than on the row label, so a citation that
// moves into a different column or a sentence is still checked. The paths are
// written relative to `internal/` with the leading segments trimmed
// (`capture/sinkactivity.go`), which is how a reader would say it out loud.
var citedGoFile = regexp.MustCompile("`([a-z][a-zA-Z0-9_]*(?:/[a-zA-Z0-9_]+)*\\.go)`")

func TestTheProcessingRecordCitesCodeThatExists(t *testing.T) {
	t.Parallel()
	for _, record := range processingRecords {
		body, err := os.ReadFile(record)
		if err != nil {
			t.Fatalf("reading the processing record %s: %v", record, err)
		}
		cited := citedGoFile.FindAllStringSubmatch(string(body), -1)
		// A record citing nothing has either lost its Durchsetzung rows or the
		// pattern has stopped matching them. Either way this gate is passing
		// vacuously, which is the one failure a census must not have.
		if len(cited) == 0 {
			t.Errorf("%s cites no Go file at all — either the enforcement rows are gone, or "+
				"citedGoFile no longer matches how they are written. A record that names no code "+
				"is a promise nothing holds.", record)
			continue
		}
		seen := map[string]bool{}
		for _, match := range cited {
			path := match[1]
			if seen[path] {
				continue
			}
			seen[path] = true
			switch matches := resolveCitedFile(t, path); len(matches) {
			case 1:
			case 0:
				t.Errorf("%s cites `%s`, which is not in the tree. The Art. 30 record names this "+
					"file as what enforces a processing activity, so a reader following the citation "+
					"arrives nowhere. Update the record in the same change that moved the code.",
					record, path)
			default:
				t.Errorf("%s cites `%s`, which %d files answer to (%s). A reader following the "+
					"citation cannot tell which enforces the activity — write the citation long "+
					"enough to name one.", record, path, len(matches), strings.Join(matches, ", "))
			}
		}
	}
}

// resolveCitedFile finds the cited path under the module, or returns "" — and
// reports an ambiguous citation as a failure rather than picking one.
//
// A suffix match rather than an exact one, because the record writes the path
// the way a reader says it — `capture/sinkactivity.go` for what actually lives
// at `internal/modules/capture/sinkactivity.go`. Requiring the full path in the
// document would make it harder to read.
//
// Every match is collected rather than the first taken. A short-circuit would
// resolve a collision to whichever file the walk reached first and report PASS,
// which is the under-recognition shape a census must not have: it reads a
// smaller answer and nothing fails to say so. Two files answering one citation
// means a reader following it cannot tell which was meant, and that ambiguity is
// itself the finding.
func resolveCitedFile(t *testing.T, cited string) []string {
	t.Helper()
	var found []string
	err := filepath.Walk("internal", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(filepath.ToSlash(path), "/"+cited) {
			found = append(found, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree for %s: %v", cited, err)
	}
	return found
}
