// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// worktree records the work tree as git trees in an index of its own, so the
// developer's index and stash stay as they were.
type worktree struct {
	root  string
	index string
}

// newWorktree starts from a copy of the real index, so git hashes only the
// files whose stat data moved.
func newWorktree(root, scratch string) (worktree, error) {
	real, err := gitOut(root, nil, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return worktree{}, err
	}
	raw, err := os.ReadFile(strings.TrimSpace(real))
	if err != nil {
		return worktree{}, err
	}
	index := filepath.Join(scratch, "index")
	return worktree{root: root, index: index}, os.WriteFile(index, raw, 0o600) // #nosec G703 -- a file in the gate's own temporary directory
}

// snapshot writes every tracked and untracked, not ignored, file as one tree.
func (w worktree) snapshot() (string, error) {
	env := []string{"GIT_INDEX_FILE=" + w.index}
	if _, err := gitOut(w.root, env, "add", "--all", "--", "."); err != nil {
		return "", err
	}
	tree, err := gitOut(w.root, env, "write-tree")
	return strings.TrimSpace(tree), err
}

// changed lists the paths whose bytes or mode differ between two trees, each
// with git's status letter: A added, D deleted, M or T changed.
func (w worktree) changed(from, to string) ([][2]string, error) {
	out, err := gitOut(w.root, nil, "diff-tree", "-r", "-z", "--no-renames", "--name-status", from, to)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	var paths [][2]string
	for i := 0; i+1 < len(fields); i += 2 {
		paths = append(paths, [2]string{fields[i], fields[i+1]})
	}
	return paths, nil
}

// passStart is the earliest time a write in a pass that starts at now carries.
// The kernel stamps a file from a clock that may lag the one Go reads. Some file
// systems keep whole seconds. So the bound is a second early.
func passStart(now time.Time) time.Time {
	return now.Add(-time.Second).Truncate(time.Second)
}

// lastPass is the snapshot after the pass that ran last, and when it started.
type lastPass struct {
	tree    string
	started time.Time
}

// keptEdits names the files restore left as they are, because someone other
// than the generators changed them while the gate ran.
type keptEdits []string

func (k keptEdits) Error() string {
	return "left as they are, changed while the generators ran:\n  " + strings.Join(k, "\n  ")
}

// restore puts back what the generators changed between tree and the last
// pass. A file that changed after the last snapshot was changed by someone
// else. So was a file last written before the last pass started, unless it is
// a generated output. A generator that writes only changed bytes rewrites a
// stale output in the first pass and leaves it in the second. The other files
// are left as they are, and the error is a keptEdits that names them.
func (w worktree) restore(tree string, last lastPass, generated func(string) bool) error {
	current, err := w.snapshot()
	if err != nil {
		return err
	}
	moved, err := w.changed(last.tree, current)
	if err != nil {
		return err
	}
	edited := map[string]bool{}
	var kept keptEdits
	for _, p := range moved {
		edited[p[1]] = true
		kept = append(kept, p[1])
	}
	paths, err := w.changed(tree, last.tree)
	if err != nil {
		return err
	}
	var checkout []string
	for _, p := range paths {
		if edited[p[1]] {
			continue
		}
		info, statErr := os.Stat(filepath.Join(w.root, p[1]))
		if statErr == nil && info.ModTime().Before(last.started) && !generated(p[1]) {
			kept = append(kept, p[1])
			continue
		}
		if p[0] == "A" {
			if err := os.Remove(filepath.Join(w.root, p[1])); err != nil {
				return err
			}
			continue
		}
		checkout = append(checkout, p[1])
	}
	if err := w.checkout(tree, checkout); err != nil {
		return err
	}
	if len(kept) > 0 {
		return kept
	}
	return nil
}

// checkout writes paths as tree holds them.
func (w worktree) checkout(tree string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	env := []string{"GIT_INDEX_FILE=" + w.index + ".restore"}
	if _, err := gitOut(w.root, env, "read-tree", tree); err != nil {
		return err
	}
	_, err := gitIn(w.root, env, strings.NewReader(strings.Join(paths, "\x00")), "checkout-index", "-f", "-z", "--stdin")
	return err
}

// gitBinary is git, found once on PATH. Every git call of a run uses this one
// binary, so a generator that writes into a PATH directory cannot swap it.
var gitBinary = sync.OnceValues(func() (string, error) {
	path, err := exec.LookPath("git")
	if err != nil {
		return "", err
	}
	return filepath.Abs(path)
})

func gitOut(root string, env []string, args ...string) (string, error) {
	return gitIn(root, env, nil, args...)
}

func gitIn(root string, env []string, stdin io.Reader, args ...string) (string, error) {
	git, err := gitBinary()
	if err != nil {
		return "", fmt.Errorf("finding git: %w", err)
	}
	cmd := exec.Command(git, append([]string{"-C", root}, args...)...) // #nosec G204 -- git; the args are this gate's own literals and git's output
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = stdin
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return string(out), nil
}
