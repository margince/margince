// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Which tools a passport is offered, on the scope axis.

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// A passport scoped only to write is still told who it writes as.
//
// The write tools' arguments say to write stored prose in the language whoami
// reports, so a caller offered them and refused whoami is handed an
// instruction it cannot follow — it spends a call finding that out and then
// guesses the language anyway, which is the guess the instruction exists to
// remove. Scopes are exact membership, so write does not imply read and this
// is not hypothetical.
func TestAWriteOnlyPassportIsStillOfferedItsOwnIdentity(t *testing.T) {
	offered := func(scopes ...principal.Scope) []string {
		ctx := agentHolding(scopes...)
		var names []string
		for _, spec := range []mcp.ToolSpec{whoami{}.Spec(), logActivity{}.Spec()} {
			if invocableByCaller(ctx, spec) {
				names = append(names, spec.Name)
			}
		}
		return names
	}

	writeOnly := offered(principal.ScopeWrite)
	if !slices.Contains(writeOnly, "whoami") {
		t.Errorf("a write-only passport is offered %v — whoami answers its own seat and must be among them", writeOnly)
	}
	if !slices.Contains(writeOnly, "log_activity") {
		t.Errorf("a write-only passport is offered %v, without the write tool it is scoped for", writeOnly)
	}

	// The widening is whoami's alone: a read-only passport must still be
	// refused the tools that write.
	readOnly := offered(principal.ScopeRead)
	if slices.Contains(readOnly, "log_activity") {
		t.Errorf("a read-only passport is offered %v — a write tool must not be among them", readOnly)
	}
}

// TestTheLinksArgumentSaysWhatAMeetingIsAbout holds the fact that decides
// whether a logged meeting lands on a timeline anybody reads.
//
// A meeting is with a PERSON and reaches their company through them. Linked to
// the deal alone it sits on no attendee's timeline and the company sees
// nothing, and putting it right afterwards is a second write. The schema is
// where that is decided, because it is what the caller reads before choosing —
// so the copy has to carry the modelling fact, not only the instruction.
func TestTheLinksArgumentSaysWhatAMeetingIsAbout(t *testing.T) {
	t.Parallel()
	schema := string(logActivity{}.Spec().InputSchema)
	for _, want := range []string{
		// All of them, in this call.
		"ALL OF THEM in this call",
		// A meeting is with a PERSON, and the company follows from that.
		"with a PERSON",
		"reaches their company through them",
		// What a deal-only link actually costs, which is the mistake this copy
		// exists to stop — on both sides, because the person's timeline and the
		// company's are two facts and losing either leaves the copy half true.
		"no attendee's timeline",
		"the company sees nothing",
		// And what doing it in two calls costs instead, including the one
		// destination that still waits on a person — said as what it does to
		// the link, not only that somebody is asked.
		"a second write",
		"stages an approval a human must decide before it takes effect",
	} {
		if !strings.Contains(schema, want) {
			t.Errorf("the links argument no longer says %q:\n%s", want, schema)
		}
	}
}
