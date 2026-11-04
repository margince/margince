// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A voice learns from its owner's sent mail only when the owner presses the
// button. The press is the consent, so the only door is the request handler.
// A worker job, a schedule or a connect hook holding the learn or the reader
// would read mail nobody asked to be read. This census finds every call of
// either in production code and holds each to its one allowed place.

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// sentMailDoors names each identifier that reads or arms the reading of sent
// mail, with the one file and function allowed to call it.
var sentMailDoors = map[string]struct{ file, caller string }{
	"LearnFromSentMail": {"internal/modules/ai/handlers_voice.go", "LearnVoiceFromSentMail"},
	"WithSentMail":      {"internal/modules/ai/voicesentmail.go", "WithVoiceSentMail"},
	"WithVoiceSentMail": {"internal/compose/server.go", ""},
	"voiceSentMail":     {"internal/compose/server.go", ""},
}

// sentMailCall is one call of a door: the file and enclosing function it is in.
type sentMailCall struct{ name, file, caller string }

func TestSentMailIsReadOnlyThroughTheOwnersPress(t *testing.T) {
	t.Parallel()
	scope := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: func(_ string, file *ast.File) bool { return len(sentMailCallsIn("", file)) > 0 },
		Exempt:  gatekit.Waive(map[string]string{}),
	}
	seen := map[string]int{}
	for _, f := range scope.Files(t) {
		for _, call := range sentMailCallsIn(f.Path, f.File) {
			seen[call.name]++
			door := sentMailDoors[call.name]
			if !strings.HasSuffix(call.file, door.file) || (door.caller != "" && call.caller != door.caller) {
				t.Errorf("%s calls %s from %s; only %s %s may.\n\tSent mail is read on the owner's press "+
					"alone, so no other code path calls the learn or holds the reader.",
					call.file, call.name, call.caller, door.file, door.caller)
			}
		}
	}
	// One each: a door nobody calls means the wiring moved and this census
	// reads a tree without its subject.
	for name := range sentMailDoors {
		if seen[name] != 1 {
			t.Errorf("%s has %d production call sites, want exactly 1", name, seen[name])
		}
	}
}

// sentMailCallsIn lists the calls of a door in a file, each with the function
// that holds it.
func sentMailCallsIn(path string, file *ast.File) []sentMailCall {
	var calls []sentMailCall
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := ""
			switch callee := call.Fun.(type) {
			case *ast.SelectorExpr:
				name = callee.Sel.Name
			case *ast.Ident:
				name = callee.Name
			}
			if _, door := sentMailDoors[name]; door {
				calls = append(calls, sentMailCall{name: name, file: path, caller: fn.Name.Name})
			}
			return true
		})
	}
	return calls
}
