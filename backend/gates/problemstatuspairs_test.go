// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

import (
	"go/ast"
	"go/token"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"testing"
)

type problemStatusPair struct {
	status int
	code   string
}

type pairCensus struct {
	codes  *emittedCensus
	known  map[string]string
	pairs  map[problemStatusPair]bool
	faults []string
}

func emittedProblemStatusPairs(t *testing.T) map[problemStatusPair]bool {
	t.Helper()
	units := readGoUnits(t, "internal")
	body := newEmittedCensus(t, units)
	body.readSources()
	p := pairCensus{codes: newEmittedCensus(t, units), known: body.codes, pairs: map[problemStatusPair]bool{}}
	p.codes.fieldCodes = true
	for name := range bodyCodeCarriers {
		shape := p.codes.structs[problemWriterDir][name]
		for _, set := range p.codes.carrierLiterals(name) {
			status, hasStatus := fieldValue(set.lit, shape, "Status")
			code, hasCode := fieldValue(set.lit, shape, "Code")
			if hasStatus && hasCode {
				p.read(status, code, set.unit, set.in, 0)
			}
		}
	}
	for _, row := range p.codes.tableRows() {
		p.read(row.status, row.code, row.unit, nil, 0)
	}
	p.readCodeAssignments()
	p.readMessageMethods()
	for _, fault := range p.faults {
		t.Error(fault)
	}
	return p.pairs
}

// Bind both arguments together when a writer receives status and code.
func (p *pairCensus) read(status, code ast.Expr, unit *goUnit, in *ast.FuncDecl, depth int) {
	if depth > 12 {
		p.faults = append(p.faults, p.codes.at(unit, status)+": status helper recursion is not bounded")
		return
	}
	if (forwardingPair(status, code) && p.codes.relaysACode(code, unit)) || p.guardedStatusRelay(status, code, unit, in) {
		return
	}
	if ident, ok := status.(*ast.Ident); ok {
		_, bound := p.codes.fieldBindings[ident.Obj]
		if _, parameter := codeParamIndex(in, ident); parameter && !bound {
			key := codeFuncKey{dir: unit.dir, name: in.Name.Name, method: in.Recv != nil}
			for _, site := range p.codes.calls[key] {
				restore := p.bind(in, site.call, site.unit, site.in)
				p.read(status, code, unit, in, depth+1)
				restore()
			}
			return
		}
	}
	if p.readTablePair(status, code, unit, in, depth) {
		return
	}
	statuses := p.statuses(status, unit, in, 0)
	if len(statuses) == 0 {
		p.faults = append(p.faults, p.codes.at(unit, status)+": cannot follow the emitted Problem's HTTP status")
		return
	}
	p.codes.codes = map[string]string{}
	p.codes.followed = map[paramRef]bool{}
	p.codes.fieldsRead = map[string]bool{}
	p.codes.read(code, unit, in)
	for value := range p.codes.codes {
		if _, known := p.known[value]; !known {
			continue
		}
		for _, status := range statuses {
			p.pairs[problemStatusPair{status, value}] = true
		}
	}
}

func forwardingPair(status, code ast.Expr) bool {
	s, sok := status.(*ast.SelectorExpr)
	c, cok := code.(*ast.SelectorExpr)
	if !sok || !cok || s.Sel.Name != "Status" || c.Sel.Name != "Code" {
		return false
	}
	return exprString(s.X) == exprString(c.X)
}

func (p *pairCensus) bind(fn *ast.FuncDecl, call *ast.CallExpr, unit *goUnit, in *ast.FuncDecl) func() {
	old := p.codes.fieldBindings
	p.codes.fieldBindings = maps.Clone(old)
	if p.codes.fieldBindings == nil {
		p.codes.fieldBindings = map[*ast.Object]fieldCodeExpr{} //nolint:staticcheck // Parser objects bind local assignments in this syntax-only census.
	}
	index := 0
	for _, parameter := range fn.Type.Params.List {
		for _, name := range parameter.Names {
			if index < len(call.Args) {
				p.codes.fieldBindings[name.Obj] = fieldCodeExpr{call.Args[index], unit, in}
			}
			index++
		}
	}
	return func() { p.codes.fieldBindings = old }
}

