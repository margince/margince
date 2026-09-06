// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every automation says whether running it twice repeats something.
//
// `workflow.Handler.Apply` is documented "idempotent on IdempotencyKey(ev)",
// and nothing enforces it: no Apply in the tree reads that key. The promise
// costs nothing while every run happens once, and becomes load-bearing the
// moment anything re-drives one — a retry then means "the row lands where it
// already was" for one handler and "the customer is told a second time" for
// another, and the caller cannot tell which.
//
// The answer lives on the handler because the handler owns Apply. Keying it on
// the ACTION KIND instead reads as tidier and is wrong: leadRouting plans
// assign_owner and applies it through RouteLead, while the engine's own
// handlers plan the same kind and apply it through applyAssignOwner. One
// vocabulary, two writes.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// judged names every handler whose re-drive question has been ANSWERED on
// purpose, with the reason, and says what the answer is.
//
// A handler absent from here fails the census below rather than defaulting
// quietly: the zero value of Spec.RedrivableWithoutDuplicating is false, which
// is the safe answer, and a safe answer nobody chose is still nobody's answer.
// The map is what turns "false because unconsidered" into "false because of
// this".
var judged = map[string]struct {
	redrivable bool
	because    string
}{
	// The three clock reminders and the two stage-change starters mint ONE
	// create_task and nothing else (taskeffect.go), applied through
	// ApplyActions, whose create arm claims (handler, occurrence_key,
	// fingerprint) before writing. A second pass loses the claim and folds.
	"no_activity_reminder":     {true, "one claimed create_task; a repeat folds on the effect claim"},
	"check_in_cadence":         {true, "one claimed create_task"},
	"renewal_reminder":         {true, "one claimed create_task"},
	"stage_change_create_task": {true, "one claimed create_task"},
	"route_lead":               {true, "one claimed create_task"},

	// These two plan a side effect nothing folds.
	"stage_change_notify": {false, "plans notify; applyNotify calls the transport unconditionally, so a repeat tells somebody twice"},
	"post_meeting_recap":  {false, "plans draft_email; a repeat composes a second draft and stages a second send behind it"},

	// The people and activities handlers bypass ApplyActions entirely and write
	// through their own stores, so the engine's effect claim does not reach
	// them. Each needs its own answer, and only one has been established.
	"recompute_lead_score":           {true, "recomputes a score FROM the records it reads; a second pass over unchanged records lands on the same number"},
	"recompute_lead_score_on_update": {true, "the same recompute on a different trigger"},

	// Not yet audited. FALSE here is a decision to refuse until somebody looks,
	// which is a different thing from the zero value meaning nobody has.
	"assign_lead_owner":                      {false, "RouteLead assigns with no IfVersion, so a delayed repeat can overwrite a reassignment a human made in between"},
	"lead_first_response":                    {false, "writes an SLA stamp through its own store; not audited for a repeat"},
	"lead_status_ladder":                     {false, "writes a status transition through its own store; not audited for a repeat"},
	"follow_up_auto_resolve":                 {false, "closes tasks through its own store; not audited for a repeat"},
	"follow_up_auto_resolve_on_promoted":     {false, "the same resolve on a different trigger"},
	"follow_up_auto_resolve_on_disqualified": {false, "the same resolve on a different trigger"},
	"follow_up_owner_reconcile":              {false, "reassigns follow-ups through its own store; not audited for a repeat"},
}

// TestEveryAutomationAnswersWhetherItMayRunTwice derives its corpus from the
// constructors compose actually registers, so a handler added to the wiring and
// not here fails on arrival.
func TestEveryAutomationAnswersWhetherItMayRunTwice(t *testing.T) {
	t.Parallel()
	handlers := everyRegisteredHandler()
	if len(handlers) == 0 {
		t.Fatal("no handlers were built, so this census would pass over nothing")
	}
	seen := map[string]bool{}
	for _, h := range handlers {
		spec := h.Spec()
		seen[spec.Name] = true
		answer, ok := judged[spec.Name]
		if !ok {
			t.Errorf("%q has not been judged: decide whether applying its effect a second "+
				"time repeats anything, and say so in `judged` with the reason", spec.Name)
			continue
		}
		if spec.RedrivableWithoutDuplicating != answer.redrivable {
			t.Errorf("%q declares redrivable=%v and the census says %v (%s) — the two have "+
				"to agree, or a caller reads one and a reviewer reads the other",
				spec.Name, spec.RedrivableWithoutDuplicating, answer.redrivable, answer.because)
		}
	}
	// And nothing lingers: a handler removed from the wiring must not keep a
	// judgement here, which would read as coverage of something that is gone.
	for name := range judged {
		if !seen[name] {
			t.Errorf("the census judges %q, which compose no longer registers", name)
		}
	}
}

// A judgement carries a reason, because "false" alone cannot be told from
// "nobody looked" — which is the exact confusion this whole file removes.
func TestEveryJudgementSaysWhy(t *testing.T) {
	t.Parallel()
	for name, answer := range judged {
		if answer.because == "" {
			t.Errorf("%q is judged with no reason, so the next reader cannot tell a decision "+
				"from a default", name)
		}
	}
}

// everyRegisteredHandler builds the handlers compose wires, from the same
// constructors. Nil stores are safe: Spec() reads none of them, and this census
// asks only what each handler DECLARES.
func everyRegisteredHandler() []workflow.Handler {
	handlers := automation.StarterWorkflows(automation.Executors{})
	handlers = append(handlers, people.LeadRoutingWorkflow(nil))
	handlers = append(handlers, people.LeadScoreWorkflows(nil)...)
	handlers = append(handlers, people.LeadSLAWorkflows(nil)...)
	handlers = append(handlers, activities.FollowUpWorkflows(nil)...)
	return append(handlers, activities.OwnershipWorkflows(nil)...)
}
