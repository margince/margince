// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// The privacy module's raw-capture correlation is spelled once.
//
// The poller keys a channel original by the provider's redelivery id, so a
// natural-key join finds no activity for it. Such a copy answers wrongly and
// never fails.
//
// It reads every file of the module, because the stage's copy sat beside the
// one file the first gate read. Other modules cannot import this one.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// privacyModuleDir is relative to the module root, where the gates package runs.
const privacyModuleDir = "internal/modules/privacy"

// rawCaptureCorrelation matches a join between an activity aliased `a` and a
// raw_capture row under any alias, in either spelling of the correlation.
var rawCaptureCorrelation = regexp.MustCompile(
	`a\.raw_capture_id\s*=\s*\w+\.id|\w+\.id\s*=\s*a\.raw_capture_id|` +
		`a\.source_id\s*=\s*\w+\.source_id|\w+\.source_id\s*=\s*a\.source_id`)

func TestThePrivacyRawCaptureCorrelationIsSpelledOnce(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(filepath.Join(privacyModuleDir, "*.go"))
	if err != nil {
		t.Fatalf("listing %s: %v", privacyModuleDir, err)
	}
	declared := 0
	scanned := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}
		scanned++
		// The constant's body is the one spelling. Cut it out, and nothing that
		// correlates the two tables may remain.
		rest, found := withoutTheLinkConstant(string(src))
		if found {
			declared++
		}
		if loose := rawCaptureCorrelation.FindAllString(rest, -1); len(loose) > 0 {
			t.Errorf("%s correlates activity to raw_capture %d time(s) outside rawCaptureLink: %v\n\n"+
				"Every use must go through the name. A copy on the natural key alone finds no "+
				"activity for a channel original, and the statement answers wrongly without failing",
				file, len(loose), loose)
		}
	}
	if scanned == 0 {
		t.Fatalf("no Go source under %s; the module moved and this gate reads nothing", privacyModuleDir)
	}
	if declared != 1 {
		t.Fatalf("rawCaptureLink is declared %d time(s) under %s, want exactly 1: the statements "+
			"that join an original to its activity need one spelling to share", declared, privacyModuleDir)
	}
}

// withoutTheLinkConstant answers the file with rawCaptureLink's declaration
// removed, and whether it was there at all.
//
// The cut runs to the closing backtick, so a reformatting cannot slide it off.
func withoutTheLinkConstant(src string) (string, bool) {
	const decl = "const rawCaptureLink = `"
	open := strings.Index(src, decl)
	if open < 0 {
		return src, false
	}
	end := strings.Index(src[open+len(decl):], "`")
	if end < 0 {
		return src, false
	}
	return src[:open] + src[open+len(decl)+end+1:], true
}
