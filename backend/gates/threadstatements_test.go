// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/token"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

type threadSQLValue struct {
	text       string
	unresolved bool
}
type threadSQLPackage struct {
	bindings       map[string][]ast.Expr
	functions      map[string]*ast.FuncDecl
	imports        map[string]string
	directory      string
	transports     map[string]int
	structs        map[string]*ast.StructType
	resolvePackage func(string) threadSQLPackage
	schema         *emSchema
	parameterMemo  map[string]threadSQLSources
}

func (p threadSQLPackage) bindingsBefore(fn *ast.FuncDecl, before token.Pos) map[string][]ast.Expr {
	return p.bindingsWith(fn, before, nil)
}

func (p threadSQLPackage) bindingsWith(fn *ast.FuncDecl, before token.Pos, parameters map[string][]ast.Expr) map[string][]ast.Expr {
	out := map[string][]ast.Expr{threadSQLBefore: {&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(int(before))}}}
	for name, values := range p.bindings {
		out[name] = values
	}
	for name, values := range parameters {
		out[name] = values
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if n == nil || n.Pos() >= before {
			return false
		}
		if closure, ok := n.(*ast.FuncLit); ok && (before < closure.Pos() || before > closure.End()) {
			return false
		}
		switch bind := n.(type) {
		case *ast.AssignStmt:
			for i, left := range bind.Lhs {
				name := threadSQLReference(left)
				if name == "" {
					continue
				}
				if i >= len(bind.Rhs) {
					out[name] = []ast.Expr{ast.NewIdent("$threadUnknownTupleResult")}
					continue
				}
				right := bind.Rhs[i]
				if bind.Tok == token.ADD_ASSIGN {
					var appended []ast.Expr
					for _, previous := range out[name] {
						appended = append(appended, &ast.BinaryExpr{X: previous, Op: token.ADD, Y: right})
					}
					if threadConditionalBinding(fn, bind.Pos()) {
						appended = append(appended, out[name]...)
					}
					out[name] = appended
				} else if threadConditionalBinding(fn, bind.Pos()) {
					out[name] = append(out[name], right)
				} else {
					out[name] = []ast.Expr{right}
				}
			}
		case *ast.CallExpr:
			p.recordBuilderWrite(out, bind, fn)
		case *ast.RangeStmt:
			if name, ok := bind.Value.(*ast.Ident); ok && before >= bind.Body.Pos() && before <= bind.Body.End() {
				if elements := threadRangeElements(bind.X, out, map[string]bool{}); len(elements) > 0 {
					out[name.Name] = elements
				}
			}
		case *ast.ValueSpec:
			for i, name := range bind.Names {
				if i < len(bind.Values) {
					out[name.Name] = []ast.Expr{bind.Values[i]}
				}
			}
		}
		return true
	})
	return out
}

func (p threadSQLPackage) values(expr ast.Expr, bindings map[string][]ast.Expr, reached map[string]bool, context *ast.FuncDecl, depth int) []threadSQLValue {
	if depth > threadSQLMaxDepth {
		return []threadSQLValue{{text: gatekit.ComputedFragment, unresolved: true}}
	}
	switch node := expr.(type) {
	case *ast.BasicLit:
		if text, ok := gatekit.LiteralText(node); ok {
			if strings.EqualFold(text, "TRUE") || strings.EqualFold(text, "FALSE") {
				text = threadBooleanAtom
			}
			return []threadSQLValue{{text: text}}
		}
	case *ast.ParenExpr:
		return p.values(node.X, bindings, reached, context, depth+1)
	case *ast.BinaryExpr:
		if node.Op == token.ADD {
			return joinThreadSQL(p.values(node.X, bindings, reached, context, depth+1), p.values(node.Y, bindings, reached, context, depth+1))
		}
	case *ast.Ident:
		key := threadBindingIdentity(context, node.Name)
		if reached[key] {
			break
		}
		next := cloneThreadSQLReach(reached, key)
		var values []threadSQLValue
		for _, value := range bindings[node.Name] {
			values = append(values, p.values(value, bindings, next, context, depth+1)...)
		}
		if len(values) > 0 {
			return values
		}
	case *ast.IndexExpr:
		return p.indexValues(node, bindings, reached, context, depth)
	case *ast.SelectorExpr:
		return p.fieldValues(node, bindings, reached, context, depth)
	case *ast.CallExpr:
		name := ""
		switch fun := node.Fun.(type) {
		case *ast.Ident:
			name = fun.Name
		case *ast.SelectorExpr:
			name = fun.Sel.Name
		}
		if values, ok := p.identifierValues(node, bindings, reached, context, depth); ok {
			return values
		}
		if values, ok := p.closureValues(node, bindings, reached, context, depth); ok {
			return values
		}
		if name == "String" {
			if values, ok := p.builderValues(node, bindings, reached, context, depth); ok {
				return values
			}
		}
		if threadSQLClauseBuilder(name) && p.canonicalClauseCall(node, name) {
			return p.clauseValues(name, node, bindings, reached, context, depth)
		}
		if name == "Sprintf" || name == "SQLf" {
			return p.formatValues(node, bindings, reached, context, depth)
		}
		if name == "string" && len(node.Args) == 1 {
			return p.values(node.Args[0], bindings, reached, context, depth+1)
		}

		if target, ok := p.calledFunction(node, context); ok && threadSQLStringResult(target) && !reached[scrubKey(receiverTypeName(target), target.Name.Name)] {
			if threadTransparentFormatter(target) {
				return p.formatValues(node, bindings, reached, context, depth)
			}
			var values []threadSQLValue
			ast.Inspect(target.Body, func(n ast.Node) bool {
				if _, closure := n.(*ast.FuncLit); closure {
					return false
				}
				result, ok := n.(*ast.ReturnStmt)
				if !ok {
					return true
				}
				if len(result.Results) > 0 {
					if threadFailureReturn(target, result) && threadCallerChecksError(node, context, bindings) {
						return false
					}
					values = append(values, p.values(result.Results[0], p.callBindings(node, bindings, reached, context, target, result.Pos(), depth), cloneThreadSQLReach(reached, scrubKey(receiverTypeName(target), target.Name.Name)), target, depth+1)...)
				}
				return false
			})
			if len(values) > 0 {
				return values
			}
		}
	}
	return []threadSQLValue{{text: gatekit.ComputedFragment}}
}

