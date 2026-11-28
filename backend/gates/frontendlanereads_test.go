// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// The frontend lane runs on a change to every path outside frontend/ it reads.
//
// Readers: the frontend trees, the frontend-scoped jobs' workflows and actions,
// scripts those run or source, and recipes they make. A read is a path built from
// literals, imports, parameters and the module's own location. It reaches an fs
// call, an import, a glob, a subprocess, a config escape, a markup reference or a
// command. wholeTreeReaders declares what a regular expression keeps from a listing.

import (
	"fmt"
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

// An import may name x.js for x.ts, or x.mjs for x.mts, as TypeScript resolves it.
func (tree repoTree) trackedModule(base string) (string, bool) {
	for _, candidate := range []string{base, strings.TrimSuffix(base, ".js") + ".ts", strings.TrimSuffix(base, ".mjs") + ".mts",
		base + ".ts", base + ".tsx", base + ".d.ts", base + ".mts", base + ".js", base + ".mjs", base + ".json",
		base + "/index.ts", base + "/index.tsx"} {
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
	notAReader      readerKind = ""
	scriptReader    readerKind = "TypeScript and JavaScript"
	configReader    readerKind = "JSON and YAML config"
	shellReader     readerKind = "shell"
	markupReader    readerKind = "CSS, HTML and MDX"
	recipeReader    readerKind = "Makefile recipe"
	subMakeReads    readerKind = "sub-Makefile recipe"
	subprocessReads readerKind = "subprocess"
	workflowReads   readerKind = "workflow and action"
)

var readerKinds = map[string]readerKind{
	".ts": scriptReader, ".tsx": scriptReader, ".mts": scriptReader, ".cts": scriptReader,
	".js": scriptReader, ".jsx": scriptReader, ".mjs": scriptReader, ".cjs": scriptReader,
	".json": configReader, ".yaml": configReader, ".yml": configReader,
	".sh":  shellReader,
	".css": markupReader, ".html": markupReader, ".mdx": markupReader,
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

func readsOf(ts *tsReader, file, source string) []string {
	switch readerKindOf(file, source) {
	case scriptReader:
		return ts.scriptReads(file)
	case configReader:
		return configReads(ts.tree, file, source)
	case shellReader:
		return shellReads(ts.tree, file, source)
	case markupReader:
		return markupReads(ts.tree, file, source)
	}
	return nil
}

var frontendGuard = regexp.MustCompile(`outputs\s*(?:\.\s*frontend\b|\[\s*['"]frontend['"]\s*\])`)

// A spelling of the scope that filteredLanes does not parse fails here: the
// census would lose the job.
func frontendLanes(t *testing.T, workflow workflowFile) []filteredLane {
	t.Helper()
	lanes, unparsed, gated := frontendGatedJobs(t, workflow, filterScopes(t, workflow))
	if gated == 0 {
		t.Fatalf("no job in %s runs on the %q scope; this census would judge no lane", classifierWorkflow, frontendTree)
	}
	for _, name := range unparsed {
		t.Fatalf("the %s job in %s runs on the %q scope in a condition filteredLanes does not read: "+
			"spell it needs.changes.outputs.%s, or teach the reader the new spelling", name, classifierWorkflow, frontendTree, frontendTree)
	}
	return lanes
}

// gated counts the jobs whose condition names the frontend scope. unparsed is each
// of them that spells the scope in a way filteredLanes misses.
func frontendGatedJobs(t *testing.T, workflow workflowFile, scopes map[string][]string) (lanes []filteredLane, unparsed []string, gated int) {
	t.Helper()
	parsed := map[string]bool{}
	for _, lane := range filteredLanes(t, workflow, scopes) {
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
	lanes, unparsed, gated := frontendGatedJobs(t, workflow, map[string][]string{frontendTree: {"frontend/**"}})
	if gated != 2 || len(lanes) != 1 || !slices.Equal(unparsed, []string{"indexed"}) {
		t.Errorf("two jobs on the frontend scope, one spelled with an index: read %d gated, %d lane(s), unparsed %v; want 2, 1, [indexed]",
			gated, len(lanes), unparsed)
	}
}

// wholeTreeReader is a reader that lists the whole repository. Each regular
// expression in it is named: keeps holds those that narrow the listing, and others
// says why each of the rest does not. A reader the lane never runs that way says
// why in unread.
type wholeTreeReader struct {
	keeps  []string
	others map[string]string
	unread string
}

var wholeTreeReaders = map[string]wholeTreeReader{
	"frontend/scripts/tsconfig-roots.test.ts": {
		keeps: []string{`(^|\/)(Makefile|[^/]+\.mk)$`, `^(frontend|extensions\/(.+\/)?frontend)\/`},
		others: map[string]string{
			`\.(ts|tsx|mts|cts)$`:                    "it narrows only paths the frontend expression kept",
			`(^|\/)tsconfig[^/]*\.json$`:             "it narrows only paths the frontend expression kept",
			`(\.d\.ts|\.tsx)$`:                       "it narrows only paths the frontend expression kept",
			`^[@-]*cd\s+(\S+)\s*&&`:                  "it reads a recipe line, not the listing",
			`\btsc(?=[ \t]|$)((?:[ \t]+[^\s&|;]+)*)`: "it reads a recipe line, not the listing",
			`\bpnpm\s+(?:run\s+)?([\w:-]+)`:          "it reads a recipe line, not the listing",
			`\s+`:                                    "it splits a command into words",
		},
	},
	"frontend/scripts/move-modules.ts": {unread: "its git grep runs from the command line alone; move-modules.test.ts imports planMoves and rewriteImports, which list nothing"},
}

// narrowWholeTree replaces each read of "." with the tracked files its reader
// keeps. laneText is what the lane runs as commands, which names no unread reader.
func narrowWholeTree(t *testing.T, tree repoTree, ts *tsReader, laneText string, reads []laneRead) []laneRead {
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
		if declared.unread != "" && strings.Contains(laneText, path.Base(read.reader)) {
			t.Errorf("wholeTreeReaders says the lane never runs %s, and a lane command names it", read.reader)
		}
		if declared.unread == "" {
			for _, problem := range wholeTreeProblems(read.reader, ts.files[read.reader].regexSources(), declared) {
				t.Error(problem)
			}
		}
		for _, expression := range declared.keeps {
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

// An undeclared regular expression and a stale declaration are both problems.
func wholeTreeProblems(reader string, found []string, declared wholeTreeReader) []string {
	var problems []string
	named := slices.Concat(declared.keeps, slices.Collect(maps.Keys(declared.others)))
	for _, expression := range found {
		if !slices.Contains(named, expression) {
			problems = append(problems, fmt.Sprintf("%s holds /%s/, which wholeTreeReaders does not name: put it in keeps when it narrows the listing, or in others with why it does not", reader, expression))
		}
	}
	for _, expression := range named {
		if !slices.Contains(found, expression) {
			problems = append(problems, fmt.Sprintf("wholeTreeReaders names /%s/ for %s, which no longer holds it", expression, reader))
		}
	}
	slices.Sort(problems)
	return problems
}

func TestAWholeTreeReaderNamesEachOfItsExpressions(t *testing.T) {
	t.Parallel()
	declared := wholeTreeReader{keeps: []string{"kept"}, others: map[string]string{"gone": "why"}}
	problems := wholeTreeProblems("r.ts", []string{"kept", "unnamed"}, declared)
	if len(problems) != 2 || !strings.Contains(problems[0], "/unnamed/") || !strings.Contains(problems[1], "/gone/") {
		t.Errorf("an unnamed expression and a stale one read %q, want both reported", problems)
	}
}

var laneReaderTree = regexp.MustCompile(`^(frontend|extensions/(.+/)?frontend)/`)

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
	jobs := readLaneJobs(t, tree, lanes)
	recipes := laneRecipes(t, tree, jobs.makes)
	census := laneCensus{byKind: map[readerKind]int{
		workflowReads: len(jobs.reads), recipeReader: len(recipes.reads), subMakeReads: recipes.subMakes,
	}}
	census.reads = slices.Concat(jobs.reads, recipes.reads)
	queue := laneQueue{tree: tree, open: openTracked, sources: map[string]string{}}
	for _, file := range paths {
		if laneReaderTree.MatchString(file) && (readerKinds[path.Ext(file)] != notAReader || path.Ext(file) == "") {
			queue.enqueue(t, file)
		}
	}
	for _, lane := range lanes {
		for _, input := range lane.inputs {
			queue.enqueue(t, input)
		}
	}
	for _, read := range census.reads {
		queue.enqueue(t, read.path)
	}
	scripts := map[string]string{}
	for file, source := range queue.sources {
		if readerKindOf(file, source) == scriptReader {
			scripts[file] = source
		}
	}
	ts, err := newTSReader(tree, scripts, openTracked)
	if err != nil {
		t.Error(err)
	}
	laneText := slices.Concat(recipes.lines, jobs.runs, queue.drain(t, ts, &census))
	census.byKind[subprocessReads] = ts.spawned
	census.reads = narrowWholeTree(t, tree, ts, strings.Join(laneText, "\n"), census.reads)
	slices.SortFunc(census.reads, func(a, b laneRead) int {
		return strings.Compare(a.reader+"\x00"+a.path, b.reader+"\x00"+b.path)
	})
	census.reads = slices.Compact(census.reads)
	return census
}

func openTracked(file string) (string, error) {
	body, err := gatekit.ReadFile(filepath.Join(repoRoot, file))
	return string(body), err
}

// laneQueue holds the readers the census has yet to read, and each one's source.
type laneQueue struct {
	tree    repoTree
	open    func(file string) (string, error)
	sources map[string]string
	files   []string
}

func (q *laneQueue) enqueue(t *testing.T, file string) {
	t.Helper()
	if _, read := q.sources[file]; read || !q.tree.files[file] {
		return
	}
	source, err := q.open(file)
	if err != nil {
		t.Fatalf("reading %s: %v", file, err)
	}
	if readerKindOf(file, source) != notAReader {
		q.sources[file] = source
		q.files = append(q.files, file)
	}
}

// drain reads each queued reader into census, and answers the shell text the lane
// runs. A read that is itself a reader joins the queue.
func (q *laneQueue) drain(t *testing.T, ts *tsReader, census *laneCensus) (shellText []string) {
	t.Helper()
	for len(q.files) > 0 {
		file := q.files[0]
		q.files = q.files[1:]
		kind := readerKindOf(file, q.sources[file])
		if _, added := ts.files[file]; !added && kind == scriptReader {
			if err := ts.add(file, q.sources[file]); err != nil {
				t.Errorf("the census cannot read %s, so it cannot see its reads: %v", file, err)
			}
		}
		if kind == shellReader {
			shellText = append(shellText, q.sources[file])
		}
		for _, read := range readsOf(ts, file, q.sources[file]) {
			census.reads = append(census.reads, laneRead{file, read})
			census.byKind[kind]++
			if kind := readerKinds[path.Ext(read)]; kind == scriptReader || kind == shellReader || kind == configReader {
				q.enqueue(t, read)
			}
		}
	}
	return shellText
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
var readFloors = map[readerKind]int{
	scriptReader: 40, configReader: 2, shellReader: 8, recipeReader: 9, subMakeReads: 1, subprocessReads: 4, workflowReads: 10,
}

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

// A planted reader of a path the scope leaves out is reported, and adding the
// path to the scope clears it.
func TestAReadTheFrontendScopeMissesIsReported(t *testing.T) {
	t.Parallel()
	const reader = "frontend/src/i18n/r.test.ts"
	source := `readFileSync(resolve(import.meta.dirname, "../../../docs/reference/copy.md"));`
	ts, err := newTSReader(plantedTree(), map[string]string{reader: source}, nil)
	if err != nil {
		t.Fatal(err)
	}
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
