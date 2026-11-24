// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
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
	Dependencies map[string]lockedSpecifier `yaml:"dependencies"`
}

// lockfileDocument is one YAML document of pnpm-lock.yaml. pnpm 12 writes the
// package-manager entry as a first document and the project lock after it.
type lockfileDocument struct {
	Importers map[string]lockedImporter `yaml:"importers"`
}

// readCoreRuntimeDeps reads what the core SPA ships, from the repository
// root's lockfile only. A lockfile inside an extension is never read: the
// unit under check would be vouching for its own packages.
func readCoreRuntimeDeps(root string) (map[string]lockedDep, error) {
	path := filepath.Join(root, lockfileName)
	raw, err := os.ReadFile(path) // #nosec G304 -- the lockfile of the tree this composer was pointed at
	if err != nil {
		return nil, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	out := map[string]lockedDep{}
	for {
		var doc lockfileDocument
		if err := decoder.Decode(&doc); errors.Is(err, io.EOF) {
			return out, nil
		} else if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for name, locked := range doc.Importers[coreImporter].Dependencies {
			// pnpm suffixes a resolved version with its peer context, "1.2.3(react@19)".
			version, _, _ := strings.Cut(locked.Version, "(")
			out[name] = lockedDep{Specifier: locked.Specifier, Version: version}
		}
	}
}

// pinUnitRuntimeDeps refuses a runtime or peer dependency the main app does
// not ship, and records the locked version of each accepted one on the unit.
// No licence check exists for a package only an extension brings.
//
// A range is accepted when it is the core's specifier or the locked version.
// The locked version satisfies both without a semver evaluator.
func pinUnitRuntimeDeps(root string, unit *extensionUnit) error {
	frontend := unit.Frontend
	if frontend == nil || len(frontend.RuntimeDeps) == 0 {
		return nil
	}
	core, err := readCoreRuntimeDeps(root)
	if err != nil {
		return fmt.Errorf("extensions/%s: %s/package.json declares runtime packages, and the core lockfile they are checked against cannot be read: %w", unit.Name, frontendLayer, err)
	}
	names := make([]string, 0, len(frontend.RuntimeDeps))
	for dep := range frontend.RuntimeDeps {
		names = append(names, dep)
	}
	sort.Strings(names)
	pins := make(map[string]string, len(names))
	for _, dep := range names {
		declared := frontend.RuntimeDeps[dep]
		locked, ok := core[dep]
		switch {
		case !ok:
			return fmt.Errorf("extensions/%s: %s/package.json needs %s, which the main app does not use. A licence check for an extension's own packages does not exist yet, so a unit may only use packages the main app already ships (dependencies of frontend/ in %s). Use one of those, or move it to devDependencies if it is not needed at run time",
				unit.Name, frontendLayer, dep, lockfileName)
		case declared != locked.Specifier && declared != locked.Version:
			return fmt.Errorf("extensions/%s: %s/package.json needs %s at %q, but the main app locks %s (declared as %q). Declare one of those two, so the unit gets the version the main app's licence check covers",
				unit.Name, frontendLayer, dep, declared, locked.Version, locked.Specifier)
		}
		pins[dep] = locked.Version
	}
	frontend.Pins = pins
	return nil
}
