// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package backendarch

// The jittered retry ladder is spelled once, in shared/kernel/backoff.
//
// It was spelled twice — the capture registry's and the overlay sweep's —
// character for character, down to the same //nolint comment on the same line
// of each. They are sibling modules and cannot import one another, so the copy
// happened for the honest reason, and the two had every opportunity to drift
// without anybody noticing.
//
// The JITTER is what makes this worth a gate rather than a tidy-up. Doubling is
// arithmetic anyone would rewrite the same way and a mistake in it shows up in
// the first test. The ±20% spread is a decision, and it is the one that stops a
// fleet which failed together from retrying together: one provider outage,
// every connection backing off on the identical schedule, and the recovery
// arriving as a thundering herd against a provider that has just come back up.
// A copy that drifted to ±5%, or dropped the jitter as noise, would look
// correct in review and would only ever be visible during an outage — which is
// the worst moment to discover it.
//
// WHAT THIS GATE CAN AND CANNOT SEE. The subject is a call to math/rand's
// Float64 — the source every jitter in this tree draws from. It cannot see
// jitter built from a different rand function (IntN, N), nor a spread computed
// some other way entirely. It is a net under the one shape the tree reaches
// for, not a proof, and the tree holds exactly zero of them outside the owner
// today.

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
)

// jitteredBackoffScope claims the randomness lives in kernel/backoff. Nothing
// is exempt: kernel sits below every tier that schedules a retry, so every
// caller can reach backoff.Jittered.
var jitteredBackoffScope = gatekit.Scope{
	Roots:   []string{"internal/shared/kernel/backoff"},
	Subject: drawsSchedulingRandomness,
	Exempt:  gatekit.Waive(map[string]string{}),
}

func TestTheJitteredLadderIsSpelledOnce(t *testing.T) {
	inside := jitteredBackoffScope.Files(t)
	if len(inside) > 1 {
		var where []string
		for _, f := range inside {
			where = append(where, f.Path)
		}
		t.Errorf("scheduling randomness is drawn in %d files inside the package that owns it:\n\t%s\n\n"+
			"One ladder, so two schedulers cannot come to disagree about how far apart to spread a fleet "+
			"that failed together. Call backoff.Jittered with your own bounds", len(inside),
			strings.Join(where, "\n\t"))
	}
}

// drawsSchedulingRandomness reports whether a file calls math/rand's Float64.
//
// The import path decides what the local name means: a file that aliases the
// package, or one with an unrelated `rand` of its own, would be read wrong by a
// name match alone.
func drawsSchedulingRandomness(_ string, file *ast.File) bool {
	local, imported := localNameForMathRand(file)
	if !imported {
		return false
	}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fn, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || fn.Sel.Name != "Float64" {
			return true
		}
		if pkg, ok := fn.X.(*ast.Ident); ok && pkg.Name == local {
			found = true
		}
		return !found
	})
	return found
}

// localNameForMathRand returns the name this file calls math/rand by, honouring
// an alias, and whether it imports it at all. Both the v1 and v2 paths count:
// they are two spellings of one package, and a gate that knew only the one the
// tree uses today would go quiet on the day it moved.
func localNameForMathRand(file *ast.File) (string, bool) {
	for _, imported := range file.Imports {
		path := strings.Trim(imported.Path.Value, `"`)
		if path != "math/rand" && path != "math/rand/v2" {
			continue
		}
		if imported.Name != nil {
			return imported.Name.Name, true
		}
		return "rand", true
	}
	return "", false
}

// TestTheJitterCensusStillSeesItsSubject is the vacuity check: a census that
// has stopped matching passes by finding nothing, which is the same word it
// prints over a clean tree.
func TestTheJitterCensusStillSeesItsSubject(t *testing.T) {
	subjects := map[string]string{
		"a jitter multiplier drawn the way this tree draws one": "" +
			"package p\nimport \"math/rand/v2\"\nfunc f() float64 { return 0.8 + 0.4*rand.Float64() }",
		"the same draw from the v1 package": "" +
			"package p\nimport \"math/rand\"\nfunc f() float64 { return rand.Float64() }",
		"a draw through an aliased import": "" +
			"package p\nimport mr \"math/rand/v2\"\nfunc f() float64 { return mr.Float64() }",
	}
	for name, body := range subjects {
		if !drawsSchedulingRandomness("x.go", parseGateFixture(t, body)) {
			t.Errorf("the census no longer recognises %s, so it is guarding nothing", name)
		}
	}

	nearMisses := map[string]string{
		"a Float64 on something that is not math/rand": "" +
			"package p\nfunc f(row pgx.Row) float64 { return row.Float64() }",
		"a file that names rand without importing it": "" +
			"package p\nfunc f(rand source) float64 { return rand.Float64() }",
		"a different rand function, which this gate says it cannot see": "" +
			"package p\nimport \"math/rand/v2\"\nfunc f() int { return rand.IntN(10) }",
	}
	for name, body := range nearMisses {
		if drawsSchedulingRandomness("x.go", parseGateFixture(t, body)) {
			t.Errorf("the census claims %s draws scheduling randomness", name)
		}
	}
}
