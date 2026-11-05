// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"strings"
	"testing"
)

// sweptRep is the role sweep's answer for an operation whose verb it calls.
// That is what the rep was answered, and the first recorded check it lacks.
// The sweep runs the real path, so the static grant walk is read only for an
// operation no swept verb serves alone. The sweep's two files must agree. A
// check the rep lacks, beside a rep that ran the call, would hide a tool the
// rep can use.
func sweptRep(t testing.TB, op parityOperation, verbs map[string]int, in parityInputs) (passes bool, decidedBy, refusedBy string, swept bool) {
	t.Helper()
	measured, known := in.measured[op.Verb]
	checks, recorded := in.checks[op.Verb]
	if !known || verbs[op.Verb] != 1 {
		return false, "", "", false
	}
	if !recorded {
		t.Errorf("%s %s: the role sweep measured %s and toolchecks.json has no row for it, so the sweep wrote half its answer",
			op.Method, op.Path, op.Verb)
	}
	for _, check := range checks {
		grant, judged := grantOfCheck(check)
		if held, holdKnown := in.roles.holds(repRole, grant); judged && holdKnown && !held {
			if measured {
				t.Errorf("%s %s: toolchecks.json says %s needs %s, which the rep lacks, and the role sweep measured the rep "+
					"running it. Run the sweep again; one of its files is stale.", op.Method, op.Path, op.Verb, check)
			}
			return measured, decidedMeasured, grant, true
		}
	}
	return measured, decidedMeasured, "", true
}

// grantOfCheck spells a recorded check as the grant walk does. A row-scope
// check is no grant, and judged is false for it.
func grantOfCheck(check string) (grant string, judged bool) {
	if check == "admin" {
		return adminRoleGrant, true
	}
	object, actions, ok := strings.Cut(check, ".")
	if !ok || object == "*" {
		return "", false
	}
	return object + ":" + actions, true
}

// sweptChecks reads the checks table the role sweep writes.
func sweptChecks(t *testing.T) map[string][]string {
	t.Helper()
	table := readSweepFile[toolChecksFile](t, toolChecksTable)
	out := map[string][]string{}
	for verb, row := range table.Verbs {
		out[verb] = row.Checks
	}
	return out
}
