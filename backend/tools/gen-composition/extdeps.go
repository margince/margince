// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// coreImporter is the lockfile entry of the core SPA, the workspace member
// whose runtime dependencies the main app ships.
const coreImporter = "frontend"

const lockfileName = "pnpm-lock.yaml"

type lockedSpecifier struct {
	Specifier string `yaml:"specifier"`
	Version   string `yaml:"version"`
}

// lockedDep is a core runtime dependency as the lockfile resolved it.
type lockedDep struct {
	Specifier string
	Version   string
}

type lockedImporter struct {
	Dependencies         map[string]lockedSpecifier `yaml:"dependencies"`
	OptionalDependencies map[string]lockedSpecifier `yaml:"optionalDependencies"`
}

// runtimeSpecifiers is what the core SPA ships: its runtime and optional
// dependencies, never its devDependencies, which no browser bundle carries.
func (i lockedImporter) runtimeSpecifiers() map[string]lockedDep {
	out := map[string]lockedDep{}
	for _, group := range []map[string]lockedSpecifier{i.Dependencies, i.OptionalDependencies} {
		for name, locked := range group {
			// pnpm suffixes a resolved version with its peer context, "1.2.3(react@19)".
			version, _, _ := strings.Cut(locked.Version, "(")
			out[name] = lockedDep{Specifier: locked.Specifier, Version: version}
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

func readCoreRuntimeSpecifiers(dir string) (map[string]lockedDep, error) {
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
// already ship, and returns the locked version of each one it accepts. No
// licence check exists for a package only an extension brings, so only
// packages the core lockfile already vetted may enter a build.
//
// A range is accepted when it is the core's own specifier or the locked
// version itself, both of which that version satisfies without a semver
// evaluator. The composed install is not frozen, so a range alone could
// resolve to a newer, unvetted release: the returned versions are what
// emitComposedFrontendWorkspace pins with overrides.
func checkUnitRuntimeDeps(name, dir string, pkg unitPackageJSON) (map[string]string, error) {
	declared := map[string]string{}
	for dep, rng := range pkg.OptionalDependencies {
		declared[dep] = rng
	}
	for dep, rng := range pkg.Dependencies {
		declared[dep] = rng
	}
	if len(declared) == 0 {
		return nil, nil
	}
	core, err := readCoreRuntimeSpecifiers(dir)
	if err != nil {
		return nil, fmt.Errorf("extensions/%s: %s/package.json declares runtime dependencies, and the core lockfile they are checked against cannot be read: %w", name, frontendLayer, err)
	}
	names := make([]string, 0, len(declared))
	for dep := range declared {
		names = append(names, dep)
	}
	sort.Strings(names)
	pins := make(map[string]string, len(names))
	for _, dep := range names {
		locked, ok := core[dep]
		switch {
		case !ok:
			return nil, fmt.Errorf("extensions/%s: %s/package.json depends on %s, which the main app does not use. A licence check for an extension's own packages does not exist yet, so a unit may only depend on a package the main app already ships (a dependency of frontend/ in %s). Use one of those, or declare it as a peerDependency or devDependency if it is not needed at run time",
				name, frontendLayer, dep, lockfileName)
		case declared[dep] != locked.Specifier && declared[dep] != locked.Version:
			return nil, fmt.Errorf("extensions/%s: %s/package.json depends on %s at %q, but the main app locks %s (declared as %q). Declare one of those two, so the unit gets the version the main app's licence check covers",
				name, frontendLayer, dep, declared[dep], locked.Version, locked.Specifier)
		}
		pins[dep] = locked.Version
	}
	return pins, nil
}
