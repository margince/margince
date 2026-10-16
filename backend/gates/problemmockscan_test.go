// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// Reading the frontend for Problem mocks. A mock is an object literal with a
// string `code` that is a Problem by its shape or by where it is sent.

import (
	"path"
	"slices"
	"strings"
)

// problemMock is one code a frontend file answers as a Problem's. own marks a
// literal in an object this file builds, which is all a marker can vouch for.
type problemMock struct {
	path     string
	line     int
	code     string
	computed bool
	status   int
	own      bool
}

// tsFunc is a function with named parameters, and the tokens of its body.
// paren is its parameter list's opening token, or its one parameter's name.
type tsFunc struct {
	name     string
	params   []tsSpan
	paren    int
	body     tsSpan
	exported bool
}

// codeSource is how a helper's parameter reaches a Problem's code: by its
// position, then the member path below it. status is the parameter that
// carries the Problem's status, when one does.
type codeSource struct {
	index  int
	path   []string
	status *codeSource
}

// tsImport is one named import: the file it resolves to and its exported name.
type tsImport struct{ from, name string }

// tsBinding is a name a for-of loop or a table test's callback binds to rows
// of an array literal. It names a tuple position, a key, or the whole row.
type tsBinding struct {
	name  string
	scope tsSpan
	table int
	index int
	key   string
}

// problemFileScan is what one file holds: its mocks, its helpers that make a
// parameter a Problem's code, and its calls of imported names.
type problemFileScan struct {
	mocks   []problemMock
	helpers map[string]codeSource
	funcs   []tsFunc
	imports map[string]tsImport
	calls   []int
	markers []tsNote
	reader  *problemFileReader
}

type problemFileReader struct {
	path     string
	source   string
	tree     *tsTree
	takers   map[string]bool
	consts   map[string]int
	objects  map[string]int
	arrays   map[string]int
	bindings []tsBinding
	patterns map[int]bool
	lines    []int
	// statusParam is the helper parameter the object being judged takes its
	// status from, set while its context is read.
	statusParam *codeRead
	scan        *problemFileScan
}

// scanProblemFile reads one tokenized file. takers are the functions whose
// first argument is a Problem, read from the frontend's Problem module.
func scanProblemFile(filePath, source string, tree *tsTree, takers map[string]bool) *problemFileScan {
	reader := &problemFileReader{
		path: filePath, source: source, tree: tree, takers: takers,
		consts: map[string]int{}, objects: map[string]int{}, arrays: map[string]int{}, patterns: map[int]bool{},
		scan: &problemFileScan{helpers: map[string]codeSource{}, imports: map[string]tsImport{}},
	}
	reader.scan.reader = reader
	for i := range source {
		if source[i] == '\n' {
			reader.lines = append(reader.lines, i)
		}
	}
	for _, comment := range tree.comments {
		if strings.Contains(comment.text, clientCodeMarker) {
			reader.scan.markers = append(reader.scan.markers, comment)
		}
	}
	reader.indexDeclarations()
	for open := range tree.tokens {
		if tree.is(open, "{") {
			reader.readObject(open)
		}
	}
	reader.readHelperCalls()
	return reader.scan
}

func (r *problemFileReader) line(i int) int { return r.lineOfPos(r.tree.tokens[i].pos) }

func (r *problemFileReader) lineOfPos(pos int) int {
	n, _ := slices.BinarySearch(r.lines, pos)
	return n + 1
}

// indexDeclarations records the file's string, object and array constants,
// its functions, its named imports and the names its tables bind.
func (r *problemFileReader) indexDeclarations() {
	tr := r.tree
	var tables []int
	for i := range tr.tokens {
		word := tr.word(i)
		switch {
		case (word == "const" || word == "let") && tr.word(i+1) != "" && tr.is(i+2, "="):
			r.indexConstant(tr.word(i+1), i+3)
		case word == "import" && tr.is(i+1, "{"):
			r.readImport(i + 1)
		case word == "for" && tr.is(i+1, "("):
			r.readForOf(i + 1)
		case word == "each" && tr.is(i-1, ".") && tr.is(i+1, "("):
			tables = append(tables, i+1)
		case (word == "const" || word == "let" || word == "var") && tr.is(i+1, "{"):
			r.patterns[i+1] = true
		}
		if fn, ok := r.functionAt(i); ok {
			r.scan.funcs = append(r.scan.funcs, fn)
			for _, param := range fn.params {
				r.patterns[param.from] = true
			}
		}
	}
	for _, paren := range tables {
		r.readEachTable(paren)
	}
}

