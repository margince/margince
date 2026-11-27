// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

type jsFile struct {
	code  string
	tree  *tsTree
	calls []jsCall
}

type jsCall struct {
	callee string
	// at is the token index of the call's "(".
	at int
}

var (
	jsDeclarers = []string{"const", "let", "var"}
	// jsStatements open a parenthesis without calling anything.
	jsStatements = []string{"if", "for", "while", "switch", "catch", "function", "return", "typeof", "await", "with"}
	jsIterators  = []string{"map", "flatMap", "forEach", "filter", "some", "every", "find", "findIndex"}
	eachTable    = regexp.MustCompile(`^(?:it|test|describe|bench)(?:\.(?:only|skip|concurrent|sequential))*\.each$`)
)

func parseJSFile(file, source string) (*jsFile, error) {
	ext := path.Ext(file)
	tokens, comments, err := tokenizeTS(source, ext == ".tsx" || ext == ".jsx")
	if err != nil {
		return nil, err
	}
	tree, err := buildTSTree(tokens)
	if err != nil {
		return nil, err
	}
	tree.comments = comments
	code := gatekit.TSWithoutComments(source)
	parsed := &jsFile{code: code, tree: tree}
	for i := range tree.tokens {
		if !tree.is(i, "(") {
			continue
		}
		if callee, first := parsed.calleeBefore(i); callee != "" && !slices.Contains(jsStatements, tree.word(first)) && tree.word(first-1) != "function" {
			parsed.calls = append(parsed.calls, jsCall{callee, i})
		}
	}
	return parsed, nil
}

// A regular expression's token sits at its end, so raw moves it back.
func (f *jsFile) start(i int) int {
	if i >= len(f.tree.tokens) {
		return len(f.code)
	}
	token := f.tree.tokens[i]
	return token.pos - len(token.raw)
}

func (f *jsFile) text(from, to int) string {
	if from >= to {
		return ""
	}
	return f.code[f.start(from):f.start(to)]
}

// calleeBefore keeps the receivers, as in path.posix.join.
func (f *jsFile) calleeBefore(i int) (string, int) {
	tr := f.tree
	first := i - 1
	if tr.word(first) == "" {
		return "", -1
	}
	for tr.is(first-1, ".") || tr.is(first-1, "?.") {
		switch receiver := first - 2; {
		case tr.word(receiver) != "":
			first = receiver
		case tr.is(receiver, ")") || tr.is(receiver, "]"):
			first = tr.match[receiver]
			if tr.word(first-1) != "" && !tr.is(first-2, ".") {
				first--
			}
		default:
			return "", -1
		}
	}
	return strings.TrimSpace(f.text(first, i)), first
}

func (f *jsFile) arg(open, position int) string {
	parts := f.tree.split(open)
	if position >= len(parts) {
		return ""
	}
	return f.text(parts[position].from, parts[position].to)
}

func (f *jsFile) args(open int) []string {
	var args []string
	for _, part := range f.tree.split(open) {
		args = append(args, f.text(part.from, part.to))
	}
	return args
}

// statementEnd is the first ";" or "," at the level of token from, or the bracket
// that closes that level.
func (f *jsFile) statementEnd(from int) int {
	tr := f.tree
	for i := from; i < len(tr.tokens); i++ {
		switch {
		case tr.match[i] > i:
			i = tr.match[i]
		case tr.is(i, ";") || tr.is(i, ",") || tr.match[i] >= 0 && tr.match[i] < i:
			return i
		}
	}
	return len(tr.tokens)
}

// declaration is the initialiser of the first const, let or var of that name.
func (f *jsFile) declaration(name string) (string, bool) {
	at, ok := f.initialiser(name)
	if !ok {
		return "", false
	}
	return f.text(at, f.statementEnd(at)), true
}

func (f *jsFile) initialiser(name string) (int, bool) {
	tr := f.tree
	for i := range tr.tokens {
		if !slices.Contains(jsDeclarers, tr.word(i)) || tr.word(i+1) != name {
			continue
		}
		for j := i + 2; j < len(tr.tokens); j++ {
			switch {
			case tr.match[j] > j:
				j = tr.match[j]
			case tr.is(j, "="):
				return j + 1, true
			case tr.is(j, ";") || tr.is(j, ",") || tr.match[j] >= 0 || tr.word(j) == "of" || tr.word(j) == "in":
				j = len(tr.tokens)
			}
		}
	}
	return 0, false
}

