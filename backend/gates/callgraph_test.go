// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The call graph two censuses share: which SQL statements a function can reach
// and which functions it can call, keyed so a method is told from a plain
// function of the same name.
//
// It lives on its own because there are two callers now — the privacy census
// (which tables an erase and an anonymize each clear) and the organization
// rename census (whether every name write reaches the duplicate re-check) — and
// a graph copied for the second drifts from the first. The narrower copy then
// walks a smaller tree and says PASS, which is the failure a census cannot
// report about itself.
//
// It yields STATEMENTS rather than tables: what a statement means is the
// caller's question, and the privacy census's answer (which tables it writes)
// is not the rename census's (whether it sets a name column).

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// graphFunc is one function's reach: the statements its body can read, the
// functions it calls, and the identifiers it merely NAMES.
//
// `reads` is not redundant with `calls`. A statement held in a package-level
// var reaches the function that executes it by name alone, and a census that
// followed only calls attributed that statement to nobody.
type graphFunc struct {
	statements []string
	calls      map[string]bool
	reads      map[string]bool
	// shadowed are names this function DECLARES, so a read of one is a read of
	// its own binding and not of the package value that happens to share the
	// spelling.
	shadowed map[string]bool
	// hidden are package-level names this function both declares AND reads,
	// where the package value holds statements. Suppression is function-wide
	// rather than lexical, so a `query := …` inside one block silences a read of
	// the package's `query` in another — and for a census asking "does this
	// function write X" that is the direction that MISSES a writer.
	//
	// So the approximation reports itself. A caller can ask whether its verdict
	// rests on a name it could not read, instead of the walk deciding quietly
	// that it did not matter.
	hidden []string
}

// declaredNames collects every identifier a function binds: its parameters and
// named results, and everything a `:=`, a `var`, a `range` or a type switch
// introduces in its body.
//
// It does not model SCOPE — a name declared inside one block shadows the
// package value for the whole function as far as this is concerned. That is the
// conservative direction for the privacy census, which compares two sides and
// would rather miss a statement on both than add one to the side that does not
// write it, and it costs the rename census a writer only if somebody names a
// local after a package-level SQL variable.
func declaredNames(fn *ast.FuncDecl) map[string]bool {
	names := map[string]bool{}
	add := func(expr ast.Expr) {
		if ident, ok := expr.(*ast.Ident); ok && ident.Name != "_" {
			names[ident.Name] = true
		}
	}
	for _, list := range []*ast.FieldList{fn.Type.Params, fn.Type.Results} {
		if list == nil {
			continue
		}
		for _, field := range list.List {
			for _, name := range field.Names {
				add(name)
			}
		}
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch v := node.(type) {
		case *ast.AssignStmt:
			if v.Tok == token.DEFINE {
				for _, lhs := range v.Lhs {
					add(lhs)
				}
			}
		case *ast.ValueSpec:
			for _, name := range v.Names {
				add(name)
			}
		case *ast.RangeStmt:
			add(v.Key)
			add(v.Value)
		case *ast.TypeSwitchStmt:
			if assign, ok := v.Assign.(*ast.AssignStmt); ok {
				for _, lhs := range assign.Lhs {
					add(lhs)
				}
			}
		case *ast.FuncLit:
			for _, list := range []*ast.FieldList{v.Type.Params, v.Type.Results} {
				if list == nil {
					continue
				}
				for _, field := range list.List {
					for _, name := range field.Names {
						add(name)
					}
				}
			}
		}
		return true
	})
	return names
}