func (r *problemFileReader) indexConstant(name string, value int) {
	tr := r.tree
	switch {
	case value >= len(tr.tokens):
	case tr.tokens[value].kind == tkString || tr.tokens[value].kind == tkTemplate:
		r.consts[name] = value
	case tr.tokens[value].kind == tkNumber:
		if _, number := unquotedNumber(tr.tokens[value]); number {
			r.consts[name] = value
		}
	case tr.is(value, "{"):
		r.objects[name] = value
	case tr.is(value, "["):
		r.arrays[name] = value
	}
}

func (r *problemFileReader) readImport(open int) {
	tr := r.tree
	end := tr.match[open]
	if tr.word(end+1) != "from" || end+2 >= len(tr.tokens) || tr.tokens[end+2].kind != tkString {
		return
	}
	spec := tr.tokens[end+2].text
	if !strings.HasPrefix(spec, ".") {
		return
	}
	from := path.Join(path.Dir(r.path), spec)
	for _, part := range tr.split(open) {
		start := part.from
		if tr.word(start) == "type" && part.to > start+1 {
			continue
		}
		name, local := tr.word(start), tr.word(start)
		if tr.word(start+1) == "as" {
			local = tr.word(start + 2)
		}
		if name != "" && local != "" {
			r.scan.imports[local] = tsImport{from: from, name: name}
		}
	}
}

// readForOf binds the names of `for (const pattern of rows)` to the rows of
// an array literal, over the loop's body.
func (r *problemFileReader) readForOf(paren int) {
	tr := r.tree
	pattern := paren + 1
	if word := tr.word(pattern); word == "const" || word == "let" || word == "var" {
		pattern++
	}
	end := pattern + 1
	if tr.match[pattern] > pattern {
		end = tr.match[pattern] + 1
	}
	if tr.word(end) != "of" {
		return
	}
	table, ok := r.arrayAt(tsSpan{end + 1, tr.match[paren]})
	if !ok {
		return
	}
	body := tsSpan{tr.match[paren] + 1, len(tr.tokens)}
	if tr.is(body.from, "{") {
		body.to = tr.match[body.from] + 1
	} else {
		body = r.expressionEnd(body.from)
	}
	r.bind(tsSpan{pattern, end}, -1, body, table)
}

// readEachTable binds a test table's callback parameters, `it.each(rows)(name,
// (a, b) => …)`, to the rows' positions.
func (r *problemFileReader) readEachTable(paren int) {
	tr := r.tree
	table, ok := r.arrayAt(tsSpan{paren + 1, tr.match[paren]})
	call := tr.match[paren] + 1
	if !ok || !tr.is(call, "(") {
		return
	}
	for _, fn := range r.scan.funcs {
		if tr.parent[fn.paren] != call {
			continue
		}
		for index, param := range fn.params {
			if r.tree.is(param.from, "{") {
				index = -1
			}
			r.bind(param, index, fn.body, table)
		}
	}
}

// bind records the names a pattern binds to the rows at position, or to the
// whole row when position is -1.
func (r *problemFileReader) bind(pattern tsSpan, position int, scope tsSpan, table int) {
	tr := r.tree
	switch {
	case pattern.to == pattern.from+1 && tr.word(pattern.from) != "":
		r.bindings = append(r.bindings, tsBinding{name: tr.word(pattern.from), scope: scope, table: table, index: position})
	case tr.is(pattern.from, "[") && position < 0:
		for index, part := range tr.split(pattern.from) {
			if name := tr.word(part.from); name != "" && part.to == part.from+1 {
				r.bindings = append(r.bindings, tsBinding{name: name, scope: scope, table: table, index: index})
			}
		}
	case tr.is(pattern.from, "{") && position < 0:
		for _, entry := range tr.entries(pattern.from) {
			name := tr.word(entry.value.from)
			if entry.value.to == entry.value.from+1 && name != "" {
				r.bindings = append(r.bindings, tsBinding{name: name, scope: scope, table: table, index: -1, key: entry.key})
			}
		}
	}
}