func (p *pairCensus) statuses(expr ast.Expr, unit *goUnit, in *ast.FuncDecl, depth int) []int {
	if depth > 12 {
		return nil
	}
	if number, err := strconv.Atoi(exprString(expr)); err == nil {
		return []int{number}
	}
	if number, known := httpStatusNames()[selectedName(expr)]; known {
		return []int{number}
	}
	switch value := expr.(type) {
	case *ast.Ident:
		if bound, ok := p.codes.fieldBindings[value.Obj]; ok {
			return p.statuses(bound.expr, bound.unit, bound.in, depth+1)
		}
		if assigned, known := assignedValue(value); known {
			return p.statuses(assigned, unit, in, depth+1)
		}
		if declaration, known := p.codes.constDecls(unit.dir)[value.Name]; known {
			return p.statuses(declaration.value, declaration.unit, in, depth+1)
		}
	case *ast.ParenExpr:
		return p.statuses(value.X, unit, in, depth+1)
	case *ast.SelectorExpr:
		base, ok := value.X.(*ast.Ident)
		if !ok {
			return nil
		}
		if rows, shape, ranged := rangedTable(base); ranged {
			var out []int
			for _, element := range rows.Elts {
				if row, ok := element.(*ast.CompositeLit); ok {
					if field, found := fieldValue(row, shape, value.Sel.Name); found {
						out = append(out, p.statuses(field, unit, in, depth+1)...)
					}
				}
			}
			return slices.Compact(out)
		}
		if typ, owner, typed := p.codes.typeOf(base, unit, 0); typed {
			if shape, declaredIn, known := p.codes.structOf(typ, owner); known {
				var out []int
				for name, declared := range p.codes.structs[declaredIn.dir] {
					if declared == shape {
						for _, set := range p.codes.composite[declaredIn.dir+"."+name] {
							if field, found := fieldValue(set.lit, shape, value.Sel.Name); found {
								out = append(out, p.statuses(field, set.unit, set.in, depth+1)...)
							}
						}
					}
				}
				return slices.Compact(out)
			}
		}
	case *ast.CallExpr:
		key, known := p.codes.callee(unit, value.Fun)
		if !known {
			return nil
		}
		fn := p.codes.funcs[key]
		if fn == nil || fn.Body == nil {
			return nil
		}
		restore := p.bind(fn, value, unit, in)
		defer restore()
		var out []int
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			if returned, ok := node.(*ast.ReturnStmt); ok && len(returned.Results) > 0 {
				out = append(out, p.statuses(returned.Results[0], p.codes.owner[fn], fn, depth+1)...)
			}
			return true
		})
		return slices.Compact(out)
	}
	return nil
}

// MessageFault codes enter the shared writer, whose status policy owns them.
func (p *pairCensus) readMessageMethods() {
	var statusOwner *goUnit
	var statusFn *ast.FuncDecl
	for _, unit := range p.codes.units {
		if unit.dir == problemWriterDir {
			for _, decl := range unit.file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "messageFaultStatus" {
					statusOwner, statusFn = unit, fn
				}
			}
		}
	}
	if statusFn == nil {
		p.faults = append(p.faults, "messageFaultStatus is missing; the method-code status policy was not read")
		return
	}
	var statuses []int
	ast.Inspect(statusFn.Body, func(node ast.Node) bool {
		if returned, ok := node.(*ast.ReturnStmt); ok && len(returned.Results) > 0 {
			statuses = append(statuses, p.statuses(returned.Results[0], statusOwner, statusFn, 0)...)
		}
		return true
	})
	for _, unit := range p.codes.units {
		for _, decl := range unit.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "MessageFault" || fn.Body == nil {
				continue
			}
			p.codes.codes = map[string]string{}
			p.codes.followed = map[paramRef]bool{}
			p.codes.fieldsRead = map[string]bool{}
			p.codes.readFaultReturns(unit, fn)
			methodStatuses := p.messageMethodStatuses(fn, unit, statuses)
			for code := range p.codes.codes {
				for _, status := range methodStatuses {
					p.pairs[problemStatusPair{status, code}] = true
				}
			}
		}
	}
}

