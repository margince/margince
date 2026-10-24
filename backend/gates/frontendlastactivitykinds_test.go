// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The browser counts the same activity kinds toward last activity as the
// server. A lead's How it stands card recomputes its last touch from the
// timeline. A kind only one side counts shows two dates for one lead.

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

var frontendLastActivity = filepath.Join(repoRoot, "frontend", "src", "features", "leads", "lastactivity.ts")

func TestTheFrontendLastActivityKindsMatchTheGoOnes(t *testing.T) {
	t.Parallel()
	inTS := tsLastActivityKinds(t)
	inGo := relstrength.LastActivityKinds()
	for _, kind := range inGo {
		if !slices.Contains(inTS, kind) {
			t.Errorf("the server counts %q toward last activity and %s does not", kind, frontendLastActivity)
		}
	}
	for _, kind := range inTS {
		if !slices.Contains(inGo, kind) {
			t.Errorf("%s counts %q toward last activity and the server does not", frontendLastActivity, kind)
		}
	}
}

// tsLastActivityKinds reads the LAST_ACTIVITY_KINDS array. Anything left once
// the quoted entries and commas are gone is an entry this parser cannot see.
func tsLastActivityKinds(t *testing.T) []string {
	t.Helper()
	source, err := os.ReadFile(frontendLastActivity)
	if err != nil {
		t.Fatalf("reading the frontend last-activity kinds: %v", err)
	}
	const opener = "export const LAST_ACTIVITY_KINDS = ["
	start := strings.Index(string(source), opener)
	if start < 0 {
		t.Fatalf("%s no longer declares %s: this gate is reading a shape that is gone", frontendLastActivity, opener)
	}
	body := string(source)[start+len(opener):]
	end := strings.Index(body, "] as const")
	if end < 0 {
		t.Fatalf("%s: LAST_ACTIVITY_KINDS is not closed by `] as const`", frontendLastActivity)
	}
	body = tsCommentInAcquisitionKinds.ReplaceAllString(body[:end], " ")
	var kinds []string
	for _, match := range tsQuotedEntry.FindAllStringSubmatch(body, -1) {
		kinds = append(kinds, match[1]+match[2]+match[3])
	}
	if rest := strings.Trim(tsQuotedEntry.ReplaceAllString(body, ""), ", \r\n\t"); rest != "" {
		t.Fatalf("%s: LAST_ACTIVITY_KINDS holds %q, which is not a quoted kind this gate can read", frontendLastActivity, rest)
	}
	if len(kinds) == 0 {
		t.Fatal("read no kinds out of LAST_ACTIVITY_KINDS: a gate that reads nothing agrees with everything")
	}
	return kinds
}
