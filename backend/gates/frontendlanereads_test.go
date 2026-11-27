// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// The frontend lane runs on a change to every path outside frontend/ it reads.
//
// The readers are the frontend trees, the scripts the lane runs or sources, and
// the recipes it makes. Out of reach: a path from process.cwd(), an environment
// variable or a loop's arithmetic, and a subprocess built at run time. What a
// regular expression keeps from a listing is declared in wholeTreeReaders.

import (
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const frontendTree = "frontend"

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

// trackedModule resolves a module specifier written without its ending. An import
// may name x.js for x.ts, as TypeScript resolves it.
func (tree repoTree) trackedModule(base string) (string, bool) {
	for _, candidate := range []string{base, strings.TrimSuffix(base, ".js") + ".ts", base + ".ts", base + ".tsx",
		base + ".mts", base + ".js", base + ".mjs", base + ".json", base + "/index.ts"} {
		if tree.files[candidate] {
			return candidate, true
		}
	}
	return "", false
}

func (tree repoTree) tracked(p string) (string, bool) {
	if tree.dirs[p] {
		return p, true
	}
	return tree.trackedModule(p)
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

func ancestors(reader string) []string {
	var dirs []string
	for dir := path.Dir(reader); ; dir = path.Dir(dir) {
		dirs = append(dirs, dir)
		if dir == "." {
			return dirs
		}
	}
}

type readerKind string

const (
	notAReader   readerKind = ""
	scriptReader readerKind = "TypeScript, JavaScript, CSS and HTML"
	configReader readerKind = "JSON and YAML config"
	shellReader  readerKind = "shell"
	recipeReader readerKind = "Makefile recipe"
)

var readerKinds = map[string]readerKind{
	".ts": scriptReader, ".tsx": scriptReader, ".mts": scriptReader, ".cts": scriptReader,
	".js": scriptReader, ".jsx": scriptReader, ".mjs": scriptReader, ".cjs": scriptReader,
	".css": scriptReader, ".html": scriptReader, ".mdx": scriptReader,
	".json": configReader, ".yaml": configReader, ".yml": configReader,
	".sh": shellReader,
}

// readerKindOf reads a file with no ending by its shebang.
func readerKindOf(file, source string) readerKind {
	if kind := readerKinds[path.Ext(file)]; kind != notAReader || path.Ext(file) != "" {
		return kind
	}
	shebang, _, _ := strings.Cut(source, "\n")
	switch {
	case !strings.HasPrefix(shebang, "#!"):
		return notAReader
	case strings.Contains(shebang, "node") || strings.Contains(shebang, "tsx") || strings.Contains(shebang, "deno"):
		return scriptReader
	case strings.Contains(shebang, "sh"):
		return shellReader
	}
	return notAReader
}

var (
	configEscape = regexp.MustCompile(`(?:^|[\s"'=:(\[])((?:\.\./)+[\w@.*/-]*)`)
	shellVarPath = regexp.MustCompile(`\$\{?\w+\}?((?:/[\w.*-]+)+)`)
	commandPath  = regexp.MustCompile(`(?:^|[\s"'=(])((?:\./|(?:\.\./)+)?\w[\w.*-]*(?:/[\w.*-]+)+|\./\w[\w.*-]*)`)
)

// A config's escapes are relative to its own directory.
func configReads(tree repoTree, file, source string) []string {
	text, _ := stripTSComments(source)
	if path.Ext(file) != ".json" {
		var kept []string
		for line := range strings.SplitSeq(source, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "#") {
				kept = append(kept, line)
			}
		}
		text = strings.Join(kept, "\n")
	}
	var found []string
	for _, match := range configEscape.FindAllStringSubmatch(text, -1) {
		if read, ok := tree.resolveRead(match[1], []string{path.Dir(file)}); ok {
			found = append(found, read)
		}
	}
	return found
}

func shellReads(tree repoTree, file, source string) []string {
	anchors := ancestors(file)
	var found []string
	for _, command := range scriptCommands(source) {
		for _, rel := range commandPaths(command) {
			if read, ok := tree.resolveRead(rel, anchors); ok {
				found = append(found, read)
			}
		}
	}
	return found
}

var (
	caseOpen        = regexp.MustCompile(`^case\b.*\bin$`)
	shellPrint      = []string{"echo", "printf", "true", "false", ":"}
	shellDeclarers  = []string{"local", "export", "readonly", "declare"}
	shellAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
)

// scriptCommands is each simple command of a script. A case pattern is dropped:
// it names the paths it matches, not paths the script opens.
func scriptCommands(source string) []string {
	var commands []string
	depth, pattern := 0, false
	for line := range strings.SplitSeq(source, "\n") {
		text := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(text, "#"):
			continue
		case caseOpen.MatchString(text):
			depth, pattern = depth+1, true
			continue
		case depth > 0 && strings.HasPrefix(text, "esac"):
			depth, pattern = depth-1, false
			continue
		case pattern:
			closing := strings.IndexByte(text, ')')
			if closing < 0 {
				continue
			}
			text, pattern = text[closing+1:], false
		}
		if depth > 0 && strings.HasSuffix(text, ";;") {
			text, pattern = strings.TrimSuffix(text, ";;"), true
		}
		commands = append(commands, simpleCommands(text)...)
	}
	return commands
}

