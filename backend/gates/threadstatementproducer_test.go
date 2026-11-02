// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type threadProducerDeclaration struct {
	node    ast.Node
	imports map[string]string
}
type threadProducerIndex struct {
	sqlSlots             map[string]map[int]bool
	sqlSlotPackages      map[string]bool
	restoreCatalogLoaded bool
	restoreDeclarations  []string
	t                    *testing.T
	packages             map[string]map[string]threadProducerDeclaration
	fragments            map[string]string
	edges                map[string][]string
	consumed             map[string]bool
}

func (index *threadProducerIndex) declarations(directory string) map[string]threadProducerDeclaration {
	if declarations, ok := index.packages[directory]; ok {
		return declarations
	}
	declarations := map[string]threadProducerDeclaration{}
	index.packages[directory] = declarations
	for _, file := range parsePackageFiles(index.t, directory) {
		imports := map[string]string{}
		for _, spec := range file.Imports {
			module, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				index.t.Fatal(err)
			}
			prefix := "github.com/margince/margince/backend/"
			if !strings.HasPrefix(module, prefix) {
				continue
			}
			name := module[strings.LastIndex(module, "/")+1:]
			if spec.Name != nil {
				name = spec.Name.Name
			}
			imports[name] = strings.TrimPrefix(module, prefix)
		}
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				declarations[scrubKey(receiverTypeName(declaration), declaration.Name.Name)] = threadProducerDeclaration{declaration, imports}
			case *ast.GenDecl:
				for _, spec := range declaration.Specs {
					switch spec := spec.(type) {
					case *ast.TypeSpec:
						declarations[spec.Name.Name] = threadProducerDeclaration{spec, imports}
					case *ast.ValueSpec:
						for _, name := range spec.Names {
							declarations[name.Name] = threadProducerDeclaration{spec, imports}
						}
					}
				}
			}
		}
	}
	return declarations
}

func (index *threadProducerIndex) fingerprint(directory, root string) (string, []string) {
	required, err := index.requiredRoots(directory, root)
	if err != nil {
		index.t.Fatal(err)
	}
	pending := make([]string, 0, len(required))
	if directory == threadRestoreOwner {
		for _, key := range index.restoreCatalog() {
			required[key] = true
		}
	}
	for key := range required {
		pending = append(pending, key)
	}
	if index.consumed == nil {
		index.consumed = map[string]bool{}
	}
	index.consumed[directory+":"+root] = true
	pieces := map[string]string{}
	if index.fragments == nil {
		index.fragments = map[string]string{}
		index.edges = map[string][]string{}
	}
	declarations := index.declarations(directory)
	callers := []string{root}
	inspected := map[string]bool{}
	for len(callers) > 0 {
		requested := callers[len(callers)-1]
		callers = callers[:len(callers)-1]
		if inspected[requested] {
			continue
		}
		inspected[requested] = true
		for name, declaration := range declarations {
			caller, ok := declaration.node.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ordinal := 0
			ast.Inspect(caller.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				target := threadProducerCallName(call.Fun)
				if target != requested && !strings.HasSuffix(requested, "."+target) {
					return true
				}
				callerKey := "caller:" + directory + ":" + name
				pieces[callerKey] = index.nodeFingerprint(caller)
				index.fragments[callerKey] = pieces[callerKey]

				argumentKey := "arguments:" + directory + ":" + name + ":" + requested + ":" + strconv.Itoa(ordinal)
				ordinal++
				arguments := index.argumentSources(directory, caller, call)
				catalogArguments := index.argumentSources(directory, caller, &ast.CallExpr{Fun: call.Fun, Args: index.catalogArguments(directory, requested, call)})
				if threadForwardsCatalog(caller, catalogArguments, declarations) {
					callers = append(callers, name)
				}
				argumentDeclaration := threadProducerDeclaration{node: &ast.CompositeLit{Elts: arguments}, imports: declaration.imports}
				fragment, edges := index.fragment(argumentKey, directory, name, declarations, argumentDeclaration)
				pieces[argumentKey] = fragment
				pending = append(pending, edges...)
				return true
			})
		}
	}
	seen := map[string]bool{}
	for len(pending) > 0 {
		key := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[key] {
			continue
		}
		seen[key] = true
		split := strings.LastIndex(key, ":")
		dir, name := key[:split], key[split+1:]
		declarations := index.declarations(dir)
		declaration, ok := declarations[name]
		if !ok {
			if required[key] {
				index.t.Fatalf("required producer declaration disappeared: %s", key)
			}
			continue
		}
		fragment, references := index.fragment(key, dir, name, declarations, declaration)
		pieces[key] = fragment
		pending = append(pending, references...)
	}
	names := make([]string, 0, len(pieces))
	for name := range pieces {
		names = append(names, name)
	}
	sort.Strings(names)
	var body strings.Builder
	for _, name := range names {
		body.WriteString(name + "\n" + pieces[name] + "\n")
	}
	return threadStatementFingerprint(body.String()), names
}