// arrayAt answers the array literal a span is: written there, cast, or a
// constant of this file by name.
func (r *problemFileReader) arrayAt(span tsSpan) (int, bool) {
	tr := r.tree
	if span.to == span.from+1 {
		open, known := r.arrays[tr.word(span.from)]
		return open, known
	}
	if tr.is(span.from, "[") && (tr.match[span.from] == span.to-1 || tr.word(tr.match[span.from]+1) == "as") {
		return span.from, true
	}
	return 0, false
}

// functionAt reads a function that starts at token i: a declaration, a named
// arrow, or an arrow written in place, each with its parameter list.
func (r *problemFileReader) functionAt(i int) (tsFunc, bool) {
	tr := r.tree
	exported := tr.word(i-1) == "export" || tr.word(i-2) == "export" && tr.word(i-1) == "async"
	switch {
	case tr.word(i) == "function" && tr.word(i+1) != "":
		for j := i + 2; j < len(tr.tokens) && !tr.is(j, "{"); j++ {
			if tr.is(j, "(") {
				return r.withBody(tsFunc{name: tr.word(i + 1), exported: exported}, j)
			}
		}
	case (tr.word(i) == "const" || tr.word(i) == "let") && tr.word(i+1) != "" && tr.is(i+2, "="):
		start := i + 3
		if tr.word(start) == "async" {
			start++
		}
		if tr.word(start) != "" && tr.is(start+1, "=>") {
			return tsFunc{
				name: tr.word(i + 1), params: []tsSpan{{start, start + 1}}, paren: start,
				body: r.arrowBody(start + 1), exported: exported,
			}, true
		}
		if tr.is(start, "(") {
			return r.withBody(tsFunc{name: tr.word(i + 1), exported: exported}, start)
		}
	case tr.is(i, "(") && !r.named(i) && tr.is(tr.match[i]+1, "=>"):
		return r.withBody(tsFunc{}, i)
	case tr.word(i) != "" && tr.is(i+1, "=>") && !tr.is(i-1, "=") && !tr.is(i-2, "="):
		return tsFunc{params: []tsSpan{{i, i + 1}}, paren: i, body: r.arrowBody(i + 1)}, true
	}
	return tsFunc{}, false
}

// named reports a parameter list a named declaration already reads.
func (r *problemFileReader) named(paren int) bool {
	tr := r.tree
	before := paren - 1
	if tr.word(before) == "async" {
		before--
	}
	return tr.is(before, "=") && (tr.word(before-2) == "const" || tr.word(before-2) == "let")
}

// withBody completes a function whose parameter list opens at paren: a block
// body, or an arrow's body to the end of its expression.
func (r *problemFileReader) withBody(fn tsFunc, paren int) (tsFunc, bool) {
	tr := r.tree
	fn.paren = paren
	for _, part := range tr.split(paren) {
		if tr.is(part.from, "...") {
			part.from++
		}
		param := tsSpan{part.from, part.from + 1}
		if tr.match[part.from] > part.from {
			param.to = tr.match[part.from] + 1
		}
		fn.params = append(fn.params, param)
	}
	after := tr.match[paren] + 1
	for j := after; j < len(tr.tokens); j++ {
		switch {
		case tr.is(j, "=>"):
			fn.body = r.arrowBody(j)
			return fn, true
		case tr.is(j, "{") && (j == after || tr.is(after, ":")):
			if tr.is(after, ":") && !r.typeEndsAt(after, j) {
				j = tr.match[j]
				continue
			}
			fn.body = tsSpan{j, tr.match[j] + 1}
			return fn, true
		case tr.is(j, ";") || tr.is(j, ","):
			return tsFunc{}, false
		}
		if tr.match[j] > j {
			j = tr.match[j]
		}
	}
	return tsFunc{}, false
}

// typeEndsAt reports whether a return type annotation that starts at colon
// ends before the brace at open, which then opens the body.
func (r *problemFileReader) typeEndsAt(colon, open int) bool {
	previous := r.tree.tokens[open-1]
	return open > colon+1 && (previous.kind == tkIdent || r.tree.is(open-1, ">") || r.tree.is(open-1, "]"))
}