// packageCallGraph reads one package's product files, keyed by RECEIVER TYPE
// and name. Statements held in package-level `var`/`const` declarations are
// folded into whichever function NAMES them, so a caller sees one set of
// statements rather than having to join two.
//
// Bare names are not enough. `apply` is a method on one service and also a
// plausible name elsewhere; following calls by name alone once walked from the
// privacy eraser into the policy store and reported a table an erase does not
// clear. So an edge is followed only when it can be resolved: a plain function,
// or a method called on the CALLER'S OWN receiver.
//
// AN UNFOLLOWED EDGE IS NOT SAFE. A call on a stored field, an interface or a
// closure is a real limit, and the honest thing to say about it is that it can
// hide a statement from whichever caller takes that route — never that the
// route carries nothing.
func packageCallGraph(t *testing.T, dir string) map[string]*graphFunc {
	t.Helper()
	files := parsePackageFiles(t, dir)
	held := packageLevelStatements(files)
	graph := map[string]*graphFunc{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			recvType, recvVar := receiverTypeName(fn), receiverVarName(fn)
			entry := &graphFunc{calls: map[string]bool{}, reads: map[string]bool{}}
			// What this function DECLARES is its own, whatever the package
			// calls something of the same name. Attributing by spelling alone
			// handed a package-level statement to any function with a local of
			// that name — a false writer here, and worse in the privacy census,
			// where an unrelated shadow adds a table to the side that does NOT
			// clear it and so hides a real divergence.
			shadowed := declaredNames(fn)
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				// Through the package's shared value reader, so a statement
				// written in double quotes with escapes decodes rather than
				// being matched as source text, and one assembled with `+` is
				// read whole.
				if expr, isExpr := node.(ast.Expr); isExpr {
					if statement, readable := stringValue(expr, nil); readable {
						entry.statements = append(entry.statements, statement)
					}
				}
				if ident, isIdent := node.(*ast.Ident); isIdent {
					entry.reads[ident.Name] = true
				}
				call, isCall := node.(*ast.CallExpr)
				if !isCall {
					return true
				}
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					entry.calls[fun.Name] = true
				case *ast.SelectorExpr:
					if base, isIdent := fun.X.(*ast.Ident); isIdent &&
						recvVar != "" && base.Name == recvVar {
						entry.calls[scrubKey(recvType, fun.Sel.Name)] = true
					}
				}
				return true
			})
			entry.shadowed = shadowed
			graph[scrubKey(recvType, fn.Name.Name)] = entry
		}
	}
	// A named statement counts for whoever names it, wherever it lives —
	// unless that function declared the name itself, in which case it is not
	// the package's value being named.
	for _, entry := range graph {
		for name := range entry.calls {
			entry.statements = append(entry.statements, held[name]...)
		}
		for name := range entry.reads {
			if entry.shadowed[name] {
				if len(held[name]) > 0 {
					entry.hidden = append(entry.hidden, held[name]...)
				}
				continue
			}
			entry.statements = append(entry.statements, held[name]...)
		}
	}
	return graph
}

// parsePackageFiles reads a package's product files once.
func parsePackageFiles(t *testing.T, dir string) []*ast.File {
	t.Helper()
	sources, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("listing %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parsing %s: %v", path, parseErr)
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		t.Fatalf("no product source found under %s, so this census covered nothing", dir)
	}
	return files
}

