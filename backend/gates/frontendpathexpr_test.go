// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

import (
	"fmt"
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
	files     map[string]*jsFile
	imports   map[string][]jsImport
	importers map[string][]jsImport
	names     map[string][]pathValue
	working   map[string]bool
	// cut counts evaluations a cycle or the depth bound stopped short. A name worked
	// out across one is not kept, so no result depends on the order of files.
	cut int
	// spawned counts the reads subprocess calls make, for the subprocess floor.
	spawned int
}

var (
	jsIdentifier = regexp.MustCompile(`^[A-Za-z_$][\w$]*$`)
	jsMember     = regexp.MustCompile(`^([A-Za-z_$][\w$]*)\.([A-Za-z_$][\w$]*)$`)
	jsCallee     = regexp.MustCompile(`^(?:new\s+)?([\w$.]+)\s*\(`)
	arrayFill    = regexp.MustCompile(`^(?:new\s+)?Array\(\s*(\d+)\s*\)\.fill\(([\s\S]+)\)$`)
)

var pathModules = []string{"path", "node:path", "path/posix", "node:path/posix", "url", "node:url"}

func newTSReader(tree repoTree, sources map[string]string) (*tsReader, error) {
	r := &tsReader{tree: tree, files: map[string]*jsFile{}, imports: map[string][]jsImport{},
		importers: map[string][]jsImport{}, names: map[string][]pathValue{}, working: map[string]bool{}}
	var failed []string
	for _, file := range sortedKeys(sources) {
		if err := r.add(file, sources[file]); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", file, err))
		}
	}
	if len(failed) > 0 {
		return r, fmt.Errorf("the census cannot read %d source(s), so it cannot see their reads:\n\t%s", len(failed), strings.Join(failed, "\n\t"))
	}
	return r, nil
}

// add keeps each name already worked out as it was.
func (r *tsReader) add(file, source string) error {
	parsed, err := parseJSFile(file, source)
	if err != nil {
		return err
	}
	r.files[file] = parsed
	for _, imported := range tsImports(parsed.code) {
		r.addImport(file, imported.local, imported.original, imported.from)
	}
	for _, call := range parsed.calls {
		if call.callee != "require" {
			continue
		}
		if local, ok := parsed.declaredBefore(call.at); ok {
			if specifier, ok := jsStringLiteral(strings.TrimSpace(parsed.arg(call.at, 0))); ok {
				r.addImport(file, local, "*", specifier)
			}
		}
	}
	return nil
}

func (r *tsReader) addImport(file, local, original, specifier string) {
	module, _ := r.module(file, specifier)
	imported := jsImport{file, local, original, specifier, module}
	r.imports[file] = append(r.imports[file], imported)
	if module != "" {
		r.importers[module] = append(r.importers[module], imported)
	}
}

func (r *tsReader) importOf(file, local string) (jsImport, bool) {
	for _, imported := range r.imports[file] {
		if imported.local == local {
			return imported, true
		}
	}
	return jsImport{}, false
}

// importedFrom matches a namespace or default binding as well as a named export.
func (r *tsReader) importedFrom(file, local string, specifiers []string) (original string, ok bool) {
	imported, found := r.importOf(file, local)
	if !found || !slices.Contains(specifiers, imported.specifier) {
		return "", false
	}
	return imported.original, true
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
	if operands := tsSplitTopLevel(expr, '+'); len(operands) > 1 {
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

// spell crosses each value with each piece, as a template literal or a + chain does.
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

// A substitution that cannot be evaluated leaves the template with no value.
func (r *tsReader) evalTemplate(file, inner string, depth int) []pathValue {
	values := []pathValue{{}}
	for inner != "" {
		literal, rest, substituted := strings.Cut(inner, "${")
		values = spell(values, []pathValue{{path: literal}})
		if !substituted {
			break
		}
		substitution, ok := tsBalanced("{" + rest)
		if !ok {
			return nil
		}
		parts := r.eval(file, substitution, depth)
		if len(parts) == 0 {
			return nil
		}
		values = spell(values, parts)
		inner = rest[len(substitution)+1:]
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
		if initialiser, ok := r.files[file].declaration(expr); ok {
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

// callArgs answers -1 for the end when no bracket closes the one at open.
func callArgs(code string, open int) ([]string, int) {
	inner, ok := tsBalanced(code[open:])
	if !ok {
		return nil, -1
	}
	var args []string
	for _, item := range tsSplitTopLevel(inner, ',') {
		if strings.TrimSpace(item) != "" {
			args = append(args, item)
		}
	}
	return args, open + len(inner) + 1
}