func cloneThreadSQLReach(reached map[string]bool, name string) map[string]bool {
	next := make(map[string]bool, len(reached)+1)
	for key, value := range reached {
		next[key] = value
	}
	next[name] = true
	return next
}

const threadSQLMaxAlternatives = 16384
const threadSQLMaxDepth = 100

func joinThreadSQL(left, right []threadSQLValue) []threadSQLValue {
	unique := map[threadSQLValue]bool{}
	out := make([]threadSQLValue, 0)
	for _, a := range left {
		for _, b := range right {
			value := threadSQLValue{text: a.text + b.text, unresolved: a.unresolved || b.unresolved}
			if unique[value] {
				continue
			}
			if len(out) == threadSQLMaxAlternatives {
				return []threadSQLValue{{text: gatekit.ComputedFragment, unresolved: true}}
			}
			unique[value] = true
			out = append(out, value)
		}
	}
	return out
}

func threadSQLClauseBuilder(name string) bool {
	for _, builder := range []string{"ActivityDiscoverClause", "ActivityContentClause", "ActivityAvailableClause", "ActivityRepairPendingClause", "StoredOriginalSQL", "ConnectorSeatSQL", "HeldThroughRecordLinks", "ThreadSQL", "ConversationSQL", "SubjectMailSQL", "AudienceWorkspaceOnly", "CurrentCommitmentClause", "CurrentTaskClause", "CurrentSourceMailClause", "ClassifyBacklogPredicate", "SignatureSourceEligible", "CitedActivityTime", "CitedActivityStamp", "CitedActivityPrecision", "signatureSourceSQL", "signatureMailSQL"} {
		if name == builder {
			return true
		}
	}
	return false
}

func (p threadSQLPackage) unguardedQueries(fn *ast.FuncDecl) []string {
	var offenders []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sqlIndex, ok := p.queryArgument(call, fn)
		if !ok || len(call.Args) <= sqlIndex {
			return true
		}
		if p.forwardedSQLParameter(fn, call.Args[sqlIndex]) {
			return true
		}

		for _, value := range p.values(call.Args[sqlIndex], p.bindingsBefore(fn, call.Pos()), map[string]bool{}, fn, 0) {
			if value.unresolved {
				offenders = append(offenders, "SQL expansion exceeds the census bound; simplify the builder so its boundary can be checked")
				continue
			}
			if strings.TrimSpace(value.text) == gatekit.ComputedFragment {
				if p.nonActivitySQL(call.Args[sqlIndex], p.bindingsBefore(fn, call.Pos()), fn) {
					continue
				}
				offenders = append(offenders, "unresolved SQL expression; expose its statement to the census")
				continue
			}
			if threadGuardColumnMismatch(value.text, p.schema) != "" {
				offenders = append(offenders, value.text)
				continue
			}
			if len(threadActivityPositions(threadStatementTokens(value.text))) == 0 {
				sources := p.sqlSources(call.Args[sqlIndex], p.bindingsBefore(fn, call.Pos()), fn, map[string]bool{}, 0)
				if !threadStructuralUnknown(value.text) || !sources.activity {
					continue
				}
				offenders = append(offenders, value.text)
				continue
			}
			if threadStatementGuarded(value.text) || (threadActivityMutation.MatchString(value.text) && p.writeBoundary(fn, map[string]bool{})) {
				continue
			}
			offenders = append(offenders, value.text)
		}
		return true
	})
	return offenders
}