// declaredBefore is the name a "const x = " right before the callee of the call at
// open binds.
func (f *jsFile) declaredBefore(open int) (string, bool) {
	tr := f.tree
	first := open - 1
	if tr.is(first-1, "=") && tr.word(first-2) != "" && slices.Contains(jsDeclarers, tr.word(first-3)) {
		return tr.word(first - 2), true
	}
	return "", false
}

// specifiers is the module each import or export statement names.
func (f *jsFile) specifiers() []string {
	tr := f.tree
	var found []string
	for i := range tr.tokens {
		if word := tr.word(i); word != "import" && word != "export" || tr.is(i+1, "(") || tr.is(i-1, ".") {
			continue
		}
		for j := i + 1; j < len(tr.tokens) && j < i+400 && !tr.is(j, ";"); j++ {
			if tr.tokens[j].kind == tkString && (j == i+1 || tr.word(j-1) == "from") {
				found = append(found, tr.tokens[j].text)
				break
			}
		}
	}
	return found
}

func (f *jsFile) regexSources() []string {
	var found []string
	for _, token := range f.tree.tokens {
		if token.raw != "" {
			found = append(found, token.raw[1:strings.LastIndexByte(token.raw, '/')])
		}
	}
	return found
}

// paramIndex answers -1 when name is not among the parameters.
func (f *jsFile) paramIndex(open int, name string) int {
	tr := f.tree
	for position, part := range tr.split(open) {
		first := part.from
		if tr.is(first, "...") {
			first++
		}
		if tr.word(first) == name {
			return position
		}
	}
	return -1
}

// evalName follows a name to what binds it: a declaration, a for-of, a parameter
// or an import.
func (r *tsReader) evalName(file, name string, depth int) []pathValue {
	key := file + "\x00" + name
	if values, done := r.names[key]; done {
		return values
	}
	if r.working[key] {
		r.cut++
		return nil
	}
	r.working[key] = true
	cut := r.cut
	values := r.bindings(file, name, depth)
	delete(r.working, key)
	if r.cut == cut {
		r.names[key] = values
	}
	return values
}

func (r *tsReader) bindings(file, name string, depth int) []pathValue {
	parsed := r.files[file]
	if parsed == nil {
		return nil
	}
	if initialiser, ok := parsed.declaration(name); ok {
		return r.eval(file, initialiser, depth)
	}
	tr := parsed.tree
	var out []pathValue
	for i := range tr.tokens {
		if tr.word(i) == "for" && tr.is(i+1, "(") && slices.Contains(jsDeclarers, tr.word(i+2)) && tr.word(i+3) == name && tr.word(i+4) == "of" {
			out = append(out, r.eval(file, parsed.text(i+5, tr.match[i+1]), depth)...)
		}
		if tr.word(i) == "function" && tr.word(i+1) != "" && tr.is(i+2, "(") {
			if position := parsed.paramIndex(i+2, name); position >= 0 {
				out = append(out, r.callerValues(file, tr.word(i+1), position, depth)...)
			}
		}
		if tr.is(i, "=>") {
			out = append(out, r.arrowValues(file, i, name, depth)...)
		}
	}
	if len(out) > 0 {
		return out[:min(len(out), maxPathValues)]
	}
	if imported, ok := r.importOf(file, name); ok && imported.module != "" && imported.original != "*" {
		return r.evalName(imported.module, imported.original, depth+1)
	}
	return nil
}

// arrowParams is the token index of the arrow's first parameter token: its "("
// or its one bare name, past a return type.
func (f *jsFile) arrowParams(arrow int) (open int, bare bool) {
	tr := f.tree
	if tr.word(arrow-1) != "" && !tr.is(arrow-2, ":") {
		return arrow - 1, true
	}
	for i := arrow - 1; i >= 0 && i > arrow-12; i-- {
		if tr.is(i, ")") && (i == arrow-1 || tr.is(i+1, ":")) {
			return tr.match[i], false
		}
	}
	return -1, false
}

