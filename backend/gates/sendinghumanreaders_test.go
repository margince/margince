// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// principal.SendingHuman has ONE reader, and it answers one question.
//
// The value names the person an outbound message goes out AS when that differs
// from the acting principal — an automation composes under the system actor
// while the mail leaves under its owner's name. It is deliberately not a
// principal: it moves no authority, no row scope and no audit attribution, and
// the only thing it may decide is whose voice a draft is written in.
//
// That restraint is the whole safety argument, and nothing but this gate holds
// it. A second reader is how such a value stops being narrow: the next author
// finds a context value that names a human, reaches for it because it is
// already there, and now something else — a permission check, a row filter, a
// send identity — turns on a value that was never authorised to decide it. The
// one existing reader already had to be reordered once, in review, because it
// consulted this before the actor and would have let one person's call read
// another person's private writing.
//
// WHAT THIS GATE CAN AND CANNOT SEE. It matches a syntactic reference to
// principal.SendingHuman, so it catches a new call site wherever it is written.
// It cannot see a reader that goes through an alias assigned at run time, and
// it says nothing about what the one permitted reader then DOES with the value —
// that is voiceSender's own tests. Adding a reader is not forbidden; doing it
// without anybody agreeing is.

import (
	"go/ast"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// principalPackage is the import path the value lives in.
const principalPackage = "github.com/margince/margince/backend/internal/shared/kernel/principal"

// sendingHumanSites are the files permitted to READ the value, each with why.
// Anything else calling principal.SendingHuman is a finding.
//
// Readers only. The setter is WithSendingHuman, a different symbol, and writing
// the value is the ordinary act this gate exists to keep safe — the automation
// engine sets it on every owned firing. What must stay rare is the set of
// questions that TURN on it.
var sendingHumanSites = gatekit.Waive(map[string]string{
	// Resolving whose voice a draft is written in is the single question this
	// value exists to answer, and voiceSender asks the acting principal first so
	// a bound sender can never redirect a human's own read.
	"internal/modules/ai/voice_draftread.go": "resolves whose voice a draft is written in",
	// The draft_email executor refuses a firing with no owner: a held draft is
	// released by the person it goes out as, so one naming nobody can be
	// released by nobody. It asks only whether an owner EXISTS and never decides
	// whose anything is — the engine put the value on this context one call
	// earlier, so re-deriving it here would be a second answer to one question.
	"internal/modules/automation/handlers_actions.go": "refuses to draft for an automation with no owner",
})

// TestTheSendingHumanHasOneReader fails when a second site reads the value.
func TestTheSendingHumanHasOneReader(t *testing.T) {
	t.Parallel()
	scope := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: readsTheSendingHuman,
		Exempt:  gatekit.Waive(map[string]string{}),
	}
	var readers []string
	reads := 0
	for _, src := range scope.Files(t) {
		readers = append(readers, src.Path)
		reads += countsSendingHumanReads(src.File)
	}
	if len(readers) == 0 {
		t.Fatal("no file reads principal.SendingHuman, so this gate is reading a tree the value has " +
			"left. Either the value is gone — delete this gate with it — or the sweep no longer " +
			"recognises its reader, which is this gate failing short rather than passing")
	}
	for _, path := range readers {
		if sendingHumanSites.Waived(t, path) {
			continue
		}
		t.Errorf("%s names principal.SendingHuman, and it is not one of the agreed sites. The value says "+
			"who a message goes out as and authorises nothing: it moves no grant, no row scope and no "+
			"audit attribution, which is what makes it safe for an automation to set from a stored "+
			"owner_id. A second consumer is how that stops being true — decide deliberately whether this "+
			"question may turn on it, then add the site to sendingHumanSites with its reason", path)
	}
	// A site that has gone is a claim about code that no longer exists, and the
	// next author reads it as governance that is still running.
	sendingHumanSites.AssertAllMatched(t)
	// The CALLS, not just the files. A per-file check passes a second read added
	// inside a file that is already listed — and a file already permitted is the
	// easiest place for one to appear, because nothing about editing it looks
	// like widening anything.
	const sendingHumanReads = 2
	if reads != sendingHumanReads {
		t.Errorf("the tree reads principal.SendingHuman %d times and this gate pins %d. More means a new "+
			"question turns on the value — the thing this gate exists to make somebody agree to. Fewer "+
			"means a read went away, or the sweep stopped recognising one, which is this gate quietly "+
			"governing less than it claims", reads, sendingHumanReads)
	}
}

// countsSendingHumanReads counts the calls to principal.SendingHuman in a file.
func countsSendingHumanReads(file *ast.File) int {
	qualifier, dotImported := gatekit.ImportedAs(file, principalPackage)
	if qualifier == "" && !dotImported {
		return 0
	}
	reads := 0
	ast.Inspect(file, func(n ast.Node) bool {
		selector, ok := n.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "SendingHuman" {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		if ok && pkg.Name == qualifier {
			reads++
		}
		return true
	})
	return reads
}

// principalDeclaration is where the value is DECLARED. Declaring is not
// reading, and a gate that counted it would report the definition as its own
// first violation — a finding nobody can act on, of the kind that gets a gate
// switched off.
const principalDeclaration = "internal/shared/kernel/principal/principal.go"

// readsTheSendingHuman selects a file that reads principal.SendingHuman.
func readsTheSendingHuman(path string, file *ast.File) bool {
	if path == principalDeclaration {
		return false
	}
	return gatekit.References(file, principalPackage, "SendingHuman")
}