// arrowBody answers the tokens after an arrow: a block, or an expression.
func (r *problemFileReader) arrowBody(arrow int) tsSpan {
	if r.tree.is(arrow+1, "{") {
		return tsSpan{arrow + 1, r.tree.match[arrow+1] + 1}
	}
	return r.expressionEnd(arrow + 1)
}

// expressionEnd answers the expression from token from up to the comma,
// semicolon or closer that ends it.
func (r *problemFileReader) expressionEnd(from int) tsSpan {
	tr := r.tree
	for j := from; j < len(tr.tokens); j++ {
		if tr.match[j] > j {
			j = tr.match[j]
			continue
		}
		if tr.is(j, ",") || tr.is(j, ";") || tr.match[j] >= 0 && tr.match[j] < j {
			return tsSpan{from, j}
		}
	}
	return tsSpan{from, len(tr.tokens)}
}

// readObject judges one brace that holds a `code` entry. A code it cannot
// read is a finding, never a pass.
func (r *problemFileReader) readObject(open int) {
	entry, found := codeEntry(r.tree.entries(open))
	if !found || r.patterns[open] {
		return
	}
	r.statusParam = nil
	status, problem := r.problemContext(open, map[int]bool{})
	if !problem {
		return
	}
	read, ok := r.resolve(entry.value, nil, 0)
	switch {
	case !ok:
		r.scan.mocks = append(r.scan.mocks, problemMock{path: r.path, line: r.line(entry.keyAt), computed: true, status: status})
	case read.helper != "":
		if r.statusParam != nil && r.statusParam.helper == read.helper {
			read.source.status = &r.statusParam.source
		}
		r.scan.helpers[read.helper] = read.source
	default:
		r.addMocks(read.literals, status, entry.keyAt, true)
	}
}

// addMocks records each literal a code resolved to, at its own line when it
// is not the code entry's own value.
func (r *problemFileReader) addMocks(literals []int, status, at int, own bool) {
	for _, literal := range literals {
		token := r.tree.tokens[literal]
		if token.kind == tkNumber {
			continue
		}
		r.scan.mocks = append(r.scan.mocks, problemMock{
			path: r.path, line: r.line(literal), code: token.text, computed: token.computed, status: status,
			own: own && r.line(literal) == r.line(at),
		})
	}
}

func codeEntry(entries []tsEntry) (tsEntry, bool) {
	for _, entry := range entries {
		if entry.key == "code" && !entry.spread {
			return entry, true
		}
	}
	return tsEntry{}, false
}

// codeRead is what a code value resolved to: the literal tokens it can be,
// or the parameter of a named helper it comes from.
type codeRead struct {
	literals []int
	helper   string
	source   codeSource
}

// resolve reads the codes value holds once path is read below it, and reports
// false for a value it cannot read.
func (r *problemFileReader) resolve(value tsSpan, path []string, depth int) (codeRead, bool) {
	tr := r.tree
	if depth > 8 || value.to <= value.from {
		return codeRead{}, false
	}
	if len(path) == 0 {
		if literals, ok := r.tree.literals(value); ok {
			return codeRead{literals: literals}, true
		}
	}
	if word := tr.word(value.from); value.to == value.from+1 && (word == "undefined" || word == "null") {
		return codeRead{}, true
	}
	if tr.is(value.from, "{") && tr.match[value.from] == value.to-1 && len(path) > 0 {
		for _, entry := range tr.entries(value.from) {
			if entry.key == path[0] && !entry.spread {
				return r.resolve(entry.value, path[1:], depth+1)
			}
		}
		return codeRead{}, true
	}
	chain, ok := memberChain(tr, value)
	if !ok {
		return codeRead{}, false
	}
	return r.resolveName(value.from, chain[0], append(chain[1:len(chain):len(chain)], path...), depth)
}