// commandPaths is the path-shaped words of one simple command. What echo and
// printf print, and an output redirection, are no reads.
func commandPaths(command string) []string {
	words := strings.Fields(command)
	var candidates []string
	for len(words) > 0 && (shellAssignment.MatchString(words[0]) || slices.Contains(shellDeclarers, words[0])) {
		if _, value, assigned := strings.Cut(words[0], "="); assigned {
			candidates = append(candidates, value)
		}
		words = words[1:]
	}
	if len(words) == 0 || !slices.Contains(shellPrint, words[0]) {
		for i, word := range words {
			if strings.HasPrefix(word, ">") || i > 0 && bareRedirection.MatchString(words[i-1]) && strings.HasPrefix(words[i-1], ">") {
				continue
			}
			candidates = append(candidates, strings.TrimLeft(word, "<"))
		}
	}
	var rels []string
	for _, word := range candidates {
		word = strings.Trim(word, "\"'")
		for _, match := range shellVarPath.FindAllStringSubmatch(word, -1) {
			rels = append(rels, strings.TrimPrefix(match[1], "/"))
		}
		for _, match := range commandPath.FindAllStringSubmatch(word, -1) {
			rels = append(rels, match[1])
		}
	}
	return rels
}

func readsOf(ts *tsReader, file, source string) []string {
	switch readerKindOf(file, source) {
	case scriptReader:
		return ts.scriptReads(file)
	case configReader:
		return configReads(ts.tree, file, source)
	case shellReader:
		return shellReads(ts.tree, file, source)
	}
	return nil
}

// A path is relative to where the line's last cd left the shell. That starts at
// the directory of the Makefile that holds the line.
func recipeReads(tree repoTree, dir, line string) []string {
	var found []string
	cwd := dir
	for _, command := range simpleCommands(strings.TrimLeft(line, " \t@+-")) {
		words := strings.Fields(command)
		if len(words) == 2 && words[0] == "cd" {
			cwd = path.Join(cwd, words[1])
			continue
		}
		for _, rel := range commandPaths(command) {
			if read, ok := tree.resolveRead(rel, []string{cwd}); ok {
				found = append(found, read)
			}
		}
	}
	return found
}

var includeLine = regexp.MustCompile(`(?m)^-?include\s+(.+)$`)

// makefile is one directory's Makefile with the files it includes folded in.
type makefile struct {
	files   []string
	targets map[string]*recipeTarget
	vars    map[string]string
	// forward is the directory a match-anything rule hands an undeclared target to.
	forward string
}

var forwardRule = regexp.MustCompile(`(?m)^%:.*\n\t\$\(MAKE\) -C (\S+) \$@[ \t]*$`)

