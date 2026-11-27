// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// The frontend lane runs on a change to every path outside frontend/ it reads.
//
// The readers are every tracked file under frontend/, the root scripts the
// frontend-scoped jobs run, and the Makefile recipes those jobs reach. Not seen:
// a path built from a function's result, process.cwd() or an environment
// variable, and a script's reads through a helper it sources.

import (
	"maps"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const frontendTree = "frontend"

// maxPathValues bounds the values one path expression can take, so a join over
// several lists cannot grow without end.
const maxPathValues = 64

type laneRead struct {
	reader, path string
}

type repoTree struct {
	files, dirs map[string]bool
}

func newRepoTree(files []string) repoTree {
	tree := repoTree{files: map[string]bool{}, dirs: map[string]bool{}}
	for _, file := range files {
		tree.files[file] = true
		for dir := path.Dir(file); dir != "."; dir = path.Dir(dir) {
			tree.dirs[dir] = true
		}
	}
	return tree
}

// moduleSuffixes are the endings a module specifier written without one resolves through.
var moduleSuffixes = []string{"", ".ts", ".tsx", ".mts", ".js", ".mjs", ".json", "/index.ts"}

func (tree repoTree) tracked(p string) (string, bool) {
	if tree.dirs[p] {
		return p, true
	}
	for _, suffix := range moduleSuffixes {
		if tree.files[p+suffix] {
			return p + suffix, true
		}
	}
	return "", false
}

func insideFrontend(p string) bool {
	return p == frontendTree || strings.HasPrefix(p, frontendTree+"/")
}

// A glob reads as the directory it walks.
func staticPrefix(p string) string {
	segments := strings.Split(p, "/")
	for i, segment := range segments {
		if strings.ContainsAny(segment, "*?{[") {
			return strings.Join(segments[:i], "/")
		}
	}
	return p
}

// A reading inside frontend/ wins, because the lane's own tree already triggers it.
func (tree repoTree) resolveRead(rel string, anchors []string) (string, bool) {
	rel = staticPrefix(rel)
	outside := ""
	for _, anchor := range anchors {
		joined := path.Join(anchor, rel)
		if joined == "." || joined == ".git" || joined == ".." || strings.HasPrefix(joined, "../") || strings.HasPrefix(joined, ".git/") {
			continue
		}
		found, ok := tree.tracked(joined)
		switch {
		case !ok:
		case insideFrontend(found):
			return "", false
		case outside == "":
			outside = found
		}
	}
	return outside, outside != ""
}

// settle drops a path the reader only writes, and a directory it never walks but
// only builds the paths it names through.
func (tree repoTree) settle(reads, opened, walked, written map[string]bool) []string {
	var kept []string
	for read := range reads {
		if walked[read] || opened[read] {
			kept = append(kept, read)
			continue
		}
		if written[read] || tree.dirs[read] && (holdsAny(read, reads) || holdsAny(read, written)) {
			continue
		}
		kept = append(kept, read)
	}
	return kept
}

func holdsAny(dir string, paths map[string]bool) bool {
	for p := range paths {
		if strings.HasPrefix(p, dir+"/") {
			return true
		}
	}
	return false
}

func ancestors(reader string) []string {
	var dirs []string
	for dir := path.Dir(reader); ; dir = path.Dir(dir) {
		dirs = append(dirs, dir)
		if dir == "." {
			return dirs
		}
	}
}

var (
	pathCall        = regexp.MustCompile(`(?:^|[^\w.$])((?:path\.|posix\.)?(?:join|resolve)|new\s+URL)\s*\(`)
	walkCall        = regexp.MustCompile(`\b(?:readdirSync|readdir|opendirSync|globSync|glob)\s*\(`)
	readCall        = regexp.MustCompile(`\b(?:readFileSync|readFile|existsSync|statSync|createReadStream)\s*\(`)
	writeCall       = regexp.MustCompile(`\b(?:writeFileSync|writeFile|appendFileSync|appendFile|mkdirSync|createWriteStream|rmSync)\s*\(`)
	moduleSpecifier = regexp.MustCompile(`\b(?:from|import|require|mock|importActual)\s*\(?\s*["'](\.[^"']*)["']`)
	escapeLiteral   = regexp.MustCompile("[\"'`]((?:\\.\\./)+[^\"'`$\\s]*)")
	jsonEscape      = regexp.MustCompile(`(?:^|[\s"'=:(])((?:\.\./)+[\w@.*/-]*)`)
	shellVarPath    = regexp.MustCompile(`\$\{?\w+\}?((?:/[\w.*-]+)+)`)
	commandPath     = regexp.MustCompile(`(?:^|[\s"'=(])((?:\.\./)*\w[\w.*-]*(?:/[\w.*-]+)+)`)
	jsIdentifier    = regexp.MustCompile(`^[A-Za-z_$][\w$]*$`)
	jsCallee        = regexp.MustCompile(`^(?:new\s+)?([\w.]+)\s*\(`)
	jsFunction      = regexp.MustCompile(`\bfunction\s+([\w$]+)\s*\(([^)]*)\)`)
	namedImport     = regexp.MustCompile(`import\s*(?:type\s+)?\{([^}]*)\}\s*from\s*["']([^"']+)["']`)
)

var patternWords = func() map[*regexp.Regexp][]string {
	words := map[*regexp.Regexp][]string{}
	for _, pattern := range []*regexp.Regexp{pathCall, readCall, walkCall, writeCall, moduleSpecifier, escapeLiteral} {
		words[pattern] = requiredWords(pattern)
	}
	return words
}()

// Every match holds one of the pattern's required words, so a file with none skips it.
func mayHold(lower string, pattern *regexp.Regexp) bool {
	words, ok := patternWords[pattern]
	if !ok {
		words = requiredWords(pattern)
	}
	return words == nil || slices.ContainsFunc(words, func(word string) bool { return strings.Contains(lower, word) })
}

// pathValue is one value of a path expression: a repository path when anchored,
// otherwise a relative path whose directory is not known.
type pathValue struct {
	path     string
	anchored bool
}

type tsReader struct {
	tree     repoTree
	code     map[string]string
	patterns map[string]*regexp.Regexp
	names    map[string][]pathValue
}

func newTSReader(tree repoTree, sources map[string]string) *tsReader {
	reader := &tsReader{tree: tree, code: map[string]string{}, patterns: map[string]*regexp.Regexp{}, names: map[string][]pathValue{}}
	for file, source := range sources {
		code, _ := stripTSComments(source)
		reader.code[file] = code
	}
	return reader
}

func (r *tsReader) pattern(expr string) *regexp.Regexp {
	compiled, ok := r.patterns[expr]
	if !ok {
		compiled = regexp.MustCompile(expr)
		r.patterns[expr] = compiled
	}
	return compiled
}

func (r *tsReader) eval(file, expr string, depth int) []pathValue {
	expr = strings.TrimSpace(expr)
	expr = strings.TrimSuffix(strings.TrimSuffix(expr, ".pathname"), ".href")
	if depth > 8 || expr == "" {
		return nil
	}
	switch expr {
	case "import.meta.dirname", "__dirname":
		return []pathValue{{path.Dir(file), true}}
	case "import.meta.url", "import.meta.filename", "__filename":
		return []pathValue{{file, true}}
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
	if jsIdentifier.MatchString(expr) {
		return r.evalName(file, expr, depth+1)
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

// evalTemplate spells a template literal once for each value its substitutions
// can take, or not at all when one of them cannot be evaluated.
func (r *tsReader) evalTemplate(file, inner string, depth int) []pathValue {
	values := []pathValue{{}}
	for inner != "" {
		literal, rest, substituted := strings.Cut(inner, "${")
		for i := range values {
			values[i].path += literal
		}
		if !substituted {
			break
		}
		_, end := callArgs(rest, -1)
		parts := r.eval(file, rest[:max(end, 0)], depth)
		if end < 0 || len(parts) == 0 {
			return nil
		}
		var next []pathValue
		for _, value := range values {
			for _, part := range parts {
				if value.path == "" && part.anchored {
					next = append(next, part)
				} else {
					next = append(next, pathValue{value.path + part.path, value.anchored})
				}
			}
		}
		values = next[:min(len(next), maxPathValues)]
		inner = rest[end+1:]
	}
	return values
}

func (r *tsReader) evalCall(file, name string, args []string, depth int) []pathValue {
	var out []pathValue
	switch name {
	case "join", "resolve", "path.join", "path.resolve", "posix.join":
		return r.evalJoin(file, args, depth)
	case "fileURLToPath", "url.fileURLToPath", "normalize", "path.normalize":
		return r.eval(file, firstArg(args), depth)
	case "dirname", "path.dirname":
		for _, value := range r.eval(file, firstArg(args), depth) {
			if value.anchored {
				out = append(out, pathValue{path.Dir(value.path), true})
			}
		}
	case "URL":
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
	}
	return out
}

// evalJoin folds join and resolve over every value each argument can take. An
// argument it cannot evaluate ends the path, or leaves it unanchored when first.
func (r *tsReader) evalJoin(file string, args []string, depth int) []pathValue {
	values := []pathValue{{}}
	for i, arg := range args {
		parts := r.eval(file, arg, depth)
		if len(parts) == 0 && i == 0 {
			continue
		}
		if len(parts) == 0 {
			break
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
	var out []pathValue
	for _, value := range values {
		if value.anchored || value.path != "" && value.path != "." {
			out = append(out, value)
		}
	}
	return out
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// evalName follows a name to what binds it. That is a declaration, a for-of over
// a literal list, the callers of the function it is a parameter of, or an import.
func (r *tsReader) evalName(file, name string, depth int) []pathValue {
	key := file + "\x00" + name
	if values, seen := r.names[key]; seen {
		return values
	}
	// Held empty while it is worked out, so a name bound through itself ends.
	r.names[key] = nil
	values := r.bindings(file, name, depth)
	r.names[key] = values
	return values
}

func (r *tsReader) bindings(file, name string, depth int) []pathValue {
	code := r.code[file]
	quoted := regexp.QuoteMeta(name)
	if at := r.pattern(`(?:^|[^\w$.])(?:const|let|var)\s+` + quoted + `\s*(?::[^=;]+)?=\s*`).FindStringIndex(code); at != nil {
		return r.eval(file, expressionAt(code, at[1]), depth)
	}
	if at := r.pattern(`for\s*\(\s*(?:const|let|var)\s+` + quoted + `\s+of\s*\[`).FindStringIndex(code); at != nil {
		elements, _ := callArgs(code, at[1]-1)
		var out []pathValue
		for _, element := range elements {
			out = append(out, r.eval(file, element, depth)...)
		}
		return out
	}
	if values := r.callSiteValues(file, name, depth); len(values) > 0 {
		return values
	}
	return r.importedValues(file, name, depth)
}

func (r *tsReader) callSiteValues(file, name string, depth int) []pathValue {
	code := r.code[file]
	var out []pathValue
	for _, declared := range jsFunction.FindAllStringSubmatchIndex(code, -1) {
		function := code[declared[2]:declared[3]]
		position := slices.IndexFunc(strings.Split(code[declared[4]:declared[5]], ","), func(param string) bool {
			param, _, _ = strings.Cut(param, "=")
			param, _, _ = strings.Cut(param, ":")
			return strings.TrimSuffix(strings.TrimSpace(param), "?") == name
		})
		if position < 0 {
			continue
		}
		for _, call := range r.pattern(`(?:^|[^\w$.])`+regexp.QuoteMeta(function)+`\s*\(`).FindAllStringIndex(code, -1) {
			if call[0] <= declared[2] && declared[2] < call[1] {
				continue
			}
			if args, end := callArgs(code, call[1]-1); end >= 0 && position < len(args) {
				out = append(out, r.eval(file, args[position], depth)...)
			}
		}
	}
	return out
}

func (r *tsReader) importedValues(file, name string, depth int) []pathValue {
	for _, imported := range namedImport.FindAllStringSubmatch(r.code[file], -1) {
		for binding := range strings.SplitSeq(imported[1], ",") {
			original, local, renamed := strings.Cut(strings.TrimPrefix(strings.TrimSpace(binding), "type "), " as ")
			if !renamed {
				local = original
			}
			if strings.TrimSpace(local) != name {
				continue
			}
			if module, found := r.module(file, imported[2]); found {
				return r.evalName(module, strings.TrimSpace(original), depth+1)
			}
			return nil
		}
	}
	return nil
}

func (r *tsReader) module(file, specifier string) (string, bool) {
	if !strings.HasPrefix(specifier, ".") {
		return "", false
	}
	base := path.Join(path.Dir(file), specifier)
	for _, candidate := range []string{base, strings.TrimSuffix(base, ".js") + ".ts", base + ".ts", base + ".tsx", base + ".mts", base + ".js", base + ".mjs", base + "/index.ts"} {
		if _, ok := r.code[candidate]; ok {
			return candidate, true
		}
	}
	return "", false
}

// A template literal reads as the directory before its first substitution.
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

// It splits a "[" list the same way, for a for-of over literal names.
func callArgs(code string, open int) ([]string, int) {
	var args []string
	depth, start := 0, open+1
	for i := open + 1; i < len(code); i++ {
		switch c := code[i]; c {
		case '"', '\'', '`':
			if i = literalEnd(code, i); i < 0 {
				return nil, -1
			}
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
			if i = literalEnd(code, i); i < 0 {
				return code[offset:]
			}
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ';', ',', '\n':
			if depth <= 0 {
				return code[offset:i]
			}
		}
	}
	return code[offset:]
}

func (r *tsReader) scriptReads(file string) []string {
	code := r.code[file]
	lower := strings.ToLower(code)
	scan := func(pattern *regexp.Regexp) [][]int {
		if !mayHold(lower, pattern) {
			return nil
		}
		return pattern.FindAllStringSubmatchIndex(code, -1)
	}
	anchors := ancestors(file)
	reads, opened, walked, written := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	place := func(into map[string]bool, rel string, anchors []string) {
		if read, ok := r.tree.resolveRead(rel, anchors); ok {
			into[read] = true
			if strings.ContainsAny(rel, "*?") {
				walked[read] = true
			}
		}
	}
	placeValues := func(into map[string]bool, values []pathValue) {
		for _, value := range values {
			if value.anchored {
				place(into, value.path, []string{"."})
			} else {
				place(into, value.path, anchors)
			}
		}
	}
	for _, at := range scan(pathCall) {
		if _, end := callArgs(code, at[1]-1); end >= 0 {
			placeValues(reads, r.eval(file, code[at[2]:end+1], 0))
		}
	}
	for sink, into := range map[*regexp.Regexp]map[string]bool{readCall: opened, walkCall: walked, writeCall: written} {
		for _, at := range scan(sink) {
			if args, end := callArgs(code, at[1]-1); end >= 0 {
				placeValues(into, r.eval(file, firstArg(args), 0))
			}
		}
	}
	for _, at := range scan(moduleSpecifier) {
		place(reads, code[at[2]:at[3]], []string{path.Dir(file)})
	}
	for _, at := range scan(escapeLiteral) {
		place(reads, code[at[2]:at[3]], anchors)
	}
	maps.Copy(reads, opened)
	maps.Copy(reads, walked)
	return r.tree.settle(reads, opened, walked, written)
}

// A config's escapes are relative to its own directory.
func (r *tsReader) configReads(file string) []string {
	var found []string
	for _, match := range jsonEscape.FindAllStringSubmatch(r.code[file], -1) {
		if read, ok := r.tree.resolveRead(match[1], []string{path.Dir(file)}); ok {
			found = append(found, read)
		}
	}
	return found
}

func shellReads(tree repoTree, file, source string) []string {
	anchors := ancestors(file)
	var found []string
	for line := range strings.SplitSeq(source, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		var rels []string
		for _, match := range shellVarPath.FindAllStringSubmatch(line, -1) {
			rels = append(rels, strings.TrimPrefix(match[1], "/"))
		}
		for _, match := range commandPath.FindAllStringSubmatch(line, -1) {
			rels = append(rels, match[1])
		}
		for _, rel := range rels {
			if read, ok := tree.resolveRead(rel, anchors); ok {
				found = append(found, read)
			}
		}
	}
	return found
}

type readerKind int

const (
	notAReader readerKind = iota
	scriptReader
	configReader
	shellReader
)

var readerKinds = map[string]readerKind{
	".ts": scriptReader, ".tsx": scriptReader, ".mts": scriptReader, ".cts": scriptReader,
	".js": scriptReader, ".jsx": scriptReader, ".mjs": scriptReader, ".cjs": scriptReader,
	".css": scriptReader, ".html": scriptReader, ".mdx": scriptReader,
	".json": configReader,
	".sh":   shellReader,
}

func readsOf(ts *tsReader, file, source string) []string {
	switch readerKinds[path.Ext(file)] {
	case scriptReader:
		return ts.scriptReads(file)
	case configReader:
		return ts.configReads(file)
	case shellReader:
		return shellReads(ts.tree, file, source)
	}
	return nil
}

// Each path is relative to the directory the line's last cd left the shell in.
func recipeReads(tree repoTree, line string) []string {
	var found []string
	cwd := "."
	for _, command := range simpleCommands(strings.TrimLeft(line, " \t@+-")) {
		words := strings.Fields(command)
		if len(words) == 2 && words[0] == "cd" {
			cwd = path.Join(cwd, words[1])
			continue
		}
		for _, match := range commandPath.FindAllStringSubmatch(command, -1) {
			if read, ok := tree.resolveRead(match[1], []string{cwd}); ok {
				found = append(found, read)
			}
		}
	}
	return found
}

func frontendLaneRecipes(t *testing.T, workflow workflowFile, lanes []filteredLane) map[string][]string {
	t.Helper()
	targets, vars := parseRecipes(t, rootMakefile)
	recipes := map[string][]string{}
	var walk func(string)
	walk = func(name string) {
		target := targets[name]
		if _, seen := recipes[name]; seen || target == nil {
			return
		}
		recipes[name] = nil
		next := slices.Clone(target.pulls)
		for _, recipe := range target.recipes {
			command := expandRefs(recipe, vars)
			recipes[name] = append(recipes[name], command)
			next = append(next, delegatedTargets(command)...)
		}
		for _, pulled := range next {
			walk(pulled)
		}
	}
	for _, lane := range lanes {
		for _, command := range jobCommands(t, workflow, workflow.Jobs[lane.name].Uses, lane.name) {
			for _, invocation := range makeInvocation.FindAllStringSubmatch(command, -1) {
				walk(invocation[1])
			}
		}
	}
	return recipes
}

func frontendReaderSources(t *testing.T, tree repoTree, lanes []filteredLane) map[string]string {
	t.Helper()
	sources := map[string]string{}
	read := func(file string) {
		body, err := gatekit.ReadFile(filepath.Join(repoRoot, file))
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}
		sources[file] = string(body)
	}
	for file := range tree.files {
		if insideFrontend(file) && readerKinds[path.Ext(file)] != notAReader {
			read(file)
		}
	}
	for _, lane := range lanes {
		for _, input := range lane.inputs {
			if tree.files[input] {
				read(input)
			}
		}
	}
	return sources
}

func frontendLaneReads(t *testing.T, workflow workflowFile) []laneRead {
	t.Helper()
	var paths []string
	for _, file := range trackedFiles(t) {
		if !file.symlink {
			paths = append(paths, file.path)
		}
	}
	tree := newRepoTree(paths)
	var lanes []filteredLane
	for _, lane := range filteredLanes(t, workflow) {
		if slices.Contains(lane.scopes, frontendTree) {
			lanes = append(lanes, lane)
		}
	}
	sources := frontendReaderSources(t, tree, lanes)
	ts := newTSReader(tree, sources)
	var reads []laneRead
	for file, source := range sources {
		for _, read := range readsOf(ts, file, source) {
			reads = append(reads, laneRead{file, read})
		}
	}
	for target, lines := range frontendLaneRecipes(t, workflow, lanes) {
		for _, line := range lines {
			for _, read := range recipeReads(tree, line) {
				reads = append(reads, laneRead{"make " + target, read})
			}
		}
	}
	slices.SortFunc(reads, func(a, b laneRead) int {
		return strings.Compare(a.reader+"\x00"+a.path, b.reader+"\x00"+b.path)
	})
	return slices.Compact(reads)
}

func uncoveredReads(reads []laneRead, scope []string) []laneRead {
	var missing []laneRead
	for _, read := range reads {
		if !covered(scope, read.path) {
			missing = append(missing, read)
		}
	}
	return missing
}

func TestTheFrontendLaneRunsOnAChangeToEveryPathItReads(t *testing.T) {
	t.Parallel()
	workflow := readWorkflow(t, classifierWorkflow)
	scope := filterScopes(t, workflow)[frontendTree]
	if len(scope) == 0 {
		t.Fatalf("%s declares no %q paths-filter scope; this census has nothing to judge against", classifierWorkflow, frontendTree)
	}
	reads := frontendLaneReads(t, workflow)
	if len(reads) < 50 {
		t.Fatalf("the census found %d outside read(s), and the frontend lane makes more than 50: it has stopped recognising them", len(reads))
	}
	for _, read := range uncoveredReads(reads, scope) {
		t.Errorf("%s reads %s, which the %q filter in %s does not cover.\n"+
			"A pull request that changes only that path skips the frontend lane, so the reader can break and merge green.\n"+
			"Add the path, or the tree it sits in, to the %q scope.", read.reader, read.path, frontendTree, classifierWorkflow, frontendTree)
	}
}

func plantedTree() repoTree {
	return newRepoTree([]string{
		"docs/reference/copy.md",
		"docs/handbook/a.md",
		"backend/internal/a/cases.json",
		"backend/internal/a/one.go",
		"backend/internal/a/two.go",
		"scripts/check.sh",
		"frontend/census/tree.ts",
		"frontend/src/x.ts",
	})
}

// Each read form the census claims to see, planted in a reader of its own. The
// last three are no read: a comment, a write, and a path inside frontend/.
func TestTheFrontendReadCensusSeesEachReadForm(t *testing.T) {
	t.Parallel()
	const tree = `export const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
export const repoRoot = join(frontendRoot, "..");`
	cases := []struct {
		name, reader, source string
		want                 []string
	}{
		{"join on a root declared in the file", "frontend/scripts/r.ts",
			`const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
readFileSync(join(repoRoot, "docs", "reference", "copy.md"));`, []string{"docs/reference/copy.md"}},
		{"resolve from the file's own directory", "frontend/src/i18n/r.test.ts",
			`const here = dirname(fileURLToPath(import.meta.url));
readFileSync(resolve(here, "..", "..", "..", "docs", "reference", "copy.md"));`, []string{"docs/reference/copy.md"}},
		{"a root imported from another module", "frontend/census/rules/r.census.ts",
			`import { repoRoot } from "../tree";
readFileSync(join(repoRoot, "docs/reference/copy.md"));`, []string{"docs/reference/copy.md"}},
		{"import.meta.dirname", "frontend/src/a/r.test.ts",
			`readFileSync(join(import.meta.dirname, "../../../docs/reference/copy.md"));`, []string{"docs/reference/copy.md"}},
		{"a module specifier that escapes frontend/", "frontend/src/a/r.test.ts",
			`import cases from "../../../backend/internal/a/cases.json";`, []string{"backend/internal/a/cases.json"}},
		{"new URL on import.meta.url", "frontend/src/a/r.test.ts",
			`readFileSync(new URL("../../../docs/reference/copy.md", import.meta.url));`, []string{"docs/reference/copy.md"}},
		{"a template filled from the call sites", "frontend/src/a/r.test.ts",
			"function source(name: string) {\n  return readFileSync(new URL(`../../../backend/internal/a/${name}`, import.meta.url));\n}\nsource(\"one.go\");\nsource(\"two.go\");",
			[]string{"backend/internal/a/one.go", "backend/internal/a/two.go"}},
		{"a for-of over a list of names", "frontend/src/a/r.test.ts",
			`const dir = join(import.meta.dirname, "../../../backend/internal/a");
for (const file of ["one.go", "two.go"]) readFileSync(join(dir, file));`,
			[]string{"backend/internal/a/one.go", "backend/internal/a/two.go"}},
		{"a directory it walks", "frontend/src/a/r.test.ts",
			`const handbook = join(import.meta.dirname, "../../../docs/handbook");
readdirSync(handbook);`, []string{"docs/handbook"}},
		{"an fs call on a path relative to frontend/", "frontend/scripts/r.ts",
			`readFileSync("../docs/reference/copy.md");`, []string{"docs/reference/copy.md"}},
		{"a glob", "frontend/src/a/r.ts",
			`import.meta.glob("../../../docs/reference/*.md");`, []string{"docs/reference"}},
		{"a path it reads and writes", "frontend/scripts/r.mjs",
			`const out = join(import.meta.dirname, "../../docs/reference/copy.md");
if (check) readFile(out); else writeFile(out, text);`, []string{"docs/reference/copy.md"}},
		{"a JSON config escape", "frontend/package.json",
			`{"scripts": {"gen": "tool ../docs/reference/copy.md -o src/x.ts"}}`, []string{"docs/reference/copy.md"}},
		{"a shell path after a variable", "frontend/scripts/r.sh",
			`cat "$ROOT/docs/reference/copy.md"`, []string{"docs/reference/copy.md"}},
		{"a comment", "frontend/src/a/r.ts",
			`// readFileSync(join(repoRoot, "docs/reference/copy.md"))`, nil},
		{"a path it only writes", "frontend/e2e/r.spec.ts",
			`const dir = path.join("..", "docs", "reference");
fs.mkdirSync(dir);
fs.writeFileSync(path.join(dir, "copy.md"), text);`, nil},
		{"a path inside frontend/", "frontend/src/a/r.ts",
			`readFileSync(join(import.meta.dirname, "../x.ts"));`, nil},
	}
	for _, c := range cases {
		ts := newTSReader(plantedTree(), map[string]string{c.reader: c.source, "frontend/census/tree.ts": tree})
		got := readsOf(ts, c.reader, c.source)
		slices.Sort(got)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: the census read %v from %s, want %v", c.name, got, c.reader, c.want)
		}
	}
}

func TestARecipeReadIsPlacedOnTheDirectoryItsCdLeft(t *testing.T) {
	t.Parallel()
	for line, want := range map[string][]string{
		"\tcd frontend && node scripts/x.mjs ../docs/reference/copy.md": {"docs/reference/copy.md"},
		"\t@bash scripts/check.sh docs/handbook/a.md":                   {"docs/handbook/a.md", "scripts/check.sh"},
		"\tcd frontend && pnpm exec vitest run src/x.ts":                nil,
	} {
		got := recipeReads(plantedTree(), line)
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("recipeReads(%q) = %v, want %v", line, got, want)
		}
	}
}

// A planted reader of a path the scope leaves out is reported, and adding the
// path to the scope clears it.
func TestAReadTheFrontendScopeMissesIsReported(t *testing.T) {
	t.Parallel()
	const reader = "frontend/src/i18n/r.test.ts"
	source := `readFileSync(resolve(import.meta.dirname, "../../../docs/reference/copy.md"));`
	ts := newTSReader(plantedTree(), map[string]string{reader: source})
	var reads []laneRead
	for _, read := range readsOf(ts, reader, source) {
		reads = append(reads, laneRead{reader, read})
	}
	scope := []string{"frontend/**", "backend/api/**"}
	if missing := uncoveredReads(reads, scope); len(missing) != 1 || missing[0].path != "docs/reference/copy.md" {
		t.Fatalf("a read of docs/reference/copy.md against %v reported %v, want that one read", scope, missing)
	}
	if missing := uncoveredReads(reads, append(scope, "docs/reference/*.md")); len(missing) != 0 {
		t.Errorf("a scope covering docs/reference/*.md still reports %v", missing)
	}
}
