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
	for _, src := range scope.Files(t) {
		readers = append(readers, src.Path)
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
