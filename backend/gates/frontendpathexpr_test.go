// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

import (
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// maxPathValues bounds the values one path expression can take, so a join over
// several lists cannot grow without end.
const maxPathValues = 64

const maxEvalDepth = 10

// pathValue is one value of a path expression: a repository path when anchored,
// otherwise a relative path whose directory is not known.
type pathValue struct {
	path     string
	anchored bool
}

// jsImport is one name a file brings in. original is "*" for a namespace and
// "default" for a default import; module is the tracked file it names.
type jsImport struct {
	file, local, original, specifier, module string
}

type tsReader struct {
	tree      repoTree
	code      map[string]string
	imports   map[string][]jsImport
	importers map[string][]jsImport
	patterns  map[string]*regexp.Regexp
	names     map[string][]pathValue
	working   map[string]bool
	sites     map[string]jsSites
	// cut counts evaluations a cycle or the depth bound stopped short. A name worked
	// out across one is not kept, so no result depends on the order of files.
	cut int
}

var (
	namedImport     = regexp.MustCompile(`^import\s+(?:type\s+)?(?:([\w$]+)\s*,\s*)?\{([^}]*)\}\s*from\s*["']([^"']+)["']`)
	namespaceImport = regexp.MustCompile(`^import\s+\*\s+as\s+([\w$]+)\s+from\s*["']([^"']+)["']`)
	defaultImport   = regexp.MustCompile(`^import\s+([\w$]+)\s*(?:,\s*\{[^}]*\})?\s*from\s*["']([^"']+)["']`)
	requireBinding  = regexp.MustCompile(`(?:const|let|var)\s+([\w$]+)\s*=\s*$`)
	jsIdentifier    = regexp.MustCompile(`^[A-Za-z_$][\w$]*$`)
	jsMember        = regexp.MustCompile(`^([A-Za-z_$][\w$]*)\.([A-Za-z_$][\w$]*)$`)
	jsCallee        = regexp.MustCompile(`^(?:new\s+)?([\w$.]+)\s*\(`)
	jsFunction      = regexp.MustCompile(`\bfunction\s+([\w$]+)\s*\(([^)]*)\)`)
	jsArrow         = regexp.MustCompile(`(?:\(([^()]*)\)|\b([A-Za-z_$][\w$]*))\s*(?::\s*[^=;{}()]+?)?\s*=>`)
	arrayFill       = regexp.MustCompile(`^(?:new\s+)?Array\(\s*(\d+)\s*\)\.fill\(([\s\S]+)\)$`)
	jsAssignedTo    = regexp.MustCompile(`(?:const|let|var)\s+([\w$]+)\s*(?::[^=]+)?=\s*(?:async\s*)?$`)
	iteratedBy      = regexp.MustCompile(`\.(?:map|flatMap|forEach|filter|some|every|find|findIndex)\(\s*(?:async\s*)?$`)
	eachTable       = regexp.MustCompile(`\b(?:it|test|describe|bench)(?:\.(?:only|skip|concurrent|sequential))*\.each\s*$`)
)

var pathModules = []string{"path", "node:path", "path/posix", "node:path/posix", "url", "node:url"}

func newTSReader(tree repoTree, sources map[string]string) *tsReader {
	r := &tsReader{tree: tree, code: map[string]string{}, imports: map[string][]jsImport{},
		importers: map[string][]jsImport{}, patterns: map[string]*regexp.Regexp{}, names: map[string][]pathValue{},
		working: map[string]bool{}, sites: map[string]jsSites{}}
	for file, source := range sources {
		r.add(file, source)
	}
	return r
}

// add takes in one more source. A name already worked out stays as it was.
func (r *tsReader) add(file, source string) {
	code, _ := stripTSComments(source)
	r.code[file] = code
	r.imports[file] = r.parseImports(file, code)
	for _, imported := range r.imports[file] {
		if imported.module != "" {
			r.importers[imported.module] = append(r.importers[imported.module], imported)
		}
	}
}

func (r *tsReader) parseImports(file, code string) []jsImport {
	var found []jsImport
	add := func(local, original, specifier string) {
		module, _ := r.module(file, specifier)
		found = append(found, jsImport{file, local, original, specifier, module})
	}
	sites := scanJSSites(code)
	r.sites[file] = sites
	for _, statement := range sites.imports {
		if match := namedImport.FindStringSubmatch(statement); match != nil {
			if match[1] != "" {
				add(match[1], "default", match[3])
			}
			for binding := range strings.SplitSeq(match[2], ",") {
				original, local, renamed := strings.Cut(strings.TrimPrefix(strings.TrimSpace(binding), "type "), " as ")
				if !renamed {
					local = original
				}
				if local = strings.TrimSpace(local); local != "" {
					add(local, strings.TrimSpace(original), match[3])
				}
			}
		} else if match := namespaceImport.FindStringSubmatch(statement); match != nil {
			add(match[1], "*", match[2])
		} else if match := defaultImport.FindStringSubmatch(statement); match != nil && match[1] != "type" {
			add(match[1], "default", match[2])
		}
	}
	for _, call := range sites.calls {
		if call.callee != "require" {
			continue
		}
		args, _ := callArgs(code, call.open)
		text, ok := jsStringLiteral(strings.TrimSpace(firstArg(args)))
		if bound := requireBinding.FindStringSubmatch(code[max(0, call.start-80):call.start]); ok && bound != nil {
			add(bound[1], "*", text)
		}
	}
	return found
}

func (r *tsReader) importOf(file, local string) (jsImport, bool) {
	for _, imported := range r.imports[file] {
		if imported.local == local {
			return imported, true
		}
	}
	return jsImport{}, false
}

// importedFrom reports whether a local name is the module itself, or one of its
// exports, imported from any of the given specifiers.
func (r *tsReader) importedFrom(file, local string, specifiers []string) (original string, ok bool) {
	imported, found := r.importOf(file, local)
	if !found || !slices.Contains(specifiers, imported.specifier) {
		return "", false
	}
	return imported.original, true
}

func (r *tsReader) pattern(expr string) *regexp.Regexp {
	compiled, ok := r.patterns[expr]
	if !ok {
		compiled = regexp.MustCompile(expr)
		r.patterns[expr] = compiled
	}
	return compiled
}

// pathFunction names the node:path or node:url function a callee is: bare as
// imported, or on the module however it was bound (path.posix.join, nodePath.join).
func (r *tsReader) pathFunction(file, callee string) (string, bool) {
	known := []string{"join", "resolve", "dirname", "normalize", "fileURLToPath"}
	segments := strings.Split(callee, ".")
	method := segments[len(segments)-1]
	if !slices.Contains(known, method) {
		if original, ok := r.importedFrom(file, callee, pathModules); ok && slices.Contains(known, original) {
			return original, true
		}
		return "", false
	}
	if len(segments) == 1 {
		original, imported := r.importedFrom(file, callee, pathModules)
		return method, !imported || original == method
	}
	receiver := segments[0]
	_, aliased := r.importedFrom(file, receiver, pathModules)
	if !aliased && !slices.Contains([]string{"path", "posix", "win32", "url"}, receiver) {
		return "", false
	}
	for _, middle := range segments[1 : len(segments)-1] {
		if middle != "posix" && middle != "win32" {
			return "", false
		}
	}
	return method, true
}

func (r *tsReader) eval(file, expr string, depth int) []pathValue {
	expr = strings.TrimSpace(expr)
	expr = strings.TrimSuffix(strings.TrimSuffix(expr, ".pathname"), ".href")
	if depth > maxEvalDepth {
		r.cut++
		return nil
	}
	if expr == "" {
		return nil
	}
	switch expr {
	case "import.meta.dirname", "__dirname":
		return []pathValue{{path.Dir(file), true}}
	case "import.meta.url", "import.meta.filename", "__filename":
		return []pathValue{{file, true}}
	}
	if operands := splitJSTopLevel(expr, '+'); len(operands) > 1 {
		return r.concat(file, operands, depth+1)
	}
	if len(expr) > 1 && expr[0] == '`' && expr[len(expr)-1] == '`' && strings.Contains(expr, "${") {
		if values := r.evalTemplate(file, expr[1:len(expr)-1], depth+1); len(values) > 0 {
			return values
		}
	}
	if text, ok := jsStringLiteral(expr); ok {
		if text == "" {
			return nil
		}
		return []pathValue{{path: text}}
	}
	if strings.HasPrefix(expr, "[") {
		elements, end := callArgs(expr, 0)
		if end != len(expr)-1 {
			return nil
		}
		return r.union(file, elements, depth+1)
	}
	if jsIdentifier.MatchString(expr) {
		return r.evalName(file, expr, depth+1)
	}
	if member := jsMember.FindStringSubmatch(expr); member != nil {
		if imported, ok := r.importOf(file, member[1]); ok && imported.original == "*" && imported.module != "" {
			return r.evalName(imported.module, member[2], depth+1)
		}
	}
	match := jsCallee.FindStringSubmatch(expr)
	if match == nil {
		return nil
	}
	args, end := callArgs(expr, len(match[0])-1)
	if end != len(expr)-1 {
		return nil
	}
	return r.evalCall(file, match[1], args, depth+1)
}

func (r *tsReader) union(file string, exprs []string, depth int) []pathValue {
	var out []pathValue
	for _, expr := range exprs {
		out = append(out, r.eval(file, expr, depth)...)
	}
	return out[:min(len(out), maxPathValues)]
}

// splitJSTopLevel cuts an expression at each sep outside quotes and brackets.
func splitJSTopLevel(expr string, sep byte) []string {
	var parts []string
	depth, start := 0, 0
	for i := 0; i < len(expr); i++ {
		switch c := expr[i]; c {
		case '"', '\'', '`':
			i = literalEnd(expr, i)
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, expr[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, expr[start:])
}

// spell joins pieces of text into each value they can take together, as a
// template literal or a + chain does.
func spell(values []pathValue, parts []pathValue) []pathValue {
	var next []pathValue
	for _, value := range values {
		for _, part := range parts {
			if value.path == "" && !value.anchored {
				next = append(next, part)
			} else {
				next = append(next, pathValue{value.path + part.path, value.anchored})
			}
		}
	}
	return next[:min(len(next), maxPathValues)]
}

func (r *tsReader) concat(file string, operands []string, depth int) []pathValue {
	values := []pathValue{{}}
	for _, operand := range operands {
		parts := r.eval(file, operand, depth)
		if len(parts) == 0 {
			return nil
		}
		values = spell(values, parts)
	}
	return values
}

// evalTemplate spells a template literal once for each value its substitutions
// can take, or not at all when one of them cannot be evaluated.
func (r *tsReader) evalTemplate(file, inner string, depth int) []pathValue {
	values := []pathValue{{}}
	for inner != "" {
		literal, rest, substituted := strings.Cut(inner, "${")
		values = spell(values, []pathValue{{path: literal}})
		if !substituted {
			break
		}
		_, end := callArgs(rest, -1)
		if end < 0 {
			return nil
		}
		parts := r.eval(file, rest[:end], depth)
		if len(parts) == 0 {
			return nil
		}
		values = spell(values, parts)
		inner = rest[end+1:]
	}
	return values
}

func (r *tsReader) evalCall(file, callee string, args []string, depth int) []pathValue {
	var out []pathValue
	if callee == "URL" {
		if len(args) < 2 {
			return nil
		}
		for _, base := range r.eval(file, args[1], depth) {
			for _, rel := range r.eval(file, args[0], depth) {
				if base.anchored {
					out = append(out, pathValue{path.Join(path.Dir(base.path), rel.path), true})
				}
			}
		}
		return out
	}
	function, ok := r.pathFunction(file, callee)
	if !ok {
		return r.returnValues(file, callee, depth)
	}
	switch function {
	case "join", "resolve":
		return r.evalJoin(file, args, depth)
	case "dirname":
		for _, value := range r.eval(file, firstArg(args), depth) {
			if value.anchored {
				out = append(out, pathValue{path.Dir(value.path), true})
			}
		}
		return out
	default:
		return r.eval(file, firstArg(args), depth)
	}
}

// evalJoin folds join and resolve over every value each argument can take. An
// argument it cannot evaluate ends the path, and a first one leaves none.
func (r *tsReader) evalJoin(file string, args []string, depth int) []pathValue {
	values := []pathValue{{}}
	for i, arg := range args {
		segments := []string{arg}
		if spread, ok := strings.CutPrefix(strings.TrimSpace(arg), "..."); ok {
			if segments, ok = r.spreadElements(file, spread, depth); !ok {
				segments = nil
			}
		}
		if len(segments) == 0 && i == 0 {
			return nil
		}
		if len(segments) == 0 {
			return partialValues(values)
		}
		for _, segment := range segments {
			parts := r.eval(file, segment, depth)
			if len(parts) == 0 && i == 0 {
				return nil
			}
			if len(parts) == 0 {
				return partialValues(values)
			}
			var next []pathValue
			for _, value := range values {
				for _, part := range parts {
					if !part.anchored {
						part = pathValue{path.Join(value.path, part.path), value.anchored}
					}
					next = append(next, part)
				}
			}
			values = next[:min(len(next), maxPathValues)]
		}
	}
	return joinedValues(values)
}

func joinedValues(values []pathValue) []pathValue {
	var out []pathValue
	for _, value := range values {
		if value.anchored || value.path != "" && value.path != "." {
			out = append(out, value)
		}
	}
	return out
}

// partialValues is a join cut short at an argument it cannot evaluate. The
// repository root drops out: a path built on it from such a name names nothing.
func partialValues(values []pathValue) []pathValue {
	var out []pathValue
	for _, value := range joinedValues(values) {
		if path.Clean(value.path) != "." {
			out = append(out, value)
		}
	}
	return out
}

// spreadElements is the element expressions a spread argument stands for, in order.
func (r *tsReader) spreadElements(file, expr string, depth int) ([]string, bool) {
	expr = strings.TrimSpace(expr)
	if fill := arrayFill.FindStringSubmatch(expr); fill != nil {
		count, err := strconv.Atoi(fill[1])
		if err != nil || count > maxPathValues {
			return nil, false
		}
		return slices.Repeat([]string{fill[2]}, count), true
	}
	if strings.HasPrefix(expr, "[") {
		elements, end := callArgs(expr, 0)
		return elements, end == len(expr)-1
	}
	if jsIdentifier.MatchString(expr) && depth < maxEvalDepth {
		if initialiser, ok := r.declaration(file, expr); ok {
			return r.spreadElements(file, initialiser, depth+1)
		}
	}
	return nil, false
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func (r *tsReader) declaration(file, name string) (string, bool) {
	code := r.code[file]
	at := r.pattern(`(?:^|[^\w$.])(?:const|let|var)\s+` + regexp.QuoteMeta(name) + `\s*(?::[^=;]+)?=\s*`).FindStringIndex(code)
	if at == nil {
		return "", false
	}
	return expressionAt(code, at[1]), true
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
	if initialiser, ok := r.declaration(file, name); ok {
		return r.eval(file, initialiser, depth)
	}
	code := r.code[file]
	var out []pathValue
	for _, at := range r.pattern(`\bfor\s*\(\s*(?:const|let|var)\s+`+regexp.QuoteMeta(name)+`\s+of\s+`).FindAllStringIndex(code, -1) {
		open := strings.IndexByte(code[at[0]:], '(') + at[0]
		if _, end := callArgs(code, open); end > at[1] {
			out = append(out, r.eval(file, code[at[1]:end], depth)...)
		}
	}
	out = append(out, r.parameterValues(file, name, depth)...)
	if len(out) > 0 {
		return out[:min(len(out), maxPathValues)]
	}
	if imported, ok := r.importOf(file, name); ok && imported.module != "" && imported.original != "*" {
		return r.evalName(imported.module, imported.original, depth+1)
	}
	return nil
}

func paramPosition(params, name string) int {
	if strings.ContainsAny(params, "{[") {
		return -1
	}
	return slices.IndexFunc(strings.Split(params, ","), func(param string) bool {
		param, _, _ = strings.Cut(param, "=")
		param, _, _ = strings.Cut(param, ":")
		return strings.TrimSuffix(strings.TrimSpace(param), "?") == name
	})
}

// parameterValues is what the callers, or an iteration, hand a parameter.
func (r *tsReader) parameterValues(file, name string, depth int) []pathValue {
	code := r.code[file]
	var out []pathValue
	for _, declared := range jsFunction.FindAllStringSubmatchIndex(code, -1) {
		if position := paramPosition(code[declared[4]:declared[5]], name); position >= 0 {
			out = append(out, r.callerValues(file, code[declared[2]:declared[3]], position, depth)...)
		}
	}
	for _, arrow := range jsArrow.FindAllStringSubmatchIndex(code, -1) {
		params := ""
		if arrow[2] >= 0 {
			params = code[arrow[2]:arrow[3]]
		} else {
			params = code[arrow[4]:arrow[5]]
		}
		if position := paramPosition(params, name); position >= 0 {
			out = append(out, r.arrowValues(file, arrow[0], position, depth)...)
		}
	}
	return out
}

// arrowValues reads how the arrow at offset is called. That is by its name, by
// an array iteration, or as the body of an it.each table.
func (r *tsReader) arrowValues(file string, offset, position int, depth int) []pathValue {
	code := r.code[file]
	before := strings.TrimRight(code[:offset], " \t\n")
	if assigned := jsAssignedTo.FindStringSubmatch(before); assigned != nil {
		return r.callerValues(file, assigned[1], position, depth)
	}
	if at := iteratedBy.FindStringIndex(before); at != nil && position == 0 {
		return r.eval(file, receiverBefore(code, at[0]), depth)
	}
	call := openerBefore(code, offset)
	if call <= 0 {
		return nil
	}
	head := strings.TrimRight(code[:call], " \t\n")
	if !strings.HasSuffix(head, ")") {
		return nil
	}
	tableOpen := matchingOpener(head, len(head)-1)
	if tableOpen < 0 || !eachTable.MatchString(head[:tableOpen]) {
		return nil
	}
	table, _ := callArgs(code, tableOpen)
	if len(table) != 1 || !strings.HasPrefix(strings.TrimSpace(table[0]), "[") {
		return nil
	}
	rows, _ := callArgs(strings.TrimSpace(table[0]), 0)
	var out []pathValue
	for _, row := range rows {
		row = strings.TrimSpace(row)
		switch {
		case strings.HasPrefix(row, "["):
			if cells, _ := callArgs(row, 0); position < len(cells) {
				out = append(out, r.eval(file, cells[position], depth)...)
			}
		case position == 0:
			out = append(out, r.eval(file, row, depth)...)
		}
	}
	return out
}

// receiverBefore is the expression a method call at dot is made on.
func receiverBefore(code string, dot int) string {
	start := dot
	for start > 0 && (code[start-1] == ')' || code[start-1] == ']') {
		opener := matchingOpener(code, start-1)
		if opener < 0 {
			return ""
		}
		start = opener
	}
	for start > 0 && (isIdentPart(code[start-1]) || code[start-1] == '.') {
		start--
	}
	return code[start:dot]
}

// matchingOpener walks back from the bracket at close to the one that opens it.
func matchingOpener(code string, close int) int {
	depth := 0
	for i := close; i >= 0; i-- {
		switch code[i] {
		case ')', ']', '}':
			depth++
		case '(', '[', '{':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// openerBefore is the "(" of the call whose arguments hold offset.
func openerBefore(code string, offset int) int {
	depth := 0
	for i := offset - 1; i >= 0; i-- {
		switch code[i] {
		case ')', ']', '}':
			depth++
		case '(', '[', '{':
			if depth == 0 {
				if code[i] == '(' {
					return i
				}
				return -1
			}
			depth--
		}
	}
	return -1
}

// callerValues is what the callers of function pass at position. They are in
// its own file and in each file that imports it.
func (r *tsReader) callerValues(file, function string, position, depth int) []pathValue {
	sites := []struct{ file, callee string }{{file, function}}
	for _, imported := range r.importers[file] {
		switch imported.original {
		case function:
			sites = append(sites, struct{ file, callee string }{imported.file, imported.local})
		case "*":
			sites = append(sites, struct{ file, callee string }{imported.file, imported.local + "." + function})
		}
	}
	var out []pathValue
	for _, site := range sites {
		code := r.code[site.file]
		for _, call := range r.pattern(`(?:^|[^\w$.])`+regexp.QuoteMeta(site.callee)+`\s*\(`).FindAllStringIndex(code, -1) {
			if strings.HasSuffix(strings.TrimRight(code[:call[0]+1], " "), "function") {
				continue
			}
			if args, end := callArgs(code, call[1]-1); end >= 0 && position < len(args) {
				out = append(out, r.eval(site.file, args[position], depth)...)
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
	if !jsIdentifier.MatchString(function) {
		return nil
	}
	code := r.code[file]
	quoted := regexp.QuoteMeta(function)
	body := ""
	if at := r.pattern(`\bfunction\s+` + quoted + `\s*\([^)]*\)[^{]*\{`).FindStringIndex(code); at != nil {
		if _, end := callArgs(code, at[1]-1); end > 0 {
			body = code[at[1]:end]
		}
	} else if at := r.pattern(`(?:const|let|var)\s+` + quoted + `\s*(?::[^=]+)?=\s*(?:async\s*)?(?:\([^()]*\)|[\w$]+)\s*(?::[^=;{}()]+?)?\s*=>\s*`).FindStringIndex(code); at != nil {
		if !strings.HasPrefix(code[at[1]:], "{") {
			return r.eval(file, expressionAt(code, at[1]), depth)
		}
		if _, end := callArgs(code, at[1]); end > 0 {
			body = code[at[1]+1 : end]
		}
	} else if imported, ok := r.importOf(file, function); ok && imported.module != "" {
		return r.returnValues(imported.module, imported.original, depth+1)
	}
	var out []pathValue
	for _, at := range r.pattern(`\breturn\s+`).FindAllStringIndex(body, -1) {
		out = append(out, r.eval(file, expressionAt(body, at[1]), depth+1)...)
	}
	return out
}

func (r *tsReader) module(file, specifier string) (string, bool) {
	if !strings.HasPrefix(specifier, ".") {
		return "", false
	}
	return r.tree.trackedModule(path.Join(path.Dir(file), stripQuery(specifier)))
}

func stripQuery(specifier string) string {
	before, _, _ := strings.Cut(specifier, "?")
	return before
}

// A template literal that cannot be spelled reads as the directory before its
// first substitution.
func jsStringLiteral(expr string) (string, bool) {
	if len(expr) < 2 || !strings.ContainsRune("\"'`", rune(expr[0])) || expr[len(expr)-1] != expr[0] {
		return "", false
	}
	inner := expr[1 : len(expr)-1]
	if strings.ContainsRune(inner, rune(expr[0])) {
		return "", false
	}
	if before, _, templated := strings.Cut(inner, "${"); templated {
		return before[:max(strings.LastIndex(before, "/"), 0)], true
	}
	return inner, true
}

// callArgs returns the top-level items of the bracket opened at open, and the
// offset of the bracket that closes it (-1 when none does).
func callArgs(code string, open int) ([]string, int) {
	var args []string
	depth, start := 0, open+1
	for i := open + 1; i < len(code); i++ {
		switch c := code[i]; c {
		case '"', '\'', '`':
			i = literalEnd(code, i)
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth == 0 {
				if strings.TrimSpace(code[start:i]) != "" {
					args = append(args, code[start:i])
				}
				return args, i
			}
			depth--
		case ',':
			if depth == 0 {
				args = append(args, code[start:i])
				start = i + 1
			}
		}
	}
	return nil, -1
}

func expressionAt(code string, offset int) string {
	depth := 0
	for i := offset; i < len(code); i++ {
		switch c := code[i]; c {
		case '"', '\'', '`':
			i = literalEnd(code, i)
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth == 0 {
				return code[offset:i]
			}
			depth--
		case ';', ',', '\n':
			if depth == 0 {
				return code[offset:i]
			}
		}
	}
	return code[offset:]
}
