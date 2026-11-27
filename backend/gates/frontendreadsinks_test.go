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

var (
	fsReads      = []string{"readFileSync", "readFile", "existsSync", "statSync", "lstatSync", "createReadStream", "readdirSync", "readdir", "opendirSync", "globSync", "globbySync", "glob", "globby", "fastGlob", "fg"}
	moduleCalls  = []string{"import", "require", "mock", "importActual"}
	processCalls = []string{"execFileSync", "execFile", "spawnSync", "spawn", "execSync", "exec", "execaSync", "execa"}
	// writeCommands make or remove the paths they are handed; they read none.
	writeCommands = []string{"mkdir", "rm", "rmdir", "mv", "cp", "touch", "ln", "chmod"}
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

// scriptReads are the paths a TypeScript or JavaScript reader hands to an fs
// call, an import, a glob or a subprocess. "." stands for the whole repository.
func (r *tsReader) scriptReads(file string) []string {
	parsed := r.files[file]
	if parsed == nil {
		return nil
	}
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
	for _, specifier := range parsed.specifiers() {
		place([]pathValue{{path: stripQuery(specifier)}}, here)
	}
	for _, call := range parsed.calls {
		args := parsed.args(call.at)
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
			before := len(reads)
			r.processReads(file, name, args, place)
			r.spawned += len(reads) - before
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

func (r *tsReader) cwdValues(file, options string) []string {
	body, ok := tsBalanced(strings.TrimSpace(options))
	if !ok || !strings.HasPrefix(strings.TrimSpace(options), "{") {
		return nil
	}
	var cwds []string
	for _, value := range r.eval(file, tsLiteralEntries(body)["cwd"], 0) {
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
	var anchored []pathValue
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
					anchored = append(anchored, values...)
					words = append(words, "")
					continue
				}
				words = append(words, values[0].path)
			}
			rest = rest[1:]
		}
		options = firstArg(rest)
	}
	if len(words) > 0 && slices.Contains(writeCommands, words[0]) {
		return
	}
	place(anchored, nil)
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
	for i, word := range rest {
		if strings.Contains(word, "/") && !strings.HasPrefix(word, "-") && (i == 0 || rest[i-1] != "-o") {
			paths = append(paths, word)
		}
	}
	return paths, false
}

var markupPath = regexp.MustCompile(`(?:@import\s+(?:url\()?|url\(|\b(?:src|href)\s*=\s*)\s*["']?([^"')\s;>]+)`)

// markupReads are the escapes a stylesheet, page or MDX file names in @import,
// url(), src, href or an import, relative to its own directory.
func markupReads(tree repoTree, file, source string) []string {
	var rels []string
	for _, match := range markupPath.FindAllStringSubmatch(source, -1) {
		rels = append(rels, match[1])
	}
	if path.Ext(file) == ".mdx" {
		for _, imported := range tsImports(source) {
			rels = append(rels, imported.from)
		}
	}
	var found []string
	for _, rel := range rels {
		if !strings.HasPrefix(rel, ".") {
			continue
		}
		if read, ok := tree.resolveRead(stripQuery(rel), []string{path.Dir(file)}); ok {
			found = append(found, read)
		}
	}
	return found
}
