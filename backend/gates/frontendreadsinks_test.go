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
	writeCommands = []string{"mkdir", "rm", "rmdir", "mv", "touch", "ln", "chmod"}
	// copyCommands read each path they are handed but the destination.
	copyCommands = []string{"cp", "install"}
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
			place(withoutQueries(r.eval(file, firstArg(args), 0)), here)
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

func withoutQueries(values []pathValue) []pathValue {
	out := make([]pathValue, len(values))
	for i, value := range values {
		out[i] = pathValue{stripQuery(value.path), value.anchored}
	}
	return out
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
	commands, tail, options := r.processCommands(file, callee, args)
	cwds := r.cwdValues(file, options)
	anchors := cwds
	if len(anchors) == 0 {
		anchors = defaultCwds
	}
	if len(commands) == 0 {
		// A command the census cannot spell still runs in its directory on its arguments.
		for _, arg := range tail {
			place(arg, nil)
		}
		for _, cwd := range cwds {
			place([]pathValue{{cwd, true}}, nil)
		}
	}
	for _, command := range commands {
		words := make([]string, len(command))
		for i, arg := range command {
			if len(arg) == 1 && (i == 0 || !arg[0].anchored) {
				words[i] = arg[0].path
			}
		}
		if writesOnly(words) {
			continue
		}
		if command[0][0].anchored {
			place(command[0], nil)
		}
		if slices.Contains(copyCommands, words[0]) && len(words) > 2 {
			drop := copyDestination(words)
			command, words = withoutIndices(command, drop), withoutIndices(words, drop)
		}
		for i, arg := range command[1:] {
			if words[i+1] == "" {
				place(arg, nil)
			}
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
}

// processCommands is each command a call can run, one per value its command takes.
// A loop may hand one call several.
func (r *tsReader) processCommands(file, callee string, args []string) (commands [][][]pathValue, tail [][]pathValue, options string) {
	rest := args[min(1, len(args)):]
	if callee == "execSync" || callee == "exec" {
		for _, value := range r.eval(file, firstArg(args), 0) {
			var command [][]pathValue
			for _, word := range strings.Fields(value.path) {
				command = append(command, []pathValue{{path: word}})
			}
			if len(command) > 0 {
				commands = append(commands, command)
			}
		}
		return commands, nil, firstArg(rest)
	}
	if len(rest) > 0 && strings.HasPrefix(strings.TrimSpace(rest[0]), "[") {
		elements, _ := callArgs(strings.TrimSpace(rest[0]), 0)
		for _, element := range elements {
			tail = append(tail, r.eval(file, element, 0))
		}
		rest = rest[1:]
	}
	for _, value := range r.eval(file, firstArg(args), 0) {
		commands = append(commands, append([][]pathValue{{value}}, tail...))
	}
	return commands, tail, firstArg(rest)
}

// A cluster ending in t takes the next word as its directory. A t with more after
// it holds the directory itself, as getopt reads -tdir.
var (
	targetOption   = regexp.MustCompile(`^(?:-[A-Za-z]*t|--target-directory)$`)
	attachedTarget = regexp.MustCompile(`^(?:-[A-Za-z]*t.|--target-directory=)`)
)

// copyDestination is the index of each word a copy writes: the directory -t names,
// or else its last word.
func copyDestination(words []string) []int {
	for i, word := range words {
		switch {
		case targetOption.MatchString(word) && i+1 < len(words):
			return []int{i, i + 1}
		case attachedTarget.MatchString(word):
			return []int{i}
		}
	}
	return []int{len(words) - 1}
}

// writesOnly reports a command that makes or removes paths, as install -d makes
// directories.
func writesOnly(words []string) bool {
	return slices.Contains(writeCommands, words[0]) ||
		words[0] == "install" && (slices.Contains(words, "-d") || slices.Contains(words, "--directory"))
}

func withoutIndices[T any](items []T, drop []int) []T {
	var kept []T
	for i, item := range items {
		if !slices.Contains(drop, i) {
			kept = append(kept, item)
		}
	}
	return kept
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
