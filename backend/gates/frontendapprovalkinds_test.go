// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// Every kind a proposal can be staged under must have words a reader
// recognises, or the queue prints the wire enum at them.
//
// `approval.kind` is server vocabulary — `fx_rate_proposal`, `site_lead` — and
// the frontend's KIND_LABEL is what turns it into a sentence. The map used to
// be pinned by a list hand-copied INTO the frontend's own test, which is a
// mirror of a mirror: it went stale in the direction that cannot fail, because
// a kind missing from BOTH copies agrees with itself. Two kinds reached a
// German list in English exactly that way.
//
// So the corpus comes from approvals.StageableKinds(), which is derived from
// the two grant maps rather than restated beside them, and this gate compares
// both directions: a kind the server can stage with no label fails, and a
// label for a kind the server cannot stage fails too — the second because a
// dead entry is how a reader learns the map is not maintained.

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/approvals"
)

const frontendApprovalKinds = "../frontend/src/screens/approvalkind.ts"

// kindLabelEntry reads one `kind: "approval.kind.x"` pair out of the map.
// Quoted keys are matched too: an entry written `"send_email":` would
// otherwise parse as nothing, and an entry this gate cannot see is one it
// silently agrees with.
var kindLabelEntry = regexp.MustCompile(`["']?\b([a-z][a-z0-9_]*)\b["']?:\s*["']approval\.kind\.`)

// tsCommentInKinds strips comments before the entries are read. The map is
// heavily commented and several comments NAME kinds; without this, a kind
// deleted from the map but mentioned in the prose above it keeps this gate
// green while the reader gets the raw enum.
var tsCommentInKinds = regexp.MustCompile(`(?s)//[^\n]*|/\*.*?\*/`)

// labelledKinds reads the KIND_LABEL literal out of the TypeScript module.
//
// Scoped to that one declaration rather than the whole file: EDITABLE_FIELDS
// lives in the same module and its keys are kinds too, so a file-wide scan
// would count a kind as labelled because it appears there instead.
func labelledKinds(t *testing.T, source string) []string {
	t.Helper()
	const opener = "export const KIND_LABEL"
	start := strings.Index(source, opener)
	if start < 0 {
		t.Fatalf("no %s declaration in %s", opener, frontendApprovalKinds)
	}
	body := source[start:]
	if end := strings.Index(body, "\n};"); end >= 0 {
		body = body[:end]
	}
	body = tsCommentInKinds.ReplaceAllString(body, "")
	var kinds []string
	for _, match := range kindLabelEntry.FindAllStringSubmatch(body, -1) {
		kinds = append(kinds, match[1])
	}
	if len(kinds) == 0 {
		t.Fatalf("read no entries out of KIND_LABEL — the parser has stopped seeing the map")
	}
	slices.Sort(kinds)
	return slices.Compact(kinds)
}

func TestEveryStageableKindHasAFrontendLabel(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendApprovalKinds)
	if err != nil {
		t.Fatalf("reading the frontend approval-kind map: %v", err)
	}
	labelled := labelledKinds(t, string(source))
	stageable := approvals.StageableKinds()

	for _, kind := range stageable {
		if !slices.Contains(labelled, kind) {
			t.Errorf("kind %q can be staged but has no entry in KIND_LABEL: a reader would see the wire enum", kind)
		}
	}
	for _, kind := range labelled {
		if !slices.Contains(stageable, kind) {
			t.Errorf("KIND_LABEL carries %q, which no grant map can stage: a dead entry teaches the next author the map is not maintained", kind)
		}
	}
}