// TestEveryProblemMockUsesAStatusItsCodeIsSentWith checks the emitted Problem contract.
func TestEveryProblemMockUsesAStatusItsCodeIsSentWith(t *testing.T) {
	t.Parallel()
	pairs := emittedProblemStatusPairs(t)
	if len(pairs) < 150 {
		t.Fatalf("read only %d status/code pairs; the census lost its writers", len(pairs))
	}
	mocks, scans := frontendProblemMocks(t)
	client, faults := clientMadeSites(scans)
	for _, fault := range faults {
		t.Error(fault)
	}
	judged := 0
	for _, mock := range mocks {
		if mock.field || mock.computed || client[mock] {
			continue
		}
		if mock.statusUnknown {
			continue
		}
		if mock.status == 0 {
			continue
		}
		judged++
		if !pairs[problemStatusPair{mock.status, mock.code}] {
			var sent []int
			for pair := range pairs {
				if pair.code == mock.code {
					sent = append(sent, pair.status)
				}
			}
			slices.Sort(sent)
			t.Errorf("%s:%d: no server path sends HTTP %d with Problem code %q (server statuses: %v)", mock.path, mock.line, mock.status, mock.code, sent)
		}
	}
	if judged < 300 {
		t.Fatalf("judged only %d concrete status/code mocks; the scan lost HTTP statuses", judged)
	}
	t.Logf("judged %d concrete HTTP status/code pairs", judged)
}

// Assigning a code after constructing a fault keeps its declared status.
func (p *pairCensus) readCodeAssignments() {
	for _, assignment := range p.codes.assigns {
		for index, lhs := range assignment.stmt.Lhs {
			selected, ok := lhs.(*ast.SelectorExpr)
			if !ok || selected.Sel.Name != "Code" || index >= len(assignment.stmt.Rhs) {
				continue
			}
			base, ok := selected.X.(*ast.Ident)
			if !ok {
				continue
			}
			value, known := assignedValue(base)
			if !known {
				continue
			}
			if reference, ok := value.(*ast.UnaryExpr); ok {
				value = reference.X
			}
			literal, ok := value.(*ast.CompositeLit)
			if !ok || !p.codes.isCarrier(literal.Type, assignment.unit) {
				continue
			}
			shape, _, typed := p.codes.structOf(literal.Type, assignment.unit)
			if !typed {
				continue
			}
			if status, found := fieldValue(literal, shape, "Status"); found {
				p.read(status, assignment.stmt.Rhs[index], assignment.unit, assignment.in, 0)
			}
		}
	}
}

// The route's status is preserved only under a guard matching its decoded code.
func (p *pairCensus) guardedStatusRelay(status, code ast.Expr, unit *goUnit, in *ast.FuncDecl) bool {
	selected, ok := status.(*ast.SelectorExpr)
	if !ok || selected.Sel.Name != "StatusCode" || in == nil {
		return false
	}
	value, known := p.codes.constant(code, unit, 0)
	if !known {
		return false
	}
	guarded := false
	ast.Inspect(in.Body, func(node ast.Node) bool {
		branch, ok := node.(*ast.IfStmt)
		if !ok || status.Pos() < branch.Body.Pos() || status.Pos() > branch.Body.End() {
			return true
		}
		ast.Inspect(branch.Cond, func(condition ast.Node) bool {
			comparison, ok := condition.(*ast.BinaryExpr)
			if !ok || comparison.Op != token.EQL {
				return true
			}
			for _, pair := range [][2]ast.Expr{{comparison.X, comparison.Y}, {comparison.Y, comparison.X}} {
				key, ok := pair[0].(*ast.SelectorExpr)
				if !ok || key.Sel.Name != "Code" {
					continue
				}
				if tested, known := p.codes.constant(pair[1], unit, 0); known && tested == value {
					guarded = true
				}
			}
			return true
		})
		return true
	})
	return guarded
}