func loadMakefile(t *testing.T, dir string) makefile {
	t.Helper()
	main := path.Join(dir, "Makefile")
	body, err := gatekit.ReadFile(filepath.Join(repoRoot, main))
	if err != nil {
		t.Fatalf("reading %s: %v", main, err)
	}
	loaded := makefile{files: []string{main}, targets: map[string]*recipeTarget{}, vars: map[string]string{}}
	var files []string
	for _, include := range includeLine.FindAllStringSubmatch(string(body), -1) {
		for _, word := range strings.Fields(include[1]) {
			files = append(files, path.Join(dir, word))
		}
	}
	for _, file := range append(files, main) {
		targets, vars := parseRecipes(t, filepath.Join(repoRoot, file))
		for name, target := range targets {
			if loaded.targets[name] == nil {
				loaded.targets[name] = &recipeTarget{}
			}
			loaded.targets[name].pulls = append(loaded.targets[name].pulls, target.pulls...)
			loaded.targets[name].recipes = append(loaded.targets[name].recipes, target.recipes...)
		}
		for name, value := range vars {
			loaded.vars[name] = value
		}
	}
	loaded.files = append(files, main)
	if forward := forwardRule.FindStringSubmatch(string(body)); forward != nil {
		loaded.forward = path.Join(dir, forward[1])
	}
	return loaded
}

// laneRecipes walks the targets the frontend-scoped jobs make, through their
// prerequisites, $(MAKE) delegations and $(MAKE) -C into another Makefile.
func laneRecipes(t *testing.T, tree repoTree, workflow workflowFile, lanes []filteredLane) []laneRead {
	t.Helper()
	makefiles := map[string]makefile{}
	var reads []laneRead
	seen := map[string]bool{}
	var walk func(dir, name string)
	walk = func(dir, name string) {
		if _, loaded := makefiles[dir]; !loaded {
			makefiles[dir] = loadMakefile(t, dir)
			for _, file := range makefiles[dir].files {
				reads = append(reads, laneRead{"make", file})
			}
		}
		key := dir + "\x00" + name
		target := makefiles[dir].targets[name]
		if target == nil && makefiles[dir].forward != "" && !seen[key] {
			seen[key] = true
			walk(makefiles[dir].forward, name)
			return
		}
		if seen[key] || target == nil {
			return
		}
		seen[key] = true
		reader := "make " + name
		if dir != "." {
			reader = "make -C " + dir + " " + name
		}
		for _, pulled := range target.pulls {
			walk(dir, pulled)
		}
		for _, recipe := range target.recipes {
			command := strings.ReplaceAll(expandRefs(recipe, makefiles[dir].vars), "$@", name)
			for _, read := range recipeReads(tree, dir, command) {
				reads = append(reads, laneRead{reader, read})
			}
			for _, sub := range subMakes(command) {
				for _, delegated := range sub.targets {
					walk(path.Join(dir, sub.dir), delegated)
				}
			}
		}
	}
	for _, lane := range lanes {
		for _, command := range jobCommands(t, workflow, workflow.Jobs[lane.name].Uses, lane.name) {
			for _, invocation := range makeInvocation.FindAllStringSubmatch(command, -1) {
				walk(".", invocation[1])
			}
		}
	}
	return reads
}

var frontendGuard = regexp.MustCompile(`outputs\s*(?:\.\s*frontend\b|\[\s*['"]frontend['"]\s*\])`)

// frontendLanes is every job the frontend scope gates. A spelling of the scope
// that filteredLanes does not parse fails here: the census would lose the job.
func frontendLanes(t *testing.T, workflow workflowFile) []filteredLane {
	t.Helper()
	lanes, unparsed, gated := frontendGatedJobs(t, workflow)
	if gated == 0 {
		t.Fatalf("no job in %s runs on the %q scope; this census would judge no lane", classifierWorkflow, frontendTree)
	}
	for _, name := range unparsed {
		t.Fatalf("the %s job in %s runs on the %q scope in a condition filteredLanes does not read: "+
			"spell it needs.changes.outputs.%s, or teach the reader the new spelling", name, classifierWorkflow, frontendTree, frontendTree)
	}
	return lanes
}