// packageLevelStatements are the SQL statements each package-level `var`/`const`
// holds, keyed by the name that holds them.
func packageLevelStatements(files []*ast.File) map[string][]string {
	held := map[string][]string{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, isGen := decl.(*ast.GenDecl)
			if !isGen || (gen.Tok != token.VAR && gen.Tok != token.CONST) {
				continue
			}
			for _, spec := range gen.Specs {
				value, isValue := spec.(*ast.ValueSpec)
				if !isValue {
					continue
				}
				// A spec whose names and values do not align one-to-one binds no
				// statement this can read — `var a, b = twoResults()` keeps one
				// expression for two names. Skipped whole rather than by index,
				// so the second name is not silently dropped while the first is
				// read against a value that is not its own.
				if len(value.Names) != len(value.Values) {
					continue
				}
				for i, name := range value.Names {
					// Every string LITERAL in the value, not ONLY the folded
					// whole. These statements are assembled — a raw string plus
					// a helper's output — so folding them returns nothing, and a
					// reader that gave up on the fold gave up on the statement.
					//
					// Where a fold DOES read whole, it is recorded beside the
					// parts rather than instead of them: `UPDATE x SET a = $1 ` +
					// `WHERE id = $2` carries neither the SET nor the WHERE on
					// its own, so a census matching a statement shape sees
					// nothing in either half. Both are kept because adding a
					// reading can only widen what a consumer sees, and it is
					// under-recognition that passes silently.
					//
					// So what a name holds is a SET OF READINGS of one value,
					// overlapping on purpose, and not a list of the statements
					// that value contains. A consumer that took the first entry
					// as "the statement" would be reading whichever reading this
					// walk happened to record first.
					ast.Inspect(value.Values[i], func(node ast.Node) bool {
						if binary, isBinary := node.(*ast.BinaryExpr); isBinary && binary.Op == token.ADD {
							if folded, readable := gatekit.ConcatenatedString(binary); readable {
								held[name.Name] = append(held[name.Name], folded)
							}
							return true
						}
						literal, isLiteral := node.(*ast.BasicLit)
						if !isLiteral || literal.Kind != token.STRING {
							return true
						}
						statement, readable := strconv.Unquote(literal.Value)
						if readable != nil {
							return true
						}
						held[name.Name] = append(held[name.Name], statement)
						return true
					})
				}
			}
		}
	}
	return held
}

// scrubKey keys a method by its receiver type and a plain function by itself.
func scrubKey(receiver, name string) string {
	if receiver == "" {
		return name
	}
	return receiver + "." + name
}

// receiverVarName is what a method calls its own receiver, so a call on it can
// be told from a call on anything else. This package's receiverTypeName already
// gives the type; only the variable was missing.
func receiverVarName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 || len(fn.Recv.List[0].Names) == 0 {
		return ""
	}
	return fn.Recv.List[0].Names[0].Name
}

// guardedBy reports whether EVERY route to this function passes through one
// that calls target directly.
//
// The rule is UNIVERSAL, and existential reachability is the trap it avoids:
// "some ancestor of this writer can also reach target" is true of nearly every
// function in a package that calls target from several places, so a gate built
// that way passes with target deleted from every real call site.
//
// A function is guarded when it calls target
// itself, or when it has callers and all of them are guarded. A function with
// NO callers is an entry point: if it has not called target by then, nothing
// will, and the write escapes.
//
// TWO HOLES, both real and neither reported by this function:
//
//   - A cycle is treated as guarded while it is on the stack, which is the only
//     terminating answer. A writer that reaches itself and is guarded nowhere
//     else therefore passes — a retry loop is the shape that does it.
//   - "Calls target" is per FUNCTION, not per path. A caller whose two arms are
//     mutually exclusive — re-check on one, the write on the other — ratifies
//     the write, and an INCOMING edge this graph cannot follow (a call through
//     a stored func value) bypasses every guard it did verify.
//
// Neither is fixable at this grain; both are named here rather than in a pull
// request, because this is where the next reader is.
func guardedBy(graph map[string]*graphFunc, name, target string) bool {
	callers := map[string][]string{}
	for caller, entry := range graph {
		for called := range entry.calls {
			callers[called] = append(callers[called], caller)
		}
	}
	verdict := map[string]bool{}
	onStack := map[string]bool{}
	var guarded func(string) bool
	guarded = func(fn string) bool {
		if known, seen := verdict[fn]; seen {
			return known
		}
		if onStack[fn] {
			return true
		}
		entry, known := graph[fn]
		if !known {
			return false
		}
		if entry.calls[target] {
			verdict[fn] = true
			return true
		}
		up := callers[fn]
		if len(up) == 0 {
			verdict[fn] = false
			return false
		}
		onStack[fn] = true
		all := true
		for _, caller := range up {
			if !guarded(caller) {
				all = false
				break
			}
		}
		delete(onStack, fn)
		verdict[fn] = all
		return all
	}
	return guarded(name)
}