func TestEveryActivityQueryKeepsItsOwnThreadBarrier(t *testing.T) {
	t.Parallel()
	defer threadStatementProbes.AssertAllMatched(t)
	defer threadStatementExceptions.AssertAllMatched(t)
	var findings []threadStatementFinding
	defer func() { threadWriteFindings(t, findings) }()
	producerIndex := threadProducerIndex{t: t, packages: map[string]map[string]threadProducerDeclaration{}}
	defer producerIndex.writeEvidence(t)
	defer producerIndex.assertRootsConsumed(t)
	packages := map[string]threadSQLPackage{}
	sources := threadStatementSources(t)
	schema := parseHeadCatalog(t)
	transports := map[string]int{}
	for _, src := range sources {
		dir := path.Dir(src.Path)
		if _, done := packages[dir]; !done {
			pkg := threadSQLPackageOf(parsePackageFiles(t, dir))
			pkg.directory = dir
			pkg.schema = schema
			packages[dir] = pkg
			for name, fn := range pkg.functions {
				if slot, ok := threadWrapperParameter(fn); ok {
					transports[dir+":"+name] = slot
				}
			}
		}
	}
	var resolvePackage func(string) threadSQLPackage
	resolvePackage = func(directory string) threadSQLPackage {
		if pkg, ok := packages[directory]; ok {
			pkg.resolvePackage = resolvePackage
			return pkg
		}
		pkg := threadSQLPackageOf(parsePackageFiles(t, directory))
		pkg.directory = directory
		pkg.schema = schema
		pkg.resolvePackage = resolvePackage
		packages[directory] = pkg
		return pkg
	}
	for dir, pkg := range packages {
		pkg.resolvePackage = resolvePackage
		pkg.transports = transports
		packages[dir] = pkg
	}
	seen := map[string]bool{}
	for _, src := range sources {
		dir := path.Dir(src.Path)
		if _, done := packages[dir]; !done {
			packages[dir] = threadSQLPackageOf(parsePackageFiles(t, dir))
		}
		for _, decl := range src.File.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			name := scrubKey(receiverTypeName(fn), fn.Name.Name)
			queries := packages[dir].unguardedQueries(fn)
			if len(queries) == 0 {
				continue
			}
			producerFingerprint, dependencies := producerIndex.fingerprint(dir, name)
			producerKey := "producer:" + src.Path + ":" + name + ":" + producerFingerprint
			variantFingerprint, resolved := threadStatementSetFingerprint(queries)
			for _, query := range queries {
				key := src.Path + ":" + name + ":" + gatekit.FirstLineOf(query)
				identity := src.Path + ":" + name + ":" + threadStatementFingerprint(query)
				if seen[identity] {
					continue
				}
				seen[identity] = true
				alias := threadStatementUnguardedAlias(query)
				group := src.Path + ":" + name + ":" + alias
				finding := threadStatementFinding{ProducerKey: producerKey, Dependencies: len(dependencies), Variants: len(queries), Source: src.Path, Function: name, SQL: query, Fingerprint: threadStatementFingerprint(query), Alias: threadStatementUnguardedAlias(query), VariantFingerprint: variantFingerprint, Resolved: resolved}
				if threadStatementProbes.Waived(t, key) {
					continue
				}
				if looksLikeSQL(stripNoise(query)) && !threadStructuralUnknown(query) && threadStatementExceptions.Waived(t, identity) {
					continue
				}
				if threadStatementExceptions.Waived(t, producerKey) {
					continue
				}
				if resolved && threadStatementExceptions.Waived(t, src.Path+":"+name+":"+variantFingerprint) {
					continue
				}
				if seen[group] {
					continue
				}
				seen[group] = true
				findings = append(findings, finding)
				t.Errorf("%s has an activity query without its own availability boundary", key)
			}
		}
	}
}

var threadStatementProbes = gatekit.Waive(map[string]string{})

func threadConditionalBinding(fn *ast.FuncDecl, pos token.Pos) bool {
	conditional := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if node == nil || conditional {
			return false
		}
		var body ast.Node
		switch branch := node.(type) {
		case *ast.IfStmt:
			if pos >= branch.Body.Pos() && pos <= branch.Body.End() {
				conditional = true
			}
			if branch.Else != nil && pos >= branch.Else.Pos() && pos <= branch.Else.End() {
				conditional = true
			}
		case *ast.ForStmt:
			body = branch.Body
		case *ast.RangeStmt:
			body = branch.Body
		case *ast.CaseClause:
			body = branch
		}
		if body != nil && pos >= body.Pos() && pos <= body.End() {
			conditional = true
		}
		return !conditional
	})
	return conditional
}
