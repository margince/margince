// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

import (
	"strings"
	"testing"
)

// A catalog entry runs with NOBODY watching: it fires on a schedule, on a
// passport its owner lent once, and every tool it names it may call without
// anyone seeing the call first.
//
// That is what makes the confirm-first tier work — a 🟡 call from an unattended
// run stops and waits for a person. A run that could ANSWER an approval would
// answer its own: stage the call, release it, re-issue it, and the tier is a
// formality it walks through by itself. The queue tools exist because a person
// in a conversation can now reach their inbox from it; a scheduled run is the
// case where there is no such person.
//
// So the decide verbs are refused HERE, in the allowlist, rather than at the
// gate: a passport that may decide is exactly what an interactive caller needs,
// and it is the RUN that must not be able to spend it. A spec that grew one of
// these names would be the first unattended self-approval in the tree, and it
// would look like an ordinary line in a list of tools.
// THE TOOL HALF OF THIS CLAIM LIVES IN COMPOSE, and it has to: the allowlist
// is declared in api/ai-tasks.yaml and joined to these entries by compose, so
// Catalog() carries no tools and a loop over spec.Tools here would check
// nothing while looking exactly like it did. See
// TestNoScheduledAgentAttachesADecideVerb in internal/compose.
//
// What stays here is the GOALS, because this is where the goals are written.
func TestNoUnattendedAgentSpecCanAnswerAnApproval(t *testing.T) {
	specs := Catalog()
	if len(specs) == 0 {
		t.Fatal("the catalog is empty — this gate checked nothing")
	}
	// The goal is where the instruction to answer an approval would be written
	// if the tool ever reached one: an agent told to answer what is waiting has
	// been given the intent, and the allowlist is then the only thing standing
	// in its way.
	for _, spec := range specs {
		goal := strings.ToLower(spec.Goal)
		for _, phrase := range []string{"approve the", "approve any", "approve every", "decide_approval"} {
			if strings.Contains(goal, phrase) {
				t.Errorf("agent spec %q is told to %q — an unattended run does not answer approvals", spec.Name, phrase)
			}
		}
	}
}
