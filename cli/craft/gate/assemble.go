package gate

import (
	"context"
	"github.com/margince/margince/cli/craft/rubric"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// runGit runs a git command in dir and returns trimmed stdout. It is a field so
// tests can inject a fake without a real repo.
type runGit func(ctx context.Context, dir string, args ...string) (string, error)

func execGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: fixed "git" binary, args are internal diff-range literals
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// Assembler builds review Inputs from a git range. The full touched-file content
// and sibling files (not just the diff hunks) are what let the reviewer judge
// style drift against the surrounding code (T5).
type Assembler struct {
	Root string
	Git  runGit
}

// NewAssembler returns an Assembler rooted at root, wired to the real git binary.
func NewAssembler(root string) *Assembler { return &Assembler{Root: root, Git: execGit} }

// Assemble gathers the diff, the full content of each touched file, the sibling
// files in each touched directory, and the nearest module AGENTS.md.
func (a *Assembler) Assemble(ctx context.Context, base, head string) (Inputs, error) {
	in := Inputs{TouchedFiles: map[string]string{}, SiblingFiles: map[string]string{}}

	diff, err := a.Git(ctx, a.Root, "diff", "--unified=3", base+"..."+head)
	if err != nil {
		return in, err
	}
	in.Diff = diff

	names, err := a.Git(ctx, a.Root, "diff", "--name-only", base+"..."+head)
	if err != nil {
		return in, err
	}
	dirs := map[string]bool{}
	for _, path := range strings.Fields(names) {
		if content, ok := a.read(path); ok {
			in.TouchedFiles[path] = content
		}
		dirs[filepath.Dir(path)] = true
	}

	for dir := range dirs {
		a.addSiblings(dir, in)
	}
	in.ModuleAGENTS = a.nearestAgents(dirs)
	return in, nil
}

// addSiblings adds the other source files in a touched directory as the style
// baseline, skipping the touched files themselves and generated code.
func (a *Assembler) addSiblings(dir string, in Inputs) {
	entries, err := os.ReadDir(filepath.Join(a.Root, dir))
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if _, touched := in.TouchedFiles[path]; touched {
			continue
		}
		if strings.HasSuffix(e.Name(), "_gen.go") || !isSource(e.Name()) {
			continue
		}
		if content, ok := a.read(path); ok {
			in.SiblingFiles[path] = content
		}
	}
}

// nearestAgents returns the ## Craftsmanship SECTION of the AGENTS.md closest to
// the touched dirs, walking up to the repo root.
//
// The section, not the file. The prompt writes this under the label "Module
// AGENTS.md (## Craftsmanship deltas)", and deltas is what the design means: the
// standard itself is rubric.json, versioned and fed separately by writeRules, and
// this is the per-directory layer on top of it. Passing the whole rulebook put
// ~570 lines of issue labels, make targets and module layout into every gate
// prompt under a label promising Craftsmanship deltas — payload the model has to
// read past to reach the rules, and a reason the rulebook could not be
// reorganised without changing what the gate sees.
//
// A file with no such section is not the nearest one, and the walk continues past
// it. That matters because a directory may carry rules of its own and no rubric —
// frontend/ does. Returning that file would hand the gate a delta layer that is
// really somebody else's rulebook.
//
// The order is SORTED, because dirs is a map and a diff routinely touches more
// than one directory. Ranging over it picks a random starting point, so the same
// diff could be judged with one directory's deltas on one run and another's on the
// next — a gate that disagrees with itself for no reason anybody changed.
func (a *Assembler) nearestAgents(dirs map[string]bool) string {
	ordered := make([]string, 0, len(dirs))
	for dir := range dirs {
		ordered = append(ordered, dir)
	}
	sort.Strings(ordered)

	for _, dir := range ordered {
		for d := dir; ; d = filepath.Dir(d) {
			if content, ok := a.read(filepath.Join(d, "AGENTS.md")); ok {
				if section, found := rubric.CraftsmanshipSection(content); found {
					return section
				}
			}
			if d == "." || d == "/" {
				break
			}
		}
	}
	return ""
}

func (a *Assembler) read(path string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(a.Root, path)) //nolint:gosec // G304: path comes from git diff output for files under a.Root
	if err != nil {
		return "", false
	}
	return string(b), true
}

func isSource(name string) bool {
	switch filepath.Ext(name) {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".sql", ".css":
		return true
	}
	return false
}
