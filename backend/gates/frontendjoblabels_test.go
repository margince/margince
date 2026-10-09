// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// History names the job behind a change by its words, never by its key. The
// words are one `systemJob.<kind>` message per kind in the frontend catalogue,
// a declared mirror of api/jobs.yaml. A kind without a message reads as a bare
// "System task", and a message without a kind is dead, so both directions fail.

import (
	"os"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

const frontendCatalogue = "../frontend/src/i18n/en.ts"

// tsJobLabelKey reads one `"systemJob.<kind>":` key out of the English
// catalogue. Comments are stripped first (tsComment), so a line that only
// mentions a key cannot stand in for one that was deleted.
var tsJobLabelKey = regexp.MustCompile(`"systemJob\.([a-z0-9_]+)"\s*:`)

func TestEveryDeclaredJobKindHasAFrontendLabel(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendCatalogue)
	if err != nil {
		t.Fatalf("reading the frontend catalogue: %v", err)
	}
	labelled := map[string]bool{}
	for _, m := range tsJobLabelKey.FindAllStringSubmatch(tsComment.ReplaceAllString(string(source), " "), -1) {
		labelled[m[1]] = true
	}

	declared := map[string]bool{}
	for kind := range jobs.Declared() {
		if !jobs.IsExtensionKind(kind) {
			declared[kind] = true
		}
	}
	if len(declared) == 0 || len(labelled) == 0 {
		t.Fatalf("read %d declared kinds and %d labels — a census that reads nothing agrees with everything", len(declared), len(labelled))
	}

	for kind := range declared {
		if !labelled[kind] {
			t.Errorf("job kind %s has no \"systemJob.%s\" message in %s (and de.ts, vi.ts), so a change it makes reads as a bare \"System task\" in History", kind, kind, frontendCatalogue)
		}
	}
	for kind := range labelled {
		if !declared[kind] {
			t.Errorf("%s labels systemJob.%s, which api/jobs.yaml does not declare — delete the message or declare the kind", frontendCatalogue, kind)
		}
	}
}
