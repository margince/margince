// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H3

package gates

// Two drafting tools keep their drafts in different places ON PURPOSE, and each
// says so beside the other's name.
//
// `draft_email` drafts one first message for one recipient and leaves it in the
// composer's saved drafts (mail_draft) for the human the agent acts for. It is
// kept off the timeline because the timeline records what happened, and an
// unsent draft has not happened.
//
// `draft_follow_ups_for` writes each draft to its deal's timeline. It drafts N
// for later triage on the deals they are about, and the draft id it answers is
// how a human finds them.
//
// AGENTS.md: two writers of one invariant either share a helper or say why they
// do not — in the code beside it. These do not share, so each site names the
// other, and this holds the CROSS-REFERENCE rather than the prose: a gate
// matching sentences fails on a rewording, and a reader who finds one site
// without the other takes its answer for the rule everywhere.

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
			file:     "backend/internal/compose/agentdraftseam.go",
			mustName: "FollowUpDrafter",
			why: "the tool that files on the timeline. Without the pointer, a reader finding this one " +
				"alone reads 'a draft stays off the timeline' as a rule the other site breaks",
		},
		{
			file:     "backend/internal/modules/agents/tools_slipping.go",
			mustName: "DraftCompanyEmail",
			why: "the tool that keeps its draft in the saved drafts. Without the pointer, a reader " +
				"finding this one alone reads the timeline write as what drafting means everywhere",
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
