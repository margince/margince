// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

import (
	"path"
	"regexp"
	"slices"
	"strings"
)

// jsCall is one call in a source: the dotted name called and the offset of its "(".
type jsCall struct {
	callee      string
	start, open int
}

// jsSites is one pass over a comment-blanked source: every call outside a string,
// and the text of each import statement.
type jsSites struct {
	calls   []jsCall
	imports []string
}

func scanJSSites(code string) jsSites {
	var sites jsSites
	last := byte(0)
	for i := 0; i < len(code); {
		c := code[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
			continue
		case c == '"' || c == '\'' || c == '`':
			i = literalEnd(code, i) + 1
		case c == '/' && (last == 0 || strings.IndexByte("(,=:[!&|?{};+-*%<>~^", last) >= 0):
			i = regexLiteralEnd(code, i) + 1
		case isIdentStart(c) && (i == 0 || !isIdentPart(code[i-1]) && code[i-1] != '.'):
			end := i
			for end < len(code) && (isIdentPart(code[end]) || code[end] == '.') {
				end++
			}
			next := end
			for next < len(code) && strings.ContainsRune(" \t\n", rune(code[next])) {
				next++
			}
			name := code[i:end]
			switch {
			case next < len(code) && code[next] == '(':
				sites.calls = append(sites.calls, jsCall{name, i, next})
			case name == "import" || name == "export":
				sites.imports = append(sites.imports, code[i:min(len(code), i+importWindow)])
			}
			i = end
			last = 'a'
			continue
		default:
			i++
		}
		last = c
	}
	return sites
}

// regexLiteralEnd is the offset of the "/" that closes the regular expression
// opened at start, or of the line's end.
func regexLiteralEnd(code string, start int) int {
	class := false
	for i := start + 1; i < len(code); i++ {
		switch c := code[i]; {
		case c == '\\':
			i++
		case c == '\n':
			return i
		case c == '[':
			class = true
		case c == ']':
			class = false
		case c == '/' && !class:
			return i
		}
	}
	return len(code) - 1
}

// importWindow is longer than any import statement's text up to its specifier.
const importWindow = 2000

var (
	fsReads         = []string{"readFileSync", "readFile", "existsSync", "statSync", "lstatSync", "createReadStream", "readdirSync", "readdir", "opendirSync", "globSync", "globbySync", "glob", "globby", "fastGlob", "fg"}
	moduleCalls     = []string{"import", "require", "mock", "importActual"}
	processCalls    = []string{"execFileSync", "execFile", "spawnSync", "spawn", "execSync", "exec", "execaSync", "execa"}
	moduleSpecifier = regexp.MustCompile(`^(?:(?:import|export)\b[^;"'()]*?\bfrom\s*|import\s*)["'](\.[^"']*)["']`)
	cwdOption       = regexp.MustCompile(`\bcwd\s*:\s*`)
)

// sinkName is the function a call reaches: fs.readFileSync and readFileSync are
// both readFileSync, fg.sync is fg.
func sinkName(callee string) string {
	segments := strings.Split(callee, ".")
	last := segments[len(segments)-1]
	if last == "sync" && len(segments) > 1 {
		return segments[len(segments)-2]
	}
	return last
}

var processModules = []string{"child_process", "node:child_process", "execa"}

// A relative fs path resolves against the process's directory. That is frontend/
// under vitest and pnpm, and the repository root under make.
var defaultCwds = []string{frontendTree, "."}

// scriptReads are the paths a TypeScript, JavaScript, CSS or HTML reader hands to
// an fs call, an import, a glob or a subprocess. "." stands for the whole repository.
func (r *tsReader) scriptReads(file string) []string {
	code := r.code[file]
	reads := map[string]bool{}
	place := func(values []pathValue, anchors []string) {
		for _, value := range values {
			switch {
			case value.anchored && path.Clean(value.path) == ".":
				reads["."] = true
			case value.anchored:
				if read, ok := r.tree.resolveRead(value.path, []string{"."}); ok {
					reads[read] = true
				}
			default:
				if read, ok := r.tree.resolveRead(value.path, anchors); ok {
					reads[read] = true
				}
			}
		}
	}
	here := []string{path.Dir(file)}
	for _, statement := range r.sites[file].imports {
		if match := moduleSpecifier.FindStringSubmatch(statement); match != nil {
			place([]pathValue{{path: stripQuery(match[1])}}, here)
		}
	}
	for _, call := range r.sites[file].calls {
		args, end := callArgs(code, call.open)
		if end < 0 {
			continue
		}
		name := sinkName(call.callee)
		switch {
		case slices.Contains(moduleCalls, name):
			place(r.eval(file, stripQuery(firstArg(args)), 0), here)
		case strings.HasSuffix(call.callee, "meta.glob"):
			place(r.eval(file, firstArg(args), 0), here)
		case slices.Contains(fsReads, name):
			anchors := defaultCwds
			if len(args) > 1 {
				if cwds := r.cwdValues(file, args[1]); len(cwds) > 0 {
					anchors = cwds
				}
			}
			place(r.eval(file, firstArg(args), 0), anchors)
		case slices.Contains(processCalls, name) && r.spawns(file, call.callee):
			r.processReads(file, name, args, place)
		}
	}
	return sortedKeys(reads)
}

