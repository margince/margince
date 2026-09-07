// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// One value, spelled in two modules, because a module never imports a sibling.
//
// activities owns the marker: it is what its transcript reader keys on and what
// its retention selector matches. deals must recognise the same marker to know
// that a meeting activity carries a recording — the proof MeetingWasHeld rests
// on — and it may not import activities to ask.
//
// So there are two constants holding one string, and if they drift the failure
// is SILENT in the direction that matters: deals stops recognising transcripts,
// MeetingWasHeld falls through to its weaker arm, and every meeting without an
// explicit held status quietly stops settling its criterion. Nothing errors.
//
// Both spellings are read out of the source rather than restated here. A gate
// that hard-codes the value it protects is a third copy, and would agree with
// itself while the two real ones disagreed.

import (
	"os"
	"regexp"
	"testing"
)

// A Go const declaration binding a string literal, captured by name.
var transcriptMarkerConst = regexp.MustCompile(`(?m)^\s*(?:const\s+)?(\w*[Tt]ranscriptSourceSystem)\s*=\s*"([^"]*)"`)

// gatekit:fixture the two files that must agree and the constant each declares
//
// The two files that must agree, each with the constant it is expected to
// declare. Named rather than searched, because the obligation is that THESE
// two spellings match — a scan that found neither would pass while holding
// nothing.
var transcriptMarkerSites = map[string]string{
	"internal/modules/activities/mapping.go": "transcriptSourceSystem",
	"internal/modules/deals/authorside.go":   "TranscriptSourceSystem",
}

func TestTheTranscriptMarkerIsSpelledTheSameInBothModules(t *testing.T) {
	t.Parallel()

	found := map[string]string{}
	for path, name := range transcriptMarkerSites {
		found[path] = transcriptMarkerValue(t, path, name)
	}

	// Under-recognition is the failure that reports PASS, so the count is
	// checked before the values are compared: two empty reads agree perfectly.
	if len(found) != len(transcriptMarkerSites) {
		t.Fatalf("read %d of %d marker sites: the scan has stopped seeing its subject",
			len(found), len(transcriptMarkerSites))
	}

	var first, firstPath string
	for path, value := range found {
		if firstPath == "" {
			first, firstPath = value, path
			continue
		}
		if value != first {
			t.Errorf("the transcript marker has drifted: %s says %q, %s says %q.\n"+
				"deals recognises a transcript by this value and cannot import "+
				"activities to ask; a mismatch makes every transcript invisible to "+
				"MeetingWasHeld without failing anything",
				firstPath, first, path, value)
		}
	}
}

// transcriptMarkerValue reads one named constant's string literal out of a
// source file, failing when the declaration it was told to find is not there —
// a renamed constant is drift too, and a scan that silently found nothing is
// the one outcome this gate must not have.
func transcriptMarkerValue(t *testing.T, relPath, name string) string {
	t.Helper()
	src, err := os.ReadFile(relPath)
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	for _, m := range transcriptMarkerConst.FindAllStringSubmatch(string(src), -1) {
		if m[1] == name {
			return m[2]
		}
	}
	t.Fatalf("%s no longer declares %s: the marker moved or was renamed, and "+
		"this gate can no longer see the value it holds", relPath, name)
	return ""
}
