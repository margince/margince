// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// generator is one command the gate runs once in each pass.
type generator struct {
	Name string // what a report calls it
	Dir  string // the working directory, relative to the repository root
	Argv []string
	Env  []string
}

// driftMakefile holds the drift gate, whose $(MAKE) line names what it
// regenerates before it diffs.
const driftMakefile = "backend/Makefile"

// makeGenerator is the regeneration the drift gate runs, and the targets it
// names.
func makeGenerator(makefile string) (generator, []string, error) {
	inDrift := false
	for line := range strings.SplitSeq(makefile, "\n") {
		switch {
		case strings.HasPrefix(line, "drift:"):
			inDrift = true
		case inDrift && !strings.HasPrefix(line, "\t"):
			inDrift = false
		case inDrift && strings.HasPrefix(strings.TrimSpace(line), "$(MAKE) "):
			targets := strings.Fields(line)[1:]
			return generator{
				Name: "make -C backend " + strings.Join(targets, " "),
				Dir:  "backend", Argv: append([]string{"make"}, targets...),
			}, targets, nil
		}
	}
	return generator{}, nil, fmt.Errorf("%s: the drift recipe has no $(MAKE) line, so nothing says what it regenerates", driftMakefile)
}

// frontendPackage is the package whose gen:* scripts write committed files.
const frontendPackage = "frontend/package.json"

// scriptCall is one script naming another, which then runs inside it.
var scriptCall = regexp.MustCompile(`pnpm (?:run )?(gen:[\w:-]+)`)

// frontendGenerators runs each gen:* script that no other gen:* script runs.
func frontendGenerators(root string) ([]generator, error) {
	raw, err := os.ReadFile(filepath.Join(root, frontendPackage)) // #nosec G304 -- a fixed path under the repository root
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("%s: %w", frontendPackage, err)
	}
	called := map[string]bool{}
	for name, script := range manifest.Scripts {
		if strings.HasPrefix(name, "gen:") {
			for _, match := range scriptCall.FindAllStringSubmatch(script, -1) {
				called[match[1]] = true
			}
		}
	}
	var generators []generator
	for _, name := range slices.Sorted(maps.Keys(manifest.Scripts)) {
		if strings.HasPrefix(name, "gen:") && !called[name] {
			generators = append(generators, generator{
				Name: "pnpm run " + name, Dir: "frontend", Argv: []string{"pnpm", "run", "--silent", name},
			})
		}
	}
	if len(generators) == 0 {
		return nil, fmt.Errorf("%s has no gen:* script", frontendPackage)
	}
	return generators, nil
}

// goTestGenerator runs one package's golden tests in their rewrite mode, from
// the module that holds the package, under the root workspace.
func goTestGenerator(root string, g goGenerator) (generator, error) {
	module, err := moduleOf(root, g.Dir)
	if err != nil {
		return generator{}, err
	}
	rel, err := filepath.Rel(module, g.Dir)
	if err != nil {
		return generator{}, err
	}
	pkg := "./" + filepath.ToSlash(rel)
	argv := append([]string{"go", "test", "-count=1", "-run", "^(" + strings.Join(g.Tests, "|") + ")$", pkg}, g.Flags...)
	return generator{
		Name: "go test " + pkg + " " + strings.Join(append(slices.Clone(g.Flags), g.Env...), " "),
		Dir:  module, Argv: argv,
		Env: append([]string{"GOWORK=" + filepath.Join(root, "go.work")}, g.Env...),
	}, nil
}

// moduleOf is the nearest directory at or above dir that holds a go.mod.
func moduleOf(root, dir string) (string, error) {
	for d := dir; ; d = filepath.Dir(d) {
		_, err := os.Stat(filepath.Join(root, d, "go.mod"))
		if err == nil {
			return d, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if d == "." || d == "/" {
			return "", fmt.Errorf("%s is in no Go module", dir)
		}
	}
}
