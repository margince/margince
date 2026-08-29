// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// Every kind a rep may put on automatic must have words on the settings screen
// that offers it, and the screen must offer no kind the product cannot automate.
//
// The card degrades gracefully — a kind it has no copy for renders its wire
// spelling rather than vanishing — and that is exactly what makes the staleness
// invisible. A fourth kind lands in AutoApplyKinds, the rep is offered a switch
// labelled `project_attribution` with no sentence under it, and every test still
// passes because nothing compares the two lists.
//
// So the corpus is approvals.SortedAutoApplyKinds(), the set the applier itself
// reads, and this compares BOTH directions: a kind the product can automate with
// no copy fails, and copy for a kind it cannot automate fails too — the second
// because a dead entry is how the next author learns the map is not maintained.
//
// The sibling gate over KIND_LABEL holds the same shape for the inbox's kinds.

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/approvals"
)

const frontendAutonomyCopy = "../frontend/src/screens/autonomy-settings.tsx"

// autonomyCopyEntry reads one `close_date_correction: {` key out of the map.
// Quoted keys match too: an entry written `"lifecycle_change":` would otherwise
// parse as nothing, and an entry this gate cannot see is one it agrees with.
var autonomyCopyEntry = regexp.MustCompile(`["']?\b([a-z][a-z0-9_]*)\b["']?:\s*\{`)

// tsCommentInAutonomyCopy strips comments before the entries are read. The map
// carries prose that NAMES kinds, and a kind deleted from the map but still
// mentioned above it would keep this gate green while the rep got the raw enum.
var tsCommentInAutonomyCopy = regexp.MustCompile(`(?s)//[^\n]*|/\*.*?\*/`)

// copiedKinds reads the KIND_COPY literal out of the card.
//
// Scoped to that one declaration rather than the whole file: the card's test
// fixtures and its story name kinds too, and a file-wide scan would count a kind
// as covered because a story mentions it.
func copiedKinds(t *testing.T, source string) []string {
	t.Helper()
	const opener = "const KIND_COPY"
	start := strings.Index(source, opener)
	if start < 0 {
		t.Fatalf("no %s declaration in %s", opener, frontendAutonomyCopy)
	}
	body := source[start:]
	if end := strings.Index(body, "\n};"); end >= 0 {
		body = body[:end]
	}
	body = tsCommentInAutonomyCopy.ReplaceAllString(body, "")
	var kinds []string
	for _, match := range autonomyCopyEntry.FindAllStringSubmatch(body, -1) {
		kinds = append(kinds, match[1])
	}
	if len(kinds) == 0 {
		t.Fatalf("read no entries out of KIND_COPY — the parser has stopped seeing the map")
	}
	slices.Sort(kinds)
	return slices.Compact(kinds)
}

func TestEveryAutomatableKindHasSettingsCopy(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendAutonomyCopy)
	if err != nil {
		t.Fatalf("reading the autonomy settings card: %v", err)
	}
	copied := copiedKinds(t, string(source))
	automatable := approvals.SortedAutoApplyKinds()

	for _, kind := range automatable {
		if !slices.Contains(copied, kind) {
			t.Errorf("kind %q can be put on automatic but has no entry in KIND_COPY: "+
				"the rep is offered a switch labelled with the wire enum and no sentence saying what it does", kind)
		}
	}
	for _, kind := range copied {
		if !slices.Contains(automatable, kind) {
			t.Errorf("KIND_COPY carries %q, which cannot be put on automatic: "+
				"the row it describes is never offered, and a dead entry teaches the next author the map is not maintained", kind)
		}
	}
}
