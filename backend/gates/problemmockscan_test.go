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

// problemMock is one code a frontend file answers as a Problem's.
type problemMock struct {
	path     string
	line     int
	code     string
	computed bool
	status   int
}

// problemShapeKeys are the Problem fields that make a `code` beside them a Problem's.
var problemShapeKeys = []string{"status", "title", "detail", "details"}

// tsFunc is a function with named parameters, and the tokens of its body.
type tsFunc struct {
	name     string
	params   []string
	body     tsSpan
	exported bool
}

// tsImport is one named import: the file it resolves to and its exported name.
type tsImport struct{ from, name string }

// importedCall is a call of an imported name, with its string arguments.
type importedCall struct {
	local string
	line  int
	args  map[int]string
}

// problemFileScan is what one file holds: its mocks, its helpers that make a
// parameter a Problem's code, and its calls of imported names.
type problemFileScan struct {
	mocks   []problemMock
	helpers map[string]int
	funcs   []tsFunc
	imports map[string]tsImport
	calls   []importedCall
}

type problemFileReader struct {
	path    string
	source  string
	tree    *tsTree
	takers  map[string]bool
	consts  map[string]string
	objects map[string]int
	scan    *problemFileScan
}

// scanProblemFile reads one tokenized file. takers are the functions whose
// first argument is a Problem, read from the frontend's Problem module.
func scanProblemFile(filePath, source string, tree *tsTree, takers map[string]bool) *problemFileScan {
	reader := &problemFileReader{
		path: filePath, source: source, tree: tree, takers: takers,
		consts: map[string]string{}, objects: map[string]int{},
		scan: &problemFileScan{helpers: map[string]int{}, imports: map[string]tsImport{}},
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

func (r *problemFileReader) line(i int) int { return lineAt(r.source, r.tree.tokens[i].pos) }

// indexDeclarations records the file's string and object constants, its
// functions and its named imports.
func (r *problemFileReader) indexDeclarations() {
	tr := r.tree
	for i := range tr.tokens {
		word := tr.word(i)
		switch {
		case (word == "const" || word == "let") && tr.word(i+1) != "" && tr.is(i+2, "="):
			if text, _, ok := tr.literal(i + 3); ok && i+3 < len(tr.tokens) {
				r.consts[tr.word(i+1)] = text
			} else if tr.is(i+3, "{") {
				r.objects[tr.word(i+1)] = i + 3
			}
		case word == "import" && tr.is(i+1, "{"):
			r.readImport(i + 1)
		}
		if fn, ok := r.functionAt(i); ok {
			r.scan.funcs = append(r.scan.funcs, fn)
		}
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

// functionAt reads a function that starts at token i: a declaration, an arrow
// or a method, each with its parameter list.
func (r *problemFileReader) functionAt(i int) (tsFunc, bool) {
	tr := r.tree
	exported := tr.word(i-1) == "export" || tr.word(i-2) == "export" && tr.word(i-1) == "async"
	switch {
	case tr.word(i) == "function" && tr.word(i+1) != "" && tr.is(i+2, "("):
		return r.withBody(tsFunc{name: tr.word(i + 1), exported: exported}, i+2)
	case tr.word(i) == "function" && tr.word(i+1) != "" && tr.is(i+2, "<"):
		for j := i + 3; j < len(tr.tokens); j++ {
			if tr.is(j, "(") {
				return r.withBody(tsFunc{name: tr.word(i + 1), exported: exported}, j)
			}
		}
	case (tr.word(i) == "const" || tr.word(i) == "let") && tr.word(i+1) != "" && tr.is(i+2, "="):
		start := i + 3
		if tr.word(start) == "async" {
			start++
		}
		name := tr.word(i + 1)
		if tr.word(start) != "" && tr.is(start+1, "=>") {
			return tsFunc{
				name: name, params: []string{tr.word(start)}, body: r.arrowBody(start + 1), exported: exported,
			}, true
		}
		if tr.is(start, "(") {
			return r.withBody(tsFunc{name: name, exported: exported}, start)
		}
	}
	return tsFunc{}, false
}

// withBody completes a function whose parameter list opens at paren: a block
// body, or an arrow's body to the end of its expression.
func (r *problemFileReader) withBody(fn tsFunc, paren int) (tsFunc, bool) {
	tr := r.tree
	for _, part := range tr.split(paren) {
		first := part.from
		if tr.is(first, "...") {
			first++
		}
		fn.params = append(fn.params, tr.word(first))
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

// arrowBody answers the tokens after an arrow: a block, or an expression up
// to the comma, semicolon or closer that ends it.
func (r *problemFileReader) arrowBody(arrow int) tsSpan {
	tr := r.tree
	if tr.is(arrow+1, "{") {
		return tsSpan{arrow + 1, tr.match[arrow+1] + 1}
	}
	for j := arrow + 1; j < len(tr.tokens); j++ {
		if tr.match[j] > j {
			j = tr.match[j]
			continue
		}
		if tr.is(j, ",") || tr.is(j, ";") || tr.match[j] >= 0 && tr.match[j] < j {
			return tsSpan{arrow + 1, j}
		}
	}
	return tsSpan{arrow + 1, len(tr.tokens)}
}

// readObject judges one brace that holds a `code` entry.
func (r *problemFileReader) readObject(open int) {
	entry, found := codeEntry(r.tree.entries(open))
	if !found {
		return
	}
	status, problem := r.problemContext(open, map[int]bool{})
	if !problem {
		return
	}
	value := entry.value
	if texts, computed, ok := r.tree.strings(value); ok {
		for _, text := range texts {
			r.scan.mocks = append(r.scan.mocks, problemMock{
				path: r.path, line: r.line(entry.keyAt), code: text, computed: computed, status: status,
			})
		}
		return
	}
	if value.to != value.from+1 || r.tree.word(value.from) == "" {
		return
	}
	name := r.tree.word(value.from)
	if text, known := r.consts[name]; known {
		r.scan.mocks = append(r.scan.mocks, problemMock{path: r.path, line: r.line(entry.keyAt), code: text, status: status})
		return
	}
	if fn, index, ok := r.paramOf(value.from, name); ok && fn.name != "" {
		r.scan.helpers[fn.name] = index
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

// paramOf answers the innermost function whose body holds token at and whose
// parameters name it.
func (r *problemFileReader) paramOf(at int, name string) (tsFunc, int, bool) {
	best, bestIndex, found := tsFunc{}, 0, false
	for _, fn := range r.scan.funcs {
		if at < fn.body.from || at >= fn.body.to {
			continue
		}
		if index := slices.Index(fn.params, name); index >= 0 && (!found || fn.body.from > best.body.from) {
			best, bestIndex, found = fn, index, true
		}
	}
	return best, bestIndex, found
}

// problemContext reports whether the object at open is a Problem. Its keys
// say so, or a Problem it is spread into, or a 4xx or 5xx answer it is the
// body of. status is the one a literal sends it with, or 0.
func (r *problemFileReader) problemContext(open int, seen map[int]bool) (int, bool) {
	tr := r.tree
	if seen[open] {
		return 0, false
	}
	seen[open] = true
	status, _ := statusOf(tr, tr.entries(open))
	if r.hasShapeKey(open, map[int]bool{}) {
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
			if code, refusal := statusOf(tr, tr.entries(arg.from)); refusal {
				return code, true
			}
			continue
		}
		named := arg.to == arg.from+1 && strings.HasSuffix(strings.ToLower(tr.word(arg.from)), "status")
		if code, refusal := sentStatus(tr, arg); refusal && (named || code > 0) {
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
		if code, refusal := statusOf(tr, tr.entries(brace)); refusal {
			return code, true
		}
		return 0, false
	}
	return 0, false
}

func refusalStatus(code int) bool { return code >= 400 && code <= 599 }

// statusOf reads an object's status entry: the refusal status a literal
// names, and whether the entry can be a refusal at all.
func statusOf(tr *tsTree, entries []tsEntry) (int, bool) {
	for _, entry := range entries {
		if entry.key == "status" && !entry.spread {
			return sentStatus(tr, entry.value)
		}
	}
	return 0, false
}

// sentStatus reads a status expression. A literal is a refusal when it is
// 4xx or 5xx, a conditional when either arm is, and anything computed may be.
func sentStatus(tr *tsTree, span tsSpan) (int, bool) {
	if span.to == span.from+1 {
		if code, ok := unquotedNumber(tr.tokens[span.from]); ok {
			return code, refusalStatus(code)
		}
	}
	if yes, no, ok := conditionalArms(tr, span); ok {
		yesCode, yesRefusal := sentStatus(tr, yes)
		noCode, noRefusal := sentStatus(tr, no)
		if yesRefusal && yesCode > 0 {
			return yesCode, true
		}
		return noCode, noRefusal || yesRefusal
	}
	return 0, true
}

// hasShapeKey reports a Problem key on the object, or on one it spreads in
// by name or as a literal.
func (r *problemFileReader) hasShapeKey(open int, seen map[int]bool) bool {
	if seen[open] {
		return false
	}
	seen[open] = true
	for _, entry := range r.tree.entries(open) {
		if slices.Contains(problemShapeKeys, entry.key) && !entry.spread {
			return true
		}
		if !entry.spread {
			continue
		}
		for _, spread := range r.objectsFilling(entry.value) {
			if r.hasShapeKey(spread, seen) {
				return true
			}
		}
	}
	return false
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

// conditionalArms splits `cond ? yes : no` at its depth-0 operators.
func conditionalArms(tr *tsTree, value tsSpan) (yes, no tsSpan, ok bool) {
	question := -1
	for i := value.from; i < value.to; i++ {
		if tr.match[i] > i {
			i = tr.match[i]
			continue
		}
		if tr.is(i, "?") && question < 0 {
			question = i
		} else if tr.is(i, ":") && question >= 0 {
			return tsSpan{question + 1, i}, tsSpan{i + 1, value.to}, true
		}
	}
	return tsSpan{}, tsSpan{}, false
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
				r.recordImportedCall(i, tr.word(i), tr.split(i+1))
			}
		}
	}
	read := map[int]bool{}
	for changed := true; changed; {
		changed = false
		for _, i := range calls {
			index, isHelper := r.scan.helpers[tr.word(i)]
			args := tr.split(i + 1)
			if !isHelper || index >= len(args) || read[i] {
				continue
			}
			read[i] = true
			changed = r.readHelperArgument(args[index]) || changed
		}
	}
}

func (r *problemFileReader) readHelperArgument(arg tsSpan) bool {
	tr := r.tree
	if texts, computed, ok := tr.strings(arg); ok {
		for _, text := range texts {
			r.scan.mocks = append(r.scan.mocks, problemMock{path: r.path, line: r.line(arg.from), code: text, computed: computed})
		}
		return false
	}
	if arg.to != arg.from+1 || tr.word(arg.from) == "" {
		return false
	}
	fn, index, ok := r.paramOf(arg.from, tr.word(arg.from))
	if _, known := r.scan.helpers[fn.name]; !ok || fn.name == "" || known {
		return false
	}
	r.scan.helpers[fn.name] = index
	return true
}

func (r *problemFileReader) recordImportedCall(at int, local string, args []tsSpan) {
	call := importedCall{local: local, line: r.line(at), args: map[int]string{}}
	for index, arg := range args {
		if texts, computed, ok := r.tree.strings(arg); ok && len(texts) == 1 && !computed {
			call.args[index] = texts[0]
		}
	}
	if len(call.args) > 0 {
		r.scan.calls = append(r.scan.calls, call)
	}
}