// frontendGatedJobs returns the lanes on the frontend scope, and in unparsed each
// job that names the scope in a spelling filteredLanes misses.
func frontendGatedJobs(t *testing.T, workflow workflowFile) (lanes []filteredLane, unparsed []string, gated int) {
	t.Helper()
	parsed := map[string]bool{}
	for _, lane := range filteredLanes(t, workflow) {
		if slices.Contains(lane.scopes, frontendTree) {
			lanes = append(lanes, lane)
			parsed[lane.name] = true
		}
	}
	for name, job := range workflow.Jobs {
		if frontendGuard.MatchString(job.If) {
			gated++
			if !parsed[name] {
				unparsed = append(unparsed, name)
			}
		}
	}
	slices.Sort(unparsed)
	return lanes, unparsed, gated
}

func TestAFrontendGuardInAnotherSpellingIsNotDropped(t *testing.T) {
	t.Parallel()
	var workflow workflowFile
	source := "jobs:\n" +
		"  dotted:\n    if: needs.changes.outputs.frontend == 'true'\n    steps: [{run: make fe-lint}]\n" +
		"  indexed:\n    if: needs.changes.outputs['frontend'] == 'true'\n    steps: [{run: make fe-unit}]\n"
	if err := unmarshalYAML([]byte(source), &workflow); err != nil {
		t.Fatalf("parsing the planted workflow: %v", err)
	}
	lanes, unparsed, gated := frontendGatedJobs(t, workflow)
	if gated != 2 || len(lanes) != 1 || !slices.Equal(unparsed, []string{"indexed"}) {
		t.Errorf("two jobs on the frontend scope, one spelled with an index: read %d gated, %d lane(s), unparsed %v; want 2, 1, [indexed]",
			gated, len(lanes), unparsed)
	}
}

// wholeTreeReader is a reader that lists the whole repository. keeps holds the
// regular expressions it keeps, verbatim, since the census cannot follow a path
// through one. A reader the lane never runs that way says why in unread.
type wholeTreeReader struct {
	keeps  []string
	unread string
}

var wholeTreeReaders = map[string]wholeTreeReader{
	"frontend/scripts/tsconfig-roots.test.ts": {keeps: []string{`(^|\/)(Makefile|[^/]+\.mk)$`, `^(frontend|extensions\/(.+\/)?frontend)\/`}},
	"frontend/scripts/move-modules.ts":        {unread: "its git grep runs from the command line alone; move-modules.test.ts imports planMoves and rewriteImports, which list nothing"},
}

// narrowWholeTree replaces each read of "." with the tracked files its reader keeps.
func narrowWholeTree(t *testing.T, tree repoTree, sources map[string]string, reads []laneRead) []laneRead {
	t.Helper()
	var narrowed []laneRead
	listers := map[string]bool{}
	for _, read := range reads {
		declared, ok := wholeTreeReaders[read.reader]
		if read.path != "." || !ok {
			narrowed = append(narrowed, read)
			continue
		}
		listers[read.reader] = true
		for _, expression := range declared.keeps {
			if !strings.Contains(sources[read.reader], "/"+expression+"/") {
				t.Errorf("%s no longer holds the expression /%s/ that wholeTreeReaders says it keeps from its listing", read.reader, expression)
			}
			kept := regexp.MustCompile(expression)
			for file := range tree.files {
				if kept.MatchString(file) && !insideFrontend(file) {
					narrowed = append(narrowed, laneRead{read.reader, file})
				}
			}
		}
	}
	for reader := range wholeTreeReaders {
		if !listers[reader] {
			t.Errorf("wholeTreeReaders names %s, which no longer lists the whole repository: remove it", reader)
		}
	}
	return narrowed
}

var laneReaderTree = regexp.MustCompile(`^(frontend|extensions/(.+/)?frontend)/`)

// laneCensus is every outside read, and how many each kind of reader makes.
type laneCensus struct {
	reads  []laneRead
	byKind map[readerKind]int
}

