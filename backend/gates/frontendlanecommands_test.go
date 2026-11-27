// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

import (
	"cmp"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var (
	configEscape = regexp.MustCompile(`(?:^|[\s"'=:(\[])((?:\.\./)+[\w@.*/-]*)`)
	shellVarPath = regexp.MustCompile(`\$\{?\w+\}?((?:/[\w.*-]+)+)`)
	commandPath  = regexp.MustCompile(`(?:^|[\s"'=(])((?:\./|(?:\.\./)+)?\w[\w.*-]*(?:/[\w.*-]+)+|\./\w[\w.*-]*)`)
)

// A config's escapes are relative to its own directory.
func configReads(tree repoTree, file, source string) []string {
	text := gatekit.TSWithoutComments(source)
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
	heredocOpen     = regexp.MustCompile(`<<-?\s*(['"]?)(\w+)['"]?`)
	shellPrint      = []string{"echo", "printf", "true", "false", ":"}
	shellRunners    = []string{"bash", "sh", "source", "."}
	shellDeclarers  = []string{"local", "export", "readonly", "declare"}
	shellAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
)

// A case pattern names the paths it matches, and a heredoc no shell runs is
// data: both are dropped.
func scriptCommands(source string) []string {
	var commands []string
	depth, pattern, heredoc := 0, false, ""
	for line := range strings.SplitSeq(source, "\n") {
		text := strings.TrimSpace(line)
		switch {
		case heredoc != "":
			if text == heredoc {
				heredoc = ""
			}
			continue
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
		if open := heredocOpen.FindStringSubmatchIndex(text); open != nil {
			if words := strings.Fields(text[:open[0]]); len(words) == 0 || !slices.Contains(shellRunners, words[0]) {
				heredoc = text[open[4]:open[5]]
			}
			text = text[:open[0]]
		}
		commands = append(commands, simpleCommands(text)...)
	}
	return commands
}

// What echo and printf print and what a write command such as mkdir is handed are
// no reads. Neither are a copy's destination and an output redirection.
func commandPaths(command string) []string {
	words := strings.Fields(command)
	var candidates []string
	for len(words) > 0 && (shellAssignment.MatchString(words[0]) || slices.Contains(shellDeclarers, words[0])) {
		if _, value, assigned := strings.Cut(words[0], "="); assigned {
			candidates = append(candidates, value)
		}
		words = words[1:]
	}
	if len(words) > 2 && slices.Contains(copyCommands, words[0]) {
		words = withoutIndices(words, copyDestination(words))
	}
	if len(words) > 0 && !slices.Contains(shellPrint, words[0]) && !writesOnly(words) {
		for i, word := range words {
			if strings.HasPrefix(word, ">") || i > 0 && (words[i-1] == "-o" || bareRedirection.MatchString(words[i-1]) && strings.HasPrefix(words[i-1], ">")) {
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

func commandLines(lines []string) []string {
	var joined []string
	pending := ""
	for _, line := range lines {
		if continued, ok := strings.CutSuffix(strings.TrimRight(line, " \t"), "\\"); ok {
			pending += continued + " "
			continue
		}
		joined = append(joined, pending+line)
		pending = ""
	}
	if pending != "" {
		joined = append(joined, pending)
	}
	return joined
}

// shellLine reads one command line run from cwd, and answers where it leaves the
// shell. A path is relative to the last cd, a subshell's cd ends with it, and
// $(CURDIR) is the Makefile's dir. A cd it cannot follow fails the census.
func shellLine(tree repoTree, dir, cwd, line string) (found []string, calls []makeCall, end string, err error) {
	var subshells []string
	for _, command := range simpleCommands(strings.TrimLeft(line, " \t@+-")) {
		text := strings.TrimSpace(command)
		for strings.HasPrefix(text, "(") {
			subshells = append(subshells, cwd)
			text = strings.TrimSpace(text[1:])
		}
		closes := 0
		for strings.HasSuffix(text, ")") && strings.Count(text, ")") > strings.Count(text, "(") {
			text, closes = strings.TrimSpace(strings.TrimSuffix(text, ")")), closes+1
		}
		if rel, err := filepath.Rel(cwd, dir); err == nil && rel == "." {
			text = strings.ReplaceAll(text, "$(CURDIR)/", "")
		} else if err == nil {
			text = strings.ReplaceAll(text, "$(CURDIR)", filepath.ToSlash(rel))
		}
		words := strings.Fields(text)
		if len(words) > 0 && (words[0] == "pushd" || words[0] == "popd") {
			return nil, nil, cwd, fmt.Errorf("%q: the census follows cd, not the directory stack", line)
		}
		if len(words) > 0 && words[0] == "cd" {
			if cwd, err = cdTarget(cwd, words); err != nil {
				return nil, nil, cwd, fmt.Errorf("%q: %w", line, err)
			}
		} else {
			for _, rel := range commandPaths(text) {
				if read, ok := tree.resolveRead(rel, []string{cwd}); ok {
					found = append(found, read)
				}
			}
			calls = append(calls, makeCalls(cwd, text)...)
		}
		for ; closes > 0 && len(subshells) > 0; closes-- {
			cwd, subshells = subshells[len(subshells)-1], subshells[:len(subshells)-1]
		}
	}
	if len(subshells) > 0 {
		return nil, nil, cwd, fmt.Errorf("%q opens a subshell a later line closes, and the census reads a line at a time", line)
	}
	return found, calls, cwd, nil
}

// cdTarget refuses a cd the census cannot place: to home, back, an absolute path,
// or a name the shell expands.
func cdTarget(cwd string, words []string) (string, error) {
	if len(words) != 2 || words[1] == "-" || strings.HasPrefix(words[1], "/") || strings.HasPrefix(words[1], "~") ||
		strings.ContainsAny(words[1], "$\"'`*?[") {
		return cwd, fmt.Errorf("the census cannot follow %q, so it cannot place what the shell reads after it", strings.Join(words, " "))
	}
	return path.Join(cwd, words[1]), nil
}

var (
	includeLine = regexp.MustCompile(`(?m)^(-?)include\s+(.+)$`)
	forwardRule = regexp.MustCompile(`(?m)^%:.*\n\t\$\(MAKE\) -C (\S+) \$@[ \t]*$`)
)

// makefile is one directory's Makefile with the files it includes folded in.
type makefile struct {
	files   []string
	targets map[string]*recipeTarget
	vars    map[string]string
	// forward is the directory a match-anything rule hands an undeclared target to.
	forward string
}

func loadMakefile(t *testing.T, tree repoTree, dir string) makefile {
	t.Helper()
	main := path.Join(dir, "Makefile")
	body, err := gatekit.ReadFile(filepath.Join(repoRoot, main))
	if err != nil {
		t.Fatalf("reading %s: %v", main, err)
	}
	loaded := makefile{targets: map[string]*recipeTarget{}, vars: map[string]string{}}
	files, err := makefileIncludes(dir, string(body), func(file string) bool { return tree.files[file] })
	if err != nil {
		t.Fatal(err)
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

type makeCall struct {
	dir, target string
}

// GNU Make skips an absent -include file and fails on an absent plain include.
func makefileIncludes(dir, body string, exists func(string) bool) ([]string, error) {
	var files []string
	for _, include := range includeLine.FindAllStringSubmatch(body, -1) {
		for _, word := range strings.Fields(include[2]) {
			file := path.Join(dir, word)
			switch {
			case strings.ContainsAny(word, "$*?["):
				return nil, fmt.Errorf("%s includes %s, a name the census cannot spell out to tell absent from present", path.Join(dir, "Makefile"), word)
			case exists(file):
				files = append(files, file)
			case include[1] == "":
				return nil, fmt.Errorf("%s includes %s, which is not in the tree", path.Join(dir, "Makefile"), file)
			}
		}
	}
	return files, nil
}

// makeCalls reads both make and $(MAKE), and -C moves the directory a call runs in.
func makeCalls(dir, line string) []makeCall {
	var calls []makeCall
	for _, command := range simpleCommands(strings.TrimLeft(line, " \t@+-")) {
		words := strings.Fields(strings.Trim(strings.TrimSpace(command), "()"))
		for len(words) > 0 && environmentAssignment.MatchString(words[0]) {
			words = words[1:]
		}
		if len(words) > 0 && words[0] == "make" {
			command = "$(MAKE) " + strings.Join(words[1:], " ")
		}
		for _, sub := range subMakes(command) {
			for _, target := range sub.targets {
				calls = append(calls, makeCall{path.Join(dir, sub.dir), target})
			}
		}
	}
	return calls
}

// reads names each read by the target whose recipe reads it, and lines holds each
// recipe line walked. subMakes counts the reads of recipes outside the root Makefile.
type recipeWalk struct {
	reads    []laneRead
	lines    []string
	subMakes int
}

// laneRecipes walks the targets the frontend-scoped jobs make, through their
// prerequisites, $(MAKE) delegations and $(MAKE) -C into another Makefile.
func laneRecipes(t *testing.T, tree repoTree, starts []makeCall) recipeWalk {
	t.Helper()
	makefiles := map[string]makefile{}
	var walked recipeWalk
	seen := map[string]bool{}
	var walk func(dir, name string)
	walk = func(dir, name string) {
		if _, loaded := makefiles[dir]; !loaded {
			makefiles[dir] = loadMakefile(t, tree, dir)
			for _, file := range makefiles[dir].files {
				walked.reads = append(walked.reads, laneRead{"make", file})
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
		for _, recipe := range commandLines(target.recipes) {
			command := strings.ReplaceAll(expandRefs(recipe, makefiles[dir].vars), "$@", name)
			walked.lines = append(walked.lines, command)
			reads, calls, _, err := shellLine(tree, dir, dir, command)
			if err != nil {
				t.Errorf("%s: %v", reader, err)
			}
			for _, read := range reads {
				walked.reads = append(walked.reads, laneRead{reader, read})
				if dir != "." {
					walked.subMakes++
				}
			}
			for _, call := range calls {
				walk(call.dir, call.target)
			}
		}
	}
	for _, start := range starts {
		walk(start.dir, start.target)
	}
	return walked
}

type laneStep struct {
	Uses             string `yaml:"uses"`
	Run              string `yaml:"run"`
	WorkingDirectory string `yaml:"working-directory"`
}

type laneDefaults struct {
	Run struct {
		WorkingDirectory string `yaml:"working-directory"`
	} `yaml:"run"`
}

type laneWorkflowFile struct {
	Defaults laneDefaults `yaml:"defaults"`
	Jobs     map[string]struct {
		Uses     string       `yaml:"uses"`
		Defaults laneDefaults `yaml:"defaults"`
		Steps    []laneStep   `yaml:"steps"`
	} `yaml:"jobs"`
}

type laneAction struct {
	Runs struct {
		Steps []laneStep `yaml:"steps"`
	} `yaml:"runs"`
}

// laneJobs is what the frontend-scoped jobs read as CI. That is the workflow files
// that hold them, each local action they use, and the paths their steps run.
type laneJobs struct {
	reads []laneRead
	makes []makeCall
	runs  []string
}

func readLaneJobs(t *testing.T, tree repoTree, lanes []filteredLane) laneJobs {
	t.Helper()
	var jobs laneJobs
	caller := strings.TrimPrefix(classifierWorkflow, "../")
	for _, lane := range lanes {
		jobs.walkJob(t, tree, caller, lane.name, ".")
	}
	return jobs
}

func readYAMLFile[T laneWorkflowFile | laneAction](t *testing.T, file string) T {
	t.Helper()
	var parsed T
	body, err := gatekit.ReadFile(filepath.Join(repoRoot, file))
	if err != nil {
		t.Fatalf("reading %s: %v", file, err)
	}
	if err := unmarshalYAML(body, &parsed); err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}
	return parsed
}

func (jobs *laneJobs) walkJob(t *testing.T, tree repoTree, file, name, workdir string) {
	t.Helper()
	workflow := readYAMLFile[laneWorkflowFile](t, file)
	jobs.reads = append(jobs.reads, laneRead{"job " + name, file})
	job := workflow.Jobs[name]
	if called, ok := strings.CutPrefix(job.Uses, "./"); ok {
		lane := readYAMLFile[laneWorkflowFile](t, called)
		for inner := range lane.Jobs {
			jobs.walkJob(t, tree, called, inner, ".")
		}
		return
	}
	dir := cmp.Or(job.Defaults.Run.WorkingDirectory, workflow.Defaults.Run.WorkingDirectory, workdir)
	jobs.walkSteps(t, tree, "job "+name, job.Steps, dir)
}

func (jobs *laneJobs) walkSteps(t *testing.T, tree repoTree, reader string, steps []laneStep, dir string) {
	t.Helper()
	for _, step := range steps {
		if action, ok := strings.CutPrefix(step.Uses, "./"); ok {
			jobs.reads = append(jobs.reads, laneRead{reader, action})
			var composite laneAction
			for _, name := range []string{"action.yml", "action.yaml"} {
				if tree.files[path.Join(action, name)] {
					composite = readYAMLFile[laneAction](t, path.Join(action, name))
				}
			}
			jobs.walkSteps(t, tree, reader, composite.Runs.Steps, ".")
		}
		// One run: block is one shell, so a cd holds until the step ends.
		workdir := cmp.Or(step.WorkingDirectory, dir)
		cwd := workdir
		for _, line := range commandLines(strings.Split(step.Run, "\n")) {
			jobs.runs = append(jobs.runs, line)
			var reads []string
			var calls []makeCall
			var err error
			if reads, calls, cwd, err = shellLine(tree, workdir, cwd, line); err != nil {
				t.Errorf("%s: %v", reader, err)
				break
			}
			for _, read := range reads {
				jobs.reads = append(jobs.reads, laneRead{reader, read})
			}
			jobs.makes = append(jobs.makes, calls...)
		}
	}
}