// resolveName reads a name at token at, with path read below it. The
// innermost scope wins: a table row, then a parameter, then a file constant.
func (r *problemFileReader) resolveName(at int, name string, path []string, depth int) (codeRead, bool) {
	tr := r.tree
	if binding, bound := r.bindingFor(at, name); bound {
		var read codeRead
		for _, row := range tr.split(binding.table) {
			value, ok := rowValue(tr, row, binding)
			rowRead, readable := r.resolve(value, path, depth+1)
			if !ok || !readable || rowRead.helper != "" {
				return codeRead{}, false
			}
			read.literals = append(read.literals, rowRead.literals...)
		}
		return read, true
	}
	if fn, index, ok := r.paramOf(at, name); ok {
		if fn.name == "" {
			return codeRead{}, false
		}
		return codeRead{helper: fn.name, source: codeSource{index: index, path: path}}, true
	}
	if literal, known := r.consts[name]; known && len(path) == 0 {
		return codeRead{literals: []int{literal}}, true
	}
	if open, known := r.objects[name]; known && len(path) > 0 {
		return r.resolve(tsSpan{open, tr.match[open] + 1}, path, depth+1)
	}
	return codeRead{}, false
}

// memberChain reads `a.b.c` as its names.
func memberChain(tr *tsTree, value tsSpan) ([]string, bool) {
	var chain []string
	for i := value.from; i < value.to; i += 2 {
		if tr.word(i) == "" || i+1 < value.to && !tr.is(i+1, ".") {
			return nil, false
		}
		chain = append(chain, tr.word(i))
	}
	return chain, len(chain) > 0
}

// rowValue answers the part of a table row a binding names.
func rowValue(tr *tsTree, row tsSpan, binding tsBinding) (tsSpan, bool) {
	switch {
	case binding.index == 0 && !tr.is(row.from, "["):
		return row, true
	case binding.index >= 0:
		parts := tr.split(row.from)
		if !tr.is(row.from, "[") || binding.index >= len(parts) {
			return tsSpan{}, false
		}
		return parts[binding.index], true
	case binding.key != "":
		if !tr.is(row.from, "{") {
			return tsSpan{}, false
		}
		for _, entry := range tr.entries(row.from) {
			if entry.key == binding.key && !entry.spread {
				return entry.value, true
			}
		}
		return tsSpan{}, false
	}
	return row, true
}

// bindingFor answers the innermost table binding of name around token at.
func (r *problemFileReader) bindingFor(at int, name string) (tsBinding, bool) {
	best, found := tsBinding{}, false
	for _, binding := range r.bindings {
		if binding.name == name && at >= binding.scope.from && at < binding.scope.to &&
			(!found || binding.scope.from > best.scope.from) {
			best, found = binding, true
		}
	}
	return best, found
}

// paramOf answers the innermost function whose body holds token at and whose
// parameters name it.
func (r *problemFileReader) paramOf(at int, name string) (tsFunc, int, bool) {
	best, bestIndex, found := tsFunc{}, 0, false
	for _, fn := range r.scan.funcs {
		if at < fn.body.from || at >= fn.body.to {
			continue
		}
		index := slices.IndexFunc(fn.params, func(param tsSpan) bool {
			return param.to == param.from+1 && r.tree.word(param.from) == name
		})
		if index >= 0 && (!found || fn.body.from > best.body.from) {
			best, bestIndex, found = fn, index, true
		}
	}
	return best, bestIndex, found
}

// problemContext reports whether the object at open is a Problem. It holds a
// refusal status, is spread into a Problem, or is a 4xx or 5xx answer's body.
func (r *problemFileReader) problemContext(open int, seen map[int]bool) (int, bool) {
	tr := r.tree
	if seen[open] {
		return 0, false
	}
	seen[open] = true
	if status, refusal := r.ownStatus(open, map[int]bool{}); refusal {
		return status, true
	}
	slot := tsSpan{open, tr.match[open] + 1}
	for p := tr.parent[open]; p >= 0; p = tr.parent[p] {
		switch {
		case tr.is(p, "(") && r.isStringify(p) && r.fills(tsSpan{p + 1, tr.match[p]}, slot):
			slot = tsSpan{p - 3, tr.match[p] + 1}
		case tr.is(p, "(") && problemCallee(tr, p) == "" && r.fills(tsSpan{p + 1, tr.match[p]}, slot):
			slot = tsSpan{p, tr.match[p] + 1}
		case tr.is(p, "("):
			return r.sentAsArgument(p, slot)
		case tr.is(p, "{"):
			return r.sentAsValue(p, slot, seen)
		default:
			return 0, false
		}
	}
	return 0, false
}