func frontendLaneReads(t *testing.T, workflow workflowFile) laneCensus {
	t.Helper()
	var paths []string
	for _, file := range trackedFiles(t) {
		if !file.symlink {
			paths = append(paths, file.path)
		}
	}
	tree := newRepoTree(paths)
	lanes := frontendLanes(t, workflow)
	sources := map[string]string{}
	var queue []string
	enqueue := func(file string) {
		if _, read := sources[file]; read || !tree.files[file] {
			return
		}
		body, err := gatekit.ReadFile(filepath.Join(repoRoot, file))
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}
		if readerKindOf(file, string(body)) != notAReader {
			sources[file] = string(body)
			queue = append(queue, file)
		}
	}
	for _, file := range paths {
		if laneReaderTree.MatchString(file) && (readerKinds[path.Ext(file)] != notAReader || path.Ext(file) == "") {
			enqueue(file)
		}
	}
	for _, lane := range lanes {
		for _, input := range lane.inputs {
			enqueue(input)
		}
	}
	census := laneCensus{byKind: map[readerKind]int{}}
	scripts := map[string]string{}
	for file, source := range sources {
		if readerKindOf(file, source) == scriptReader {
			scripts[file] = source
		}
	}
	ts := newTSReader(tree, scripts)
	for len(queue) > 0 {
		file := queue[0]
		queue = queue[1:]
		kind := readerKindOf(file, sources[file])
		if _, added := ts.code[file]; !added && kind == scriptReader {
			ts.add(file, sources[file])
		}
		for _, read := range readsOf(ts, file, sources[file]) {
			census.reads = append(census.reads, laneRead{file, read})
			census.byKind[kind]++
			if kind := readerKinds[path.Ext(read)]; kind == scriptReader || kind == shellReader {
				enqueue(read)
			}
		}
	}
	recipes := laneRecipes(t, tree, workflow, lanes)
	census.byKind[recipeReader] = len(recipes)
	census.reads = narrowWholeTree(t, tree, sources, append(census.reads, recipes...))
	slices.SortFunc(census.reads, func(a, b laneRead) int {
		return strings.Compare(a.reader+"\x00"+a.path, b.reader+"\x00"+b.path)
	})
	census.reads = slices.Compact(census.reads)
	return census
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

// readFloors sits under each kind's count on today's tree, so a kind of reader
// the census stopped seeing fails rather than shrinking the sum.
var readFloors = map[readerKind]int{scriptReader: 40, configReader: 2, shellReader: 8, recipeReader: 9}

func TestTheFrontendLaneRunsOnAChangeToEveryPathItReads(t *testing.T) {
	t.Parallel()
	workflow := readWorkflow(t, classifierWorkflow)
	scope := filterScopes(t, workflow)[frontendTree]
	if len(scope) == 0 {
		t.Fatalf("%s declares no %q paths-filter scope; this census has nothing to judge against", classifierWorkflow, frontendTree)
	}
	census := frontendLaneReads(t, workflow)
	for kind, floor := range readFloors {
		if census.byKind[kind] < floor {
			t.Errorf("the census found %d outside read(s) by %s readers and expects at least %d: it has stopped recognising them",
				census.byKind[kind], kind, floor)
		}
	}
	for _, read := range uncoveredReads(census.reads, scope) {
		if read.path == "." {
			t.Errorf("%s lists the whole repository. Name the regular expressions it keeps from the listing in wholeTreeReaders.", read.reader)
			continue
		}
		t.Errorf("%s reads %s, which the %q filter in %s does not cover.\n"+
			"A pull request that changes only that path skips the frontend lane, so the reader can break and merge green.\n"+
			"Add the path, or the tree it sits in, to the %q scope.", read.reader, read.path, frontendTree, classifierWorkflow, frontendTree)
	}
}

func TestARecipeReadIsPlacedOnTheDirectoryItsCdLeft(t *testing.T) {
	t.Parallel()
	cases := []struct {
		dir, line string
		want      []string
	}{
		{".", "\tcd frontend && node scripts/x.mjs ../docs/reference/copy.md", []string{"docs/reference/copy.md"}},
		{".", "\t@bash scripts/check.sh docs/handbook/a.md", []string{"docs/handbook/a.md", "scripts/check.sh"}},
		{".", "\t@./scripts/check.sh", []string{"scripts/check.sh"}},
		{"backend", "\tgo run ./internal/a", []string{"backend/internal/a"}},
		{".", "\tcd frontend && pnpm exec vitest run src/x.ts", nil},
	}
	for _, c := range cases {
		got := recipeReads(plantedTree(), c.dir, c.line)
		slices.Sort(got)
		if !slices.Equal(got, c.want) {
			t.Errorf("recipeReads(%q, %q) = %v, want %v", c.dir, c.line, got, c.want)
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
