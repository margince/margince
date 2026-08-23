package gate

import (
	"context"
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

// nearestAgents returns the ## Craftsmanship-bearing AGENTS.md closest to the
// touched dirs, walking up to the repo root.
//
// Two properties this has to have, and neither is free:
//
// The heading is a REQUIREMENT, not a description. A directory may hold an
// AGENTS.md that carries its own rules and no Craftsmanship section — frontend/
// does. Returning that one would hand the gate a prompt with no rubric in it at
// all, and the gate would still answer: a verdict reached without the rules,
// reported exactly like one reached with them. So a file that does not carry the
// section is not the nearest one; the walk continues past it.
//
// And the order is SORTED, because dirs is a map and a diff routinely touches
// more than one. Ranging over it directly picks a random starting directory, so
// the same diff could be judged against one rulebook on one run and another on
// the next — a gate that disagrees with itself for no reason anybody changed.
func (a *Assembler) nearestAgents(dirs map[string]bool) string {
	ordered := make([]string, 0, len(dirs))
	for dir := range dirs {
		ordered = append(ordered, dir)
	}
	sort.Strings(ordered)

	for _, dir := range ordered {
		for d := dir; ; d = filepath.Dir(d) {
			content, ok := a.read(filepath.Join(d, "AGENTS.md"))
			if ok && strings.Contains(content, craftsmanshipHeading) {
				return content
			}
			if d == "." || d == "/" {
				break
			}
		}
	}
	return ""
}

// craftsmanshipHeading is the section that makes an AGENTS.md usable as a gate
// prompt. `make check-craft-doc` asserts the root rulebook still carries it.
const craftsmanshipHeading = "## Craftsmanship\n"

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
