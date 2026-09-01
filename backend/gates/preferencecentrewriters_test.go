// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The public preference centre answers in ONE shape, and resolves "which
// address is theirs" in ONE place.
//
// Both claims are load-bearing and both are the kind that rots quietly.
//
// The RESPONSE SHAPE: the read and the save return the same body, and the
// page reads masked_email, workspace_name, choice and refused out of it.
// A second writer of that body is how one of them starts omitting a field
// — the save answering without `refused`, say — and nothing would fail:
// the client renders an absent array as "nothing refused", which is
// exactly the sentence a dropped field makes false.
//
// The PRIMARY ADDRESS: the preference centre and the confirm card both
// show a person their own address, and they must agree on which one that
// is. The ordering (is_primary DESC, created_at) is the whole rule, and a
// hand-copied second spelling that dropped the tiebreak would show two
// surfaces two different addresses for the same person — each looking
// correct on its own screen.
//
// WHAT THIS GATE CAN AND CANNOT SEE. It matches the call, by name, in the
// consent module's Go. A caller that reached the same statement through a
// variable holding the SQL, or a copy that spelled the ORDER BY without
// going through the helper, is outside what an AST walk can judge; the
// second is the real gap, and it is why the address census below counts
// the ORDER BY itself rather than the function name.

import (
	"fmt"
	"go/ast"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// consentRoot is where every spelling this gate counts must live.
const consentRoot = "internal/modules/consent"

// TestThePreferenceCentreAnswersInOneShape holds the "one spelling"
// claim on writePreferenceCenter.
func TestThePreferenceCentreAnswersInOneShape(t *testing.T) {
	t.Parallel()
	// The BODY KEYS, not the function name: a second writer would be a
	// second map literal carrying them, and renaming the helper would not
	// help it escape.
	const key = "masked_email"
	scope := gatekit.Scope{
		Roots:   []string{consentRoot},
		Subject: fileContains(key),
		Exempt:  gatekit.Waive(map[string]string{}),
	}
	total, where := countAcross(t, scope, key)
	// EXACTLY one. Zero is the direction this fails silently in — the
	// helper renamed its keys, or moved out of the module, and a gate
	// asking "no second writer" prints the same clean word over a tree
	// where the shape is no longer written at all.
	if total != 1 {
		t.Errorf("the preference-centre response body is written %d time(s), want exactly 1: %s\n\n"+
			"The read and the save must answer in one shape — a second writer is how one of them "+
			"starts omitting a field the page reads as meaningful.", total, strings.Join(where, ", "))
	}
}

// TestThePreferenceCentreResolvesOnePrimaryAddress holds the "cannot
// disagree" claim on primaryEmailTx.
func TestThePreferenceCentreResolvesOnePrimaryAddress(t *testing.T) {
	t.Parallel()
	// The ORDERING is the rule, so the ordering is what is counted. A
	// copy that named the helper but re-spelled the ORDER BY is the
	// defect; a copy that omitted the tiebreak would show a different
	// address on a person carrying two.
	const key = "pe.is_primary DESC, pe.created_at"
	scope := gatekit.Scope{
		Roots:   []string{consentRoot},
		Subject: fileContains(key),
		Exempt:  gatekit.Waive(map[string]string{}),
	}
	total, where := countAcross(t, scope, key)
	if total != 1 {
		t.Errorf("the person's primary address is resolved %d time(s) in consent, want exactly 1: %s\n\n"+
			"The preference centre and the confirm card show a person their own address and must "+
			"agree which one it is; a second spelling that drops the tiebreak shows two surfaces "+
			"two different addresses.", total, strings.Join(where, ", "))
	}
}

// TestOneSendDerivesEveryUnsubscribeDestination holds the "every
// destination" claim on unsubscribeLinks.
//
// The defect this whole change exists for was three destinations
// collapsing into one URL: the visible "Unsubscribe" link pointed at the
// POST-only machine endpoint, so a human click got 405. A second builder
// is how they drift apart again — one caller keeps the pages while
// another goes back to the API path, and each looks correct alone.
func TestOneSendDerivesEveryUnsubscribeDestination(t *testing.T) {
	t.Parallel()
	// The PUBLIC PREFIX, not the function name: every destination is built
	// by appending to it, so a second builder has to spell it too.
	const key = "/v1/public/preferences/"
	scope := gatekit.Scope{
		Roots:   []string{"internal/modules/activities"},
		Subject: fileContains(key),
		// Both spell the prefix to SERVE or to redact it, never to build a
		// link a recipient is sent. This gate is about the send path's
		// builders; a reader of the same path is not a second one.
		Exempt: gatekit.Waive(map[string]string{
			"internal/compose/publicpreferences.go":                   "the edge that serves the prefix, matching requests rather than minting links",
			"internal/shared/kernel/capabilitypath/capabilitypath.go": "the access-log redactor, which knows the prefix so a token never reaches a log line",
		}),
	}
	total, where := countAcross(t, scope, key)
	if total != 1 {
		t.Errorf("the public preference URL is built %d time(s) in the send path, want exactly 1: %s\n\n"+
			"The header, the two visible links and the redacted copy come from ONE builder; a second "+
			"is how the machine endpoint ends up behind a human link again.", total, strings.Join(where, ", "))
	}
}

// fileContains is the Subject predicate: does this file spell the string
// anywhere in its source.
func fileContains(needle string) func(string, *ast.File) bool {
	pattern := regexp.MustCompile(regexp.QuoteMeta(needle))
	return func(_ string, file *ast.File) bool {
		return countInFile(file, pattern) > 0
	}
}

// countAcross totals a needle's occurrences over a scope's files and says
// where it found them.
func countAcross(t *testing.T, scope gatekit.Scope, needle string) (int, []string) {
	t.Helper()
	pattern := regexp.MustCompile(regexp.QuoteMeta(needle))
	total := 0
	var where []string
	for _, f := range scope.Files(t) {
		n := countInFile(f.File, pattern)
		if n == 0 {
			continue
		}
		total += n
		where = append(where, fmt.Sprintf("%s (%d)", f.Path, n))
	}
	return total, where
}

// countInFile counts a pattern's matches in a file's string literals —
// where every subject here lives, and where a copy would land.
//
// Read through gatekit.LiteralText rather than off lit.Value: the source
// text carries its escapes, so a statement written in double quotes would
// not match the pattern at all and this census would report a clean tree
// over exactly the copy it exists to find.
func countInFile(file *ast.File, pattern *regexp.Regexp) int {
	total := 0
	ast.Inspect(file, func(n ast.Node) bool {
		expr, isExpr := n.(ast.Expr)
		if !isExpr {
			return true
		}
		text, isLiteral := gatekit.LiteralText(expr)
		if !isLiteral {
			return true
		}
		total += len(pattern.FindAllString(text, -1))
		return true
	})
	return total
}
