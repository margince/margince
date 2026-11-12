// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// restore puts the work tree back to tree: a changed or deleted file gets its
// old bytes, and a file the generators added is removed.
func (w worktree) restore(tree string) error {
	current, err := w.snapshot()
	if err != nil {
		return err
	}
	paths, err := w.changed(tree, current)
	if err != nil {
		return err
	}
	var checkout []string
	for _, p := range paths {
		if p[0] == "A" {
			if err := os.Remove(filepath.Join(w.root, p[1])); err != nil {
				return err
			}
			continue
		}
		checkout = append(checkout, p[1])
	}
	if len(checkout) == 0 {
		return nil
	}
	env := []string{"GIT_INDEX_FILE=" + w.index + ".restore"}
	if _, err := gitOut(w.root, env, "read-tree", tree); err != nil {
		return err
	}
	cmd := exec.Command("git", "-C", w.root, "checkout-index", "-f", "-z", "--stdin") // #nosec G204 -- git with literal args and the repository root
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(strings.Join(checkout, "\x00"))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout-index: %w: %s", err, out)
	}
	return nil
}

func gitOut(root string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...) // #nosec G204 -- git; the args are this gate's own literals and git's output
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return string(out), nil
}