func (index *threadProducerIndex) fragment(key, dir, name string, declarations map[string]threadProducerDeclaration, declaration threadProducerDeclaration) (string, []string) {
	if index.fragments == nil {
		index.fragments = map[string]string{}
		index.edges = map[string][]string{}
	}
	if fragment, ok := index.fragments[key]; ok {
		return fragment, index.edges[key]
	}
	functions := map[string]*ast.FuncDecl{}
	for name, candidate := range declarations {
		if fn, ok := candidate.node.(*ast.FuncDecl); ok {
			functions[name] = fn
		}
	}
	pkg := threadSQLPackage{functions: functions}
	caller, _ := declaration.node.(*ast.FuncDecl)
	shadowed := map[string]bool{}
	if caller != nil {
		shadowed = declaredNames(caller)
		if receiver := receiverVarName(caller); receiver != "" {
			shadowed[receiver] = true
		}
	}
	var references []string
	index.fragments[key] = index.nodeFingerprint(declaration.node)

	if _, typed := declaration.node.(*ast.TypeSpec); typed {
		index.edges[key] = nil
		return index.fragments[key], nil
	}
	ast.Inspect(declaration.node, func(node ast.Node) bool {
		switch expression := node.(type) {
		case *ast.CallExpr:
			if target, ok := pkg.calledFunction(expression, caller); ok {
				references = append(references, dir+":"+scrubKey(receiverTypeName(target), target.Name.Name))
			}
		case *ast.Ident:
			if _, ok := declarations[expression.Name]; ok && !shadowed[expression.Name] {
				references = append(references, dir+":"+expression.Name)
			}
		case *ast.SelectorExpr:
			if qualifier, ok := expression.X.(*ast.Ident); ok {
				if imported, ok := declaration.imports[qualifier.Name]; ok && !shadowed[qualifier.Name] {
					references = append(references, imported+":"+expression.Sel.Name)
					return true
				}
			}

		}
		return true
	})
	index.edges[key] = references
	return index.fragments[key], references
}

func (index *threadProducerIndex) nodeFingerprint(node ast.Node) string {
	var rendered bytes.Buffer
	if err := format.Node(&rendered, token.NewFileSet(), node); err != nil {
		index.t.Fatal(err)
	}
	digest := sha256.Sum256(rendered.Bytes())
	return hex.EncodeToString(digest[:])
}

func (index *threadProducerIndex) argumentSources(directory string, caller *ast.FuncDecl, call *ast.CallExpr) []ast.Expr {
	pkg := threadSQLPackage{bindings: map[string][]ast.Expr{}}
	for name, declaration := range index.declarations(directory) {
		if value, ok := declaration.node.(*ast.ValueSpec); ok {
			pkg.bindings[name] = value.Values
		}
	}
	bindings := pkg.bindingsBefore(caller, call.Pos())
	expressions := append([]ast.Expr{}, call.Args...)
	seen := map[string]bool{}
	for position := 0; position < len(expressions); position++ {
		ast.Inspect(expressions[position], func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok || seen[identifier.Name] {
				return true
			}
			seen[identifier.Name] = true
			expressions = append(expressions, bindings[identifier.Name]...)
			return true
		})
	}
	return expressions
}

func threadProducerCallName(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.SelectorExpr:
		return expression.Sel.Name
	case *ast.IndexExpr:
		return threadProducerCallName(expression.X)
	case *ast.IndexListExpr:
		return threadProducerCallName(expression.X)
	}
	return ""
}

func (index *threadProducerIndex) writeEvidence(t *testing.T) {
	t.Helper()
	evidence := struct {
		Declarations map[string]string
		Edges        map[string][]string
	}{index.fragments, index.edges}
	encoded, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(threadEvidenceOutput(t), "thread-statement-producers.json")
	if err := os.WriteFile(destination, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("Producer dependency evidence:", destination)
}

var threadStatementProducerRoots = map[string][]string{
	"internal/modules/search:QueryExecutor.answerRows": {
		"internal/modules/search:planCompiler.compileStatement",
		"internal/modules/search:searchBranches",
		"internal/modules/search:joinTables",
	},
	"internal/modules/search:Store.Search": {
		"internal/modules/search:rankedShape.statement",
		"internal/modules/search:groupedShape.statement",
		"internal/modules/search:searchBranches",
		"internal/modules/search:branchScope",
	},
}

func (index *threadProducerIndex) requiredRoots(directory, root string) (map[string]bool, error) {
	required := map[string]bool{directory + ":" + root: true}
	for _, key := range threadStatementProducerRoots[directory+":"+root] {
		required[key] = true
	}
	for key := range required {
		split := strings.LastIndex(key, ":")
		if split < 1 || split == len(key)-1 {
			return nil, fmt.Errorf("invalid producer declaration: %s", key)
		}
		if _, exists := index.declarations(key[:split])[key[split+1:]]; !exists {
			return nil, fmt.Errorf("missing required producer declaration: %s", key)
		}
	}
	return required, nil
}

func (index *threadProducerIndex) assertRootsConsumed(t *testing.T) {
	t.Helper()
	for key := range threadStatementProducerRoots {
		if !index.consumed[key] {
			t.Errorf("producer catalog has no checked subject: %s", key)
		}
	}
}
