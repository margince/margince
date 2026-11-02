// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every River job declares its role, and the declaration is the contract: a job
// either does tenant work for one workspace (jobs.WorkspaceScoped, method
// WorkspaceID) or only scans and enqueues (jobs.FleetWide). A job
// that declares neither is the shape this gate exists to prevent — an
// inline `for each workspace` loop inside one job row, whose per-workspace
// failures have nowhere durable to land, so River records success while
// tenants silently failed.
//
// api/jobs.yaml states that role per kind and jobkinds_gen.go turns each
// statement into a compile-time assertion, so the role of a DECLARED kind is
// the compiler's to check. What is left here is the two halves those generated
// assertions provably cannot reach — a type the contract has never heard of,
// and a type that answers to both roles at once.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// jobArgsFloor guards against a vacuous pass: a walker that silently
// matched nothing would otherwise report green. The tree holds ~30 job
// kinds before the dispatcher split and more after; this floor only has to
// be low enough never to false-alarm.
// It falls with ADR-0103: collapsing the workspace dispatchers retires 27 child
// kinds, and a child that carried a Workspace arg was one of the kinds this
// census inspected.
const jobArgsFloor = 20

// goFilesUnder returns every hand-written .go file beneath root.
//
// Walked RECURSIVELY, not globbed: compose grows subpackages under a
// named-trigger policy (compose/briefs is the pilot), and a job args type
// or a worker in one of them would be invisible to a flat glob — a gate
// with a blind spot that widens every time the tree does.
func goFilesUnder(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	return paths, err
}

// parseGoFilesUnder parses every hand-written Go file beneath dir.
func parseGoFilesUnder(t *testing.T, dir string) (*token.FileSet, []*ast.File) {
	t.Helper()
	paths, err := goFilesUnder(dir)
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	fset := gatekit.SourceFileSet()
	files := make([]*ast.File, 0, len(paths))
	for _, path := range paths {
		file, err := gatekit.ParseFile(path, parser.ParseComments)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		files = append(files, file)
	}
	return fset, files
}

// methodsByType returns, per declared type in dir, the set of method names
// on it.
func methodsByType(t *testing.T, dir string) map[string]map[string]bool {
	t.Helper()
	_, files := parseGoFilesUnder(t, dir)
	byType := map[string]map[string]bool{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			recv := receiverTypeName(fn)
			if recv == "" {
				continue
			}
			if byType[recv] == nil {
				byType[recv] = map[string]bool{}
			}
			byType[recv][fn.Name.Name] = true
		}
	}
	return byType
}

// declaredKindTypes reads the registration markers the compiler requires.
func declaredKindTypes(t *testing.T) map[string]bool {
	t.Helper()
	path := filepath.Join("internal", "compose", "jobkinds_gen.go")
	file, err := gatekit.ParseFile(path, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	marker := jobRegistrationMarker(t, file)
	declared := map[string]bool{}
	for _, declaration := range file.Decls {
		method, ok := declaration.(*ast.FuncDecl)
		if ok && method.Recv != nil && method.Name.Name == marker {
			declared[promptReceiverType(method)] = true
		}
	}
	if len(declared) < jobArgsFloor {
		t.Fatalf("registration census found only %d declared args types", len(declared))
	}
	return declared
}

// Composed kinds are unknown to the committed generator and use two runtime carriers.
// Boot validates those carriers against the composed catalog before River starts.
var composedJobArgsTypes = map[string]bool{
	"extJobDispatcherArgs": true,
	"extJobWorkspaceArgs":  true,
}

// TestEveryJobArgsTypeIsDeclaredInTheContract is the half the generated
// assertions cannot reach. They name only kinds the contract already carries,
// so a type someone added and merely ENQUEUED — Runner.Enqueue takes a bare
// river.JobArgs, and cmd/api holds seven inserter handles — would be invisible
// to them. This walks the tree instead, so the type existing at all is enough.
func TestEveryJobArgsTypeIsDeclaredInTheContract(t *testing.T) {
	t.Parallel()
	byType := methodsByType(t, filepath.Join("internal", "compose"))
	declared := declaredKindTypes(t)

	found := 0
	for typeName, methods := range byType {
		if !methods["Kind"] {
			continue
		}
		found++
		if composedJobArgsTypes[typeName] {
			continue
		}
		if !declared[typeName] {
			t.Errorf("%s is a River job (it declares Kind()) but is not in api/jobs.yaml. Add it there and run `make gen` — an undeclared kind runs on River's one-minute default and is invisible to both job surfaces.", typeName)
		}
	}
	if found < jobArgsFloor {
		t.Fatalf("found only %d job args types, expected at least %d — the walker matched nothing and this gate would pass vacuously", found, jobArgsFloor)
	}
}

// TestNoJobArgsDeclaresBothRoles holds what the generated assertions cannot:
// Go has no negative constraint, so `var _ jobs.FleetWide = T{}` is satisfied
// just as happily by a type that ALSO implements WorkspaceID. Only a walker
// sees both at once.
func TestNoJobArgsDeclaresBothRoles(t *testing.T) {
	t.Parallel()
	byType := methodsByType(t, filepath.Join("internal", "compose"))
	roled := 0
	for typeName, methods := range byType {
		scoped, fleet := methods["WorkspaceID"], methods["FleetWide"]
		if !scoped && !fleet {
			continue
		}
		roled++
		if scoped && fleet {
			t.Errorf("%s declares both WorkspaceID() and FleetWide(): a job does one workspace's work or dispatches, never both", typeName)
		}
	}
	if roled < jobArgsFloor {
		t.Fatalf("found only %d types declaring a role, expected at least %d — the walker matched almost nothing and this gate would pass vacuously", roled, jobArgsFloor)
	}
}