// spawns tells a child_process call from a method that shares its name, such as
// RegExp's exec.
func (r *tsReader) spawns(file, callee string) bool {
	receiver, method, dotted := strings.Cut(callee, ".")
	if !dotted {
		original, ok := r.importedFrom(file, callee, processModules)
		return ok && original == callee
	}
	original, ok := r.importedFrom(file, receiver, processModules)
	return ok && (original == "*" || original == "default") && method != ""
}

// cwdValues reads the cwd property of an options object.
func (r *tsReader) cwdValues(file, options string) []string {
	options = strings.TrimSpace(options)
	at := cwdOption.FindStringIndex(options)
	if !strings.HasPrefix(options, "{") || at == nil {
		return nil
	}
	var cwds []string
	for _, value := range r.eval(file, expressionAt(options, at[1]), 0) {
		if value.anchored {
			cwds = append(cwds, path.Clean(value.path))
		}
	}
	return cwds
}

// processReads places what a subprocess reads. A listing reads what it lists;
// any other command reads the directory it runs in and each path it is handed.
func (r *tsReader) processReads(file, callee string, args []string, place func([]pathValue, []string)) {
	var words []string
	options := ""
	if callee == "execSync" || callee == "exec" {
		for _, value := range r.eval(file, firstArg(args), 0) {
			words = strings.Fields(value.path)
		}
		if len(args) > 1 {
			options = args[1]
		}
	} else {
		for _, value := range r.eval(file, firstArg(args), 0) {
			words = []string{value.path}
		}
		rest := args[min(1, len(args)):]
		if len(rest) > 0 && strings.HasPrefix(strings.TrimSpace(rest[0]), "[") {
			elements, _ := callArgs(strings.TrimSpace(rest[0]), 0)
			for _, element := range elements {
				values := r.eval(file, element, 0)
				if len(values) != 1 || values[0].anchored {
					place(values, nil)
					words = append(words, "")
					continue
				}
				words = append(words, values[0].path)
			}
			rest = rest[1:]
		}
		options = firstArg(rest)
	}
	cwds := r.cwdValues(file, options)
	anchors := cwds
	if len(anchors) == 0 {
		anchors = defaultCwds
	}
	listed, listing := listedPaths(words)
	for _, cwd := range cwds {
		if !listing || len(listed) == 0 {
			place([]pathValue{{cwd, true}}, nil)
		}
	}
	for _, word := range listed {
		place([]pathValue{{path: word}}, anchors)
	}
}

// listedPaths is the pathspecs a listing command names; listing is false for any
// other command, whose path-shaped arguments are its reads instead.
func listedPaths(words []string) (paths []string, listing bool) {
	if len(words) == 0 {
		return nil, false
	}
	command, rest := words[0], words[1:]
	switch {
	case command == "git" && len(rest) > 0 && (rest[0] == "ls-files" || rest[0] == "grep"):
		afterDashes := false
		for _, word := range rest[1:] {
			switch {
			case word == "--":
				afterDashes = true
			case afterDashes || rest[0] == "ls-files" && word != "" && !strings.HasPrefix(word, "-"):
				paths = append(paths, word)
			}
		}
		return paths, true
	case command == "find":
		for _, word := range rest {
			if word == "" || strings.HasPrefix(word, "-") || strings.HasPrefix(word, "(") || word == "!" {
				break
			}
			paths = append(paths, word)
		}
		return paths, true
	}
	for _, word := range rest {
		if strings.Contains(word, "/") && !strings.HasPrefix(word, "-") {
			paths = append(paths, word)
		}
	}
	return paths, false
}
