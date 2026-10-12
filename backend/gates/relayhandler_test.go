// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H2

package gates

// Every method wearing the event-relay handler shape is wired to the bus.
//
// The shape excuses a method from the store entry-point auth gate, because the
// bus has no caller authority to admit. So each one is held to being
// registered as a consumer, or dispatched by a registered sibling.
//
// H2: the walk matches the method name a registration passes by value. A
// handler registered through an interface, a stored field or a closure is
// invisible to it, and would report here as unwired rather than pass.

import (
	"go/ast"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// isRelayHandler reports whether this method is an events.Handler: the
// signature the relay delivers one envelope through.
//
// Such a method is entered by the bus, so there is no caller authority to
// admit. It binds the system principal its own writes run under.
//
// An admission check here would test the context the method just built, and
// could never refuse. The admission that binds is on the module stores it
// delegates to, where this gate already asks for it.
//
// Matched on the parameter's type name rather than its qualifier. The tree
// imports the kernel envelope under two aliases, and both are one type.
func isRelayHandler(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil || len(fn.Type.Params.List) != 2 {
		return false
	}
	if !takesContext(fn) || !returnsOnlyError(fn) {
		return false
	}
	sel, ok := fn.Type.Params.List[1].Type.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Envelope"
}

// declaresRelayHandler reports whether the file holds one, which is what the
// scope above keys its root-completeness check on.
func declaresRelayHandler(path string, file *ast.File) bool {
	if isIntegrationTagged(path) {
		return false
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Recv != nil && fn.Name.IsExported() && isRelayHandler(fn) {
			return true
		}
	}
	return false
}

// returnsOnlyError holds the other half of the Handler signature. A method
// answering a value as well as an error is doing something the relay cannot
// consume, whatever its second parameter is.
func returnsOnlyError(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
		return false
	}
	ident, ok := fn.Type.Results.List[0].Type.(*ast.Ident)
	return ok && ident.Name == "error"
}

// TestEveryRelayHandlerIsRegisteredWithASubscriber is the counterweight to the
// exemption above.
//
// The shape is cheap to wear. A method taking a context and an envelope is
// excused the admission question, and nothing in the signature says the bus
// calls it.
//
// So each one is held to being registered, by name, where the worker binds its
// consumer groups. A service method that merely looks like a handler is
// reported here rather than excused.
func TestEveryRelayHandlerIsRegisteredWithASubscriber(t *testing.T) {
	t.Parallel()

	registered := subscriberRegistrations(t)
	dispatched := map[string]map[string]bool{}
	found := 0
	// The Scope answers the question this census cannot. A handler declared
	// outside these two tiers is reported as living under no root, rather
	// than going uncounted.
	handlerFiles := gatekit.Scope{
		Roots:   []string{composeTierDir, modulesDir},
		Subject: declaresRelayHandler,
	}
	// Two passes, because a dispatcher and the handlers it calls sit in
	// different files of one package.
	for _, src := range handlerFiles.Files(t) {
		dir := filepath.ToSlash(filepath.Dir(src.Path))
		if dispatched[dir] == nil {
			dispatched[dir] = map[string]bool{}
		}
		collectDispatched(dispatched[dir], src.File, registered)
	}
	for _, src := range handlerFiles.Files(t) {
		for _, decl := range src.File.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || !fn.Name.IsExported() || !isRelayHandler(fn) {
				continue
			}
			found++
			if registered[fn.Name.Name] || dispatched[filepath.ToSlash(filepath.Dir(src.Path))][fn.Name.Name] {
				continue
			}
			t.Errorf("%s: %s wears the events.Handler shape, so the auth gate excuses it, "+
				"but no subscriber in cmd/worker registers it and no registered handler in its "+
				"file calls it: either wire it to the bus or give it the admission its callers need",
				src.Path, fn.Name.Name)
		}
	}
	// A reader of this gate is told when its subject has emptied out. Nothing
	// fails if the corpus is zero, and a zero corpus is what a broken walk
	// looks like.
	if found < fewestRelayHandlers {
		t.Errorf("found %d relay handlers, fewer than the %d this gate was measured against: "+
			"the walk reads a smaller tree than it did, which reports PASS with nothing to notice",
			found, fewestRelayHandlers)
	}
}

// fewestRelayHandlers is the floor the census was measured against. It may
// fall when a consumer is retired. The number is here so a walk that stops
// reading the tree fails instead of passing.
const fewestRelayHandlers = 18

// collectDispatched records what the registered handlers in one package call,
// so a handler the bus reaches one step in is accepted.
//
// The engine surface is built that way. One registered HandleEvent switches on
// the envelope and calls a handler per event, from another file of the same
// package.
//
// Demanding direct registration would report those as unreachable, which is the
// opposite of true.
func collectDispatched(into map[string]bool, file *ast.File, registered map[string]bool) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !registered[fn.Name.Name] {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				into[sel.Sel.Name] = true
			}
			return true
		})
	}
}

// subscriberRegistrations collects the method names cmd/worker hands to a
// subscriber. It reads selector expressions rather than resolving types: the
// registration is always a method value passed by name.
func subscriberRegistrations(t *testing.T) map[string]bool {
	t.Helper()

	names := map[string]bool{}
	err := filepath.WalkDir(workerCmdDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		file, parseErr := gatekit.ParseFile(path, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isSubscriberRun(call) {
				return true
			}
			for _, arg := range call.Args {
				if sel, ok := arg.(*ast.SelectorExpr); ok {
					names[sel.Sel.Name] = true
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("reading the worker's subscriber registrations: %v", err)
	}
	return names
}

// isSubscriberRun matches the worker's own spellings for binding a consumer
// group. The prefix is what they share, so a fourth lane inherits the rule by
// being named like its neighbours.
func isSubscriberRun(call *ast.CallExpr) bool {
	ident, ok := call.Fun.(*ast.Ident)
	return ok && strings.HasPrefix(ident.Name, "run") && strings.Contains(ident.Name, "Subscriber")
}

const workerCmdDir = "cmd/worker"
