// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// coreImporter is the lockfile entry of the core SPA, the workspace member
// whose runtime dependencies the main app ships.
const coreImporter = "frontend"

const lockfileName = "pnpm-lock.yaml"

type lockedSpecifier struct {
	Specifier string `yaml:"specifier"`
}

type lockedImporter struct {
	Dependencies         map[string]lockedSpecifier `yaml:"dependencies"`
	OptionalDependencies map[string]lockedSpecifier `yaml:"optionalDependencies"`
}

// runtimeSpecifiers is what the core SPA ships: its runtime and optional
// dependencies, never its devDependencies, which no browser bundle carries.
func (i lockedImporter) runtimeSpecifiers() map[string]string {
	out := map[string]string{}
	for _, group := range []map[string]lockedSpecifier{i.Dependencies, i.OptionalDependencies} {
		for name, locked := range group {
			out[name] = locked.Specifier
		}
	}
	return out
}

// findLockfile walks up from the unit directory to the repository's lockfile.
func findLockfile(dir string) (string, error) {
	for cur := dir; ; {
		candidate := filepath.Join(cur, lockfileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("no %s found above %s", lockfileName, dir)
		}
		cur = parent
	}
}

func readCoreRuntimeSpecifiers(dir string) (map[string]string, error) {
	path, err := findLockfile(dir)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- the lockfile of the tree this composer was pointed at
	if err != nil {
		return nil, err
	}
	var lock struct {
		Importers map[string]lockedImporter `yaml:"importers"`
	}
	if err := yaml.Unmarshal(raw, &lock); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return lock.Importers[coreImporter].runtimeSpecifiers(), nil
}

// checkUnitRuntimeDeps refuses a runtime dependency the main app does not
// already ship at the same range. No licence check exists for a package only
// an extension brings, so the only packages that may enter a build are ones
// the core lockfile already vetted. The range must match the core's specifier
// exactly: pnpm then resolves the unit's entry to the version already locked,
// and no semver evaluator is needed to say so.
func checkUnitRuntimeDeps(name, dir string, pkg unitPackageJSON) error {
	declared := map[string]string{}
	for dep, rng := range pkg.OptionalDependencies {
		declared[dep] = rng
	}
	for dep, rng := range pkg.Dependencies {
		declared[dep] = rng
	}
	if len(declared) == 0 {
		return nil
	}
	core, err := readCoreRuntimeSpecifiers(dir)
	if err != nil {
		return fmt.Errorf("extensions/%s: %s/package.json declares runtime dependencies, and the core lockfile they are checked against cannot be read: %w", name, frontendLayer, err)
	}
	names := make([]string, 0, len(declared))
	for dep := range declared {
		names = append(names, dep)
	}
	sort.Strings(names)
	for _, dep := range names {
		want, locked := core[dep]
		switch {
		case !locked:
			return fmt.Errorf("extensions/%s: %s/package.json depends on %s, which the main app does not use. A licence check for an extension's own packages does not exist yet, so a unit may only depend on a package the main app already ships (a dependency of frontend/ in %s). Use one of those, or declare it as a peerDependency or devDependency if it is not needed at run time",
				name, frontendLayer, dep, lockfileName)
		case want != declared[dep]:
			return fmt.Errorf("extensions/%s: %s/package.json depends on %s at %q, but the main app uses it at %q. Declare the same range, so the unit gets the version already locked, which the main app's licence check covers",
				name, frontendLayer, dep, declared[dep], want)
		}
	}
	return nil
}
