// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// One value, spelled once. activities keys its transcript reader and retention
// selector on it. deals recognises a recording by it (MeetingWasHeld). auth lets
// a meeting's guests read its transcript by it. If two spellings drifted,
// deals would stop recognising transcripts with no error, and a guest would lose
// the transcript. So the kernel (meetingoutcome) holds the literal and every
// other declaration refers to it.

import (
	"bytes"
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// transcriptMarkerDecl is a const named for the transcript source and what it
// is bound to.
var transcriptMarkerDecl = regexp.MustCompile(`(?m)^\s*(?:const\s+)?(\w*[Tt]ranscriptSource\w*)\s*=\s*(\S+)`)

// transcriptMarkerLiteralUse is a comparison of source_system with the marker
// spelled out, in SQL or in Go, instead of through the constant.
var transcriptMarkerLiteralUse = regexp.MustCompile(`source_system(?:\s*=|\s+IS\s+(?:NOT\s+)?DISTINCT\s+FROM)\s*'transcript'|"source_system"\]\s*==\s*"transcript"`)

const (
	transcriptMarkerHome    = "internal/shared/kernel/meetingoutcome/meetingoutcome.go"
	transcriptMarkerLiteral = `"transcript"`
	transcriptMarkerRef     = "meetingoutcome.TranscriptSourceSystem"
)

func TestTheTranscriptMarkerIsSpelledOnce(t *testing.T) {
	t.Parallel()
	declared := 0
	err := gatekit.WalkDir("internal", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := gatekit.ReadFile(path)
		if err != nil {
			return err
		}
		// Each pattern spells its word in this case: source_system in both use
		// branches, ranscriptSource in every declared name.
		if path != transcriptMarkerHome && bytes.Contains(src, []byte("source_system")) &&
			transcriptMarkerLiteralUse.Match(src) {
			t.Errorf("%s compares source_system with the marker spelled out; use %s", path, transcriptMarkerRef)
		}
		if !bytes.Contains(src, []byte("ranscriptSource")) {
			return nil
		}
		for _, m := range transcriptMarkerDecl.FindAllStringSubmatch(string(src), -1) {
			declared++
			switch {
			case path == transcriptMarkerHome:
				if m[2] != transcriptMarkerLiteral {
					t.Errorf("%s binds %s to %s, want the literal %s", path, m[1], m[2], transcriptMarkerLiteral)
				}
			case m[2] != transcriptMarkerRef:
				t.Errorf("%s declares %s = %s; refer to %s instead, so the marker has one spelling",
					path, m[1], m[2], transcriptMarkerRef)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The home, activities, deals: fewer means the scan stopped seeing them.
	if declared < 3 {
		t.Fatalf("found %d transcript marker declarations, want at least 3: the scan has stopped seeing its subject", declared)
	}
}