// sentAsArgument judges an argument of the call at paren. It is a Problem as
// a Problem taker's first, or as a body beside a 4xx or 5xx status.
func (r *problemFileReader) sentAsArgument(paren int, slot tsSpan) (int, bool) {
	tr := r.tree
	args := tr.split(paren)
	index := slices.IndexFunc(args, func(arg tsSpan) bool { return r.fills(arg, slot) })
	if index < 0 {
		return 0, false
	}
	if index == 0 && r.takers[problemCallee(tr, paren)] {
		return 0, true
	}
	for i, arg := range args {
		if i == index {
			continue
		}
		if tr.is(arg.from, "{") && tr.match[arg.from] == arg.to-1 {
			if code, refusal := r.statusOf(tr.entries(arg.from)); refusal {
				return code, true
			}
			continue
		}
		if code, refusal := r.boundStatus(arg); refusal {
			return code, true
		}
	}
	return 0, false
}

// sentAsValue judges an entry's value in the object at brace. It is a
// Problem spread into one, or as a body beside a 4xx or 5xx status.
func (r *problemFileReader) sentAsValue(brace int, slot tsSpan, seen map[int]bool) (int, bool) {
	tr := r.tree
	for _, entry := range tr.entries(brace) {
		if !r.fills(entry.value, slot) {
			continue
		}
		if entry.spread {
			return r.problemContext(brace, seen)
		}
		if code, refusal := r.statusOf(tr.entries(brace)); refusal {
			return code, true
		}
		return 0, false
	}
	return 0, false
}

func refusalStatus(code int) bool { return code >= 400 && code <= 599 }

// statusOf reads an object's status entry as a refusal status.
func (r *problemFileReader) statusOf(entries []tsEntry) (int, bool) {
	for _, entry := range entries {
		if entry.key == "status" && !entry.spread {
			return r.boundStatus(entry.value)
		}
	}
	return 0, false
}

// boundStatus reads a status expression as a refusal: a 4xx or 5xx literal,
// constant or table row, or a helper parameter its calls decide. A value it
// cannot follow counts when it is named `status`.
func (r *problemFileReader) boundStatus(span tsSpan) (int, bool) {
	tr := r.tree
	read, ok := r.resolve(span, nil, 0)
	switch {
	case ok && read.helper != "":
		r.statusParam = &read
		return 0, true
	case ok && len(read.literals) > 0:
		return refusalOf(tr, read.literals)
	}
	chain, named := memberChain(tr, span)
	return 0, named && chain[len(chain)-1] == "status"
}

// refusalOf answers the first refusal status among number literals.
func refusalOf(tr *tsTree, literals []int) (int, bool) {
	for _, literal := range literals {
		if code, number := unquotedNumber(tr.tokens[literal]); number && refusalStatus(code) {
			return code, true
		}
	}
	return 0, false
}

// ownStatus reads the refusal status an object holds, or one it spreads in
// by name or as a literal.
func (r *problemFileReader) ownStatus(open int, seen map[int]bool) (int, bool) {
	if seen[open] {
		return 0, false
	}
	seen[open] = true
	entries := r.tree.entries(open)
	if status, refusal := r.statusOf(entries); refusal {
		return status, true
	}
	for _, entry := range entries {
		if !entry.spread {
			continue
		}
		for _, spread := range r.objectsFilling(entry.value) {
			if status, refusal := r.ownStatus(spread, seen); refusal {
				return status, true
			}
		}
	}
	return 0, false
}

// objectsFilling answers the object literals a value can be: itself, either
// arm of a conditional, a parenthesised one, or a constant object by name.
func (r *problemFileReader) objectsFilling(value tsSpan) []int {
	tr := r.tree
	if value.to <= value.from {
		return nil
	}
	if value.to == value.from+1 {
		if open, known := r.objects[tr.word(value.from)]; known {
			return []int{open}
		}
		return nil
	}
	if tr.match[value.from] == value.to-1 {
		if tr.is(value.from, "{") {
			return []int{value.from}
		}
		if tr.is(value.from, "(") {
			return r.objectsFilling(tsSpan{value.from + 1, value.to - 1})
		}
	}
	yes, no, ok := conditionalArms(tr, value)
	if !ok {
		return nil
	}
	return append(r.objectsFilling(yes), r.objectsFilling(no)...)
}