// arrowValues is what reaches the arrow's parameter name. It comes from the callers
// of the name the arrow is assigned to, the array an iteration walks, or a table.
func (r *tsReader) arrowValues(file string, arrow int, name string, depth int) []pathValue {
	parsed := r.files[file]
	tr := parsed.tree
	open, bare := parsed.arrowParams(arrow)
	position := -1
	switch {
	case open < 0:
		return nil
	case bare && tr.word(open) == name:
		position = 0
	case !bare:
		position = parsed.paramIndex(open, name)
	}
	if position < 0 {
		return nil
	}
	before := open - 1
	if tr.word(before) == "async" {
		before--
	}
	if tr.is(before, "=") {
		for i := before - 1; i > before-12 && i > 0; i-- {
			if slices.Contains(jsDeclarers, tr.word(i-1)) && tr.word(i) != "" {
				return r.callerValues(file, tr.word(i), position, depth)
			}
		}
		return nil
	}
	call := tr.parent[open]
	if !tr.is(call, "(") {
		return nil
	}
	if callee, _ := parsed.calleeBefore(call); callee != "" {
		dot := strings.LastIndexByte(callee, '.')
		if dot > 0 && position == 0 && slices.Contains(jsIterators, callee[dot+1:]) {
			return r.eval(file, callee[:dot], depth)
		}
		return nil
	}
	return r.tableValues(file, call, position, depth)
}

// tableValues reads the it.each table whose returned function is called at call.
func (r *tsReader) tableValues(file string, call, position, depth int) []pathValue {
	parsed := r.files[file]
	tr := parsed.tree
	if !tr.is(call-1, ")") {
		return nil
	}
	tableOpen := tr.match[call-1]
	if callee, _ := parsed.calleeBefore(tableOpen); !eachTable.MatchString(callee) {
		return nil
	}
	table := tr.split(tableOpen)
	if len(table) != 1 || !tr.is(table[0].from, "[") {
		return nil
	}
	var out []pathValue
	for _, row := range tr.split(table[0].from) {
		switch {
		case tr.is(row.from, "[") && tr.match[row.from] == row.to-1:
			if cells := tr.split(row.from); position < len(cells) {
				out = append(out, r.eval(file, parsed.text(cells[position].from, cells[position].to), depth)...)
			}
		case position == 0:
			out = append(out, r.eval(file, parsed.text(row.from, row.to), depth)...)
		}
	}
	return out
}

// callerValues is what the callers of function pass at position. They are in
// its own file and in each file that imports it.
func (r *tsReader) callerValues(file, function string, position, depth int) []pathValue {
	sites := []jsImport{{file: file, local: function}}
	for _, imported := range r.importers[file] {
		switch imported.original {
		case function:
			sites = append(sites, imported)
		case "*":
			sites = append(sites, jsImport{file: imported.file, local: imported.local + "." + function})
		}
	}
	var out []pathValue
	for _, site := range sites {
		parsed := r.files[site.file]
		if parsed == nil {
			continue
		}
		for _, call := range parsed.calls {
			if call.callee == site.local {
				out = append(out, r.eval(site.file, parsed.arg(call.at, position), depth)...)
			}
		}
	}
	return out
}

// returnValues is what a function or arrow of that name returns, in this file or
// the module it is imported from.
func (r *tsReader) returnValues(file, function string, depth int) []pathValue {
	if depth > maxEvalDepth {
		r.cut++
		return nil
	}
	parsed := r.files[file]
	if parsed == nil || !jsIdentifier.MatchString(function) {
		return nil
	}
	tr := parsed.tree
	body := -1
	for i := range tr.tokens {
		if tr.word(i) == "function" && tr.word(i+1) == function && tr.is(i+2, "(") {
			for j := tr.match[i+2] + 1; j < len(tr.tokens) && body < 0; j++ {
				if tr.is(j, "{") {
					body = j
				}
			}
			break
		}
	}
	if at, ok := parsed.initialiser(function); body < 0 && ok {
		for i := at; i < parsed.statementEnd(at); i++ {
			if !tr.is(i, "=>") {
				continue
			}
			if !tr.is(i+1, "{") {
				return r.eval(file, parsed.text(i+1, parsed.statementEnd(i+1)), depth+1)
			}
			body = i + 1
			break
		}
	}
	if body < 0 {
		if imported, ok := r.importOf(file, function); ok && imported.module != "" {
			return r.returnValues(imported.module, imported.original, depth+1)
		}
		return nil
	}
	var out []pathValue
	for i := body + 1; i < tr.match[body]; i++ {
		if tr.word(i) == "return" {
			out = append(out, r.eval(file, parsed.text(i+1, parsed.statementEnd(i+1)), depth+1)...)
		}
	}
	return out
}
