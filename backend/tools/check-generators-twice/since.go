// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"path"
	"slices"
	"strings"
)

// generatorInputs are the trees every generator is built from or steered by:
// the tools, the contract they read, and the recipes and scripts that run them.
var generatorInputs = []string{
	"Makefile", "backend/Makefile", "backend/api/", "backend/tools/",
	"frontend/package.json", "frontend/pnpm-lock.yaml", "frontend/scripts/", "scripts/",
}

// dependencyFiles pin the libraries a generator links, whose upgrade can
// change the order it writes in.
var dependencyFiles = []string{"go.mod", "go.sum", "go.work", "go.work.sum"}

// touchesGenerator names the first changed path that can change what a pass
// writes. A branch with none runs the generators its base ran, on the inputs
// its base gave them, so two passes would repeat the base's verdict.
func (p plan) touchesGenerator(changed []string) (string, bool) {
	for _, file := range changed {
		if slices.Contains(dependencyFiles, path.Base(file)) || p.generated(file) ||
			slices.Contains(p.goldenDirs, path.Dir(file)) {
			return file, true
		}
		for _, input := range generatorInputs {
			if file == input || (strings.HasSuffix(input, "/") && strings.HasPrefix(file, input)) {
				return file, true
			}
		}
	}
	return "", false
}

// changedSince lists the paths that differ from the merge base with ref:
// committed, uncommitted and untracked alike.
func changedSince(root, ref string) ([]string, error) {
	fork, err := gitOut(root, nil, "merge-base", ref, "HEAD")
	if err != nil {
		return nil, err
	}
	tracked, err := gitOut(root, nil, "diff", "--name-only", "-z", strings.TrimSpace(fork))
	if err != nil {
		return nil, err
	}
	untracked, err := gitOut(root, nil, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	var changed []string
	for _, name := range strings.Split(tracked+untracked, "\x00") {
		if name != "" {
			changed = append(changed, name)
		}
	}
	return changed, nil
}