// fills reports whether span is slot: itself, cast with `as` or `satisfies`,
// or as an arm of a conditional.
func (r *problemFileReader) fills(span, slot tsSpan) bool {
	tr := r.tree
	if span.from == slot.from && (span.to == slot.to ||
		span.to > slot.to && (tr.word(slot.to) == "as" || tr.word(slot.to) == "satisfies")) {
		return true
	}
	if yes, no, ok := conditionalArms(tr, span); ok {
		return r.fills(yes, slot) || r.fills(no, slot)
	}
	return false
}

func (r *problemFileReader) isStringify(paren int) bool {
	tr := r.tree
	return tr.word(paren-1) == "stringify" && tr.is(paren-2, ".") && tr.word(paren-3) == "JSON"
}

// problemCallee names the function a parenthesis calls; "" for a grouping, and
// "?" for a call whose callee is not one name.
func problemCallee(tr *tsTree, paren int) string {
	if paren == 0 {
		return ""
	}
	before := tr.tokens[paren-1]
	switch {
	case before.kind == tkIdent && !isKeyword(before.text) &&
		!slices.Contains([]string{"if", "while", "for", "switch", "catch", "return"}, before.text):
		return before.text
	case tr.is(paren-1, ")") || tr.is(paren-1, "]") || tr.is(paren-1, ">"):
		return "?"
	}
	return ""
}

// readHelperCalls reads the code each call of a helper passes. A helper that
// passes its own parameter on makes its function a helper too.
func (r *problemFileReader) readHelperCalls() {
	tr := r.tree
	var calls []int
	for i := range tr.tokens {
		if tr.word(i) != "" && tr.is(i+1, "(") && !tr.is(i-1, ".") && tr.word(i-1) != "function" {
			calls = append(calls, i)
			if _, imported := r.scan.imports[tr.word(i)]; imported {
				r.scan.calls = append(r.scan.calls, i)
			}
		}
	}
	read := map[int]bool{}
	for changed := true; changed; {
		changed = false
		for _, call := range calls {
			source, isHelper := r.scan.helpers[tr.word(call)]
			if isHelper && !read[call] {
				read[call] = true
				changed = r.readCall(call, source) || changed
			}
		}
	}
}

// readCall reads the code one helper call passes, and reports whether it
// made the calling function a helper.
func (r *problemFileReader) readCall(call int, source codeSource) bool {
	args := r.tree.split(call + 1)
	if source.index >= len(args) {
		return false
	}
	status, refusal, passed := r.callStatus(args, source.status)
	if !refusal {
		return false
	}
	read, ok := r.resolve(args[source.index], source.path, 0)
	switch {
	case !ok:
		r.scan.mocks = append(r.scan.mocks, problemMock{path: r.path, line: r.line(call), computed: true, status: status})
	case read.helper != "":
		if _, known := r.scan.helpers[read.helper]; !known {
			if passed != nil && passed.helper == read.helper {
				read.source.status = &passed.source
			}
			r.scan.helpers[read.helper] = read.source
			return true
		}
	default:
		r.addMocks(read.literals, status, call, false)
	}
	return false
}

// callStatus judges the status a call passes a helper that takes one. A
// literal decides; one the call takes from its own caller is passed on; one
// it cannot follow may be a refusal.
func (r *problemFileReader) callStatus(args []tsSpan, param *codeSource) (status int, refusal bool, passed *codeRead) {
	if param == nil || param.index >= len(args) {
		return 0, true, nil
	}
	read, ok := r.resolve(args[param.index], param.path, 0)
	switch {
	case ok && read.helper != "":
		return 0, true, &read
	case ok && len(read.literals) > 0:
		status, refusal = refusalOf(r.tree, read.literals)
		return status, refusal, nil
	}
	return 0, true, nil
}