func (p *pairCensus) messageMethodStatuses(message *ast.FuncDecl, unit *goUnit, fallback []int) []int {
	receiver, known := methodReceiverType(message)
	if !known {
		return fallback
	}
	var unwrap *ast.FuncDecl
	var unwrapOwner *goUnit
	for _, owner := range p.codes.units {
		if owner.dir == unit.dir {
			for _, decl := range owner.file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Name.Name != "Unwrap" || fn.Body == nil {
					continue
				}
				if name, method := methodReceiverType(fn); method && name == receiver {
					unwrap, unwrapOwner = fn, owner
				}
			}
		}
	}
	if unwrap == nil {
		if statuses := p.isMethodStatuses(receiver, unit); len(statuses) > 0 {
			return statuses
		}
		return []int{http.StatusUnprocessableEntity}
	}
	var out []int
	ast.Inspect(unwrap.Body, func(node ast.Node) bool {
		returned, ok := node.(*ast.ReturnStmt)
		if !ok || len(returned.Results) != 1 {
			return true
		}
		wanted, known := p.codes.literalType(unwrapOwner, returned.Results[0])
		if !known {
			return true
		}
		for _, row := range p.codes.tableRows() {
			named, known := p.codes.literalType(row.unit, row.sentinel)
			if known && named == wanted {
				out = append(out, p.statuses(row.status, row.unit, nil, 0)...)
			}
		}
		return true
	})
	if len(out) == 0 {
		return fallback
	}
	return slices.Compact(out)
}

func methodReceiverType(fn *ast.FuncDecl) (string, bool) {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return "", false
	}
	typ := fn.Recv.List[0].Type
	if pointer, ok := typ.(*ast.StarExpr); ok {
		typ = pointer.X
	}
	named, ok := typ.(*ast.Ident)
	if !ok {
		return "", false
	}
	return named.Name, true
}

func (p *pairCensus) readTablePair(status, code ast.Expr, unit *goUnit, in *ast.FuncDecl, depth int) bool {
	s, sok := status.(*ast.SelectorExpr)
	c, cok := code.(*ast.SelectorExpr)
	if !sok || !cok || exprString(s.X) != exprString(c.X) {
		return false
	}
	base, ok := s.X.(*ast.Ident)
	if !ok {
		return false
	}
	rows, shape, ranged := rangedTable(base)
	if !ranged {
		return false
	}
	for _, element := range rows.Elts {
		row, ok := element.(*ast.CompositeLit)
		if !ok {
			continue
		}
		rowStatus, hasStatus := fieldValue(row, shape, s.Sel.Name)
		rowCode, hasCode := fieldValue(row, shape, c.Sel.Name)
		if hasStatus && hasCode {
			p.read(rowStatus, rowCode, unit, in, depth+1)
		}
	}
	return true
}

func (p *pairCensus) isMethodStatuses(receiver string, unit *goUnit) []int {
	var out []int
	for _, owner := range p.codes.units {
		if owner.dir != unit.dir {
			continue
		}
		for _, decl := range owner.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "Is" || fn.Body == nil {
				continue
			}
			name, method := methodReceiverType(fn)
			if !method || name != receiver {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				binary, ok := node.(*ast.BinaryExpr)
				if !ok || binary.Op != token.EQL {
					return true
				}
				for _, expr := range []ast.Expr{binary.X, binary.Y} {
					wanted, known := p.codes.literalType(owner, expr)
					if !known {
						continue
					}
					for _, row := range p.codes.tableRows() {
						named, known := p.codes.literalType(row.unit, row.sentinel)
						if known && named == wanted {
							out = append(out, p.statuses(row.status, row.unit, nil, 0)...)
						}
					}
				}
				return true
			})
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
