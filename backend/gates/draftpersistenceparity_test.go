// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H3

package gates

// Two drafting tools answer the persistence question differently ON PURPOSE,
// and each says so beside the other's name.
//
// `draft_email` returns text and files nothing — the same answer the HTTP draft
// endpoint gives, which is the one the web app's own button calls. It drafts
// ONE message meant to be sent now, and drafting proposes words while sending
// is the separate consent-gated act.
//
// `draft_follow_ups_for` writes each draft to the deal's timeline. It drafts N
// for later triage, and the draft id it answers is the whole point: nobody
// triages ten drafts out of a chat scrollback.
//
// AGENTS.md: two writers of one invariant either share a helper or say why they
// do not — in the code beside it, not in the PR where the next reader will not
// see it. These legitimately do not share, so the saying why is the obligation,
// and an undocumented deliberate inconsistency reads exactly like an oversight.
// That is how the ticket behind this came to exist.
//
// So this holds the CROSS-REFERENCE rather than the prose. A gate matching
// sentences fails on a rewording; what must not rot is that each site still
// points at the other, because a reader who finds one and not the other draws
// the wrong conclusion — which is the whole failure.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEachDraftingToolNamesTheOneThatAnswersDifferently(t *testing.T) {
	t.Parallel()

	for _, pair := range []struct{ file, mustName, why string }{
		{
			file:     "backend/internal/compose/comms.go",
			mustName: "FollowUpDrafter",
			why: "the tool that DOES persist. Without the pointer, a reader finding this one " +
				"alone reads 'drafting is not a write' as a rule the other site breaks",
		},
		{
			file:     "backend/internal/modules/agents/tools_slipping.go",
			mustName: "DraftAccountEmail",
			why: "the tool that does NOT persist. Without the pointer, a reader finding this one " +
				"alone reads the timeline write as what drafting means everywhere",
		},
	} {
		raw, err := os.ReadFile(filepath.Join(repoRoot, pair.file))
		if err != nil {
			t.Errorf("reading %s: %v", pair.file, err)
			continue
		}
		if !strings.Contains(string(raw), pair.mustName) {
			t.Errorf("%s no longer names %s — %s", pair.file, pair.mustName, pair.why)
		}
	}
}
