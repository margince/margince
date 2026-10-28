// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const (
	// moduleUnitsDir holds the units copied out of the module cache. The cache
	// is read-only, and the frontend install writes beside a unit's frontend/.
	moduleUnitsDir = "build/extensions"
	// moduleUnitsList names every unit an installation turns on, one
	// "<name>\t<absolute dir>" per line, so -verify reads the set generate used.
	moduleUnitsList = "build/extensions.list"
	backendModule   = "github.com/margince/margince/backend"
)

var majorVersionSuffix = regexp.MustCompile(`^v[0-9]+$`)

// moduleUnit is one unit an installation turns on.
type moduleUnit struct {
	name string
	dir  string
}

// listedPackage is the part of `go list -find -json` this composer reads.
// Module.Dir is where the go command resolved the module, any replace applied.
type listedPackage struct {
	ImportPath string
	Module     *struct {
		Path    string
		Version string
		Dir     string
		Replace *struct {
			Version string
		}
	}
	Error *struct {
		Err string
	}
}

// resolveModuleUnits turns on every extension an installation's own Go module
// imports. The go command resolves each one, so go.sum and the checksum
// database verify a download. A replace onto a folder is composed where it
// sits. With no installation named, only extensions/ is composed.
func resolveModuleUnits(root, instanceDir string) error {
	outDir := filepath.Join(root, filepath.FromSlash(moduleUnitsDir))
	listPath := filepath.Join(root, filepath.FromSlash(moduleUnitsList))
	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	if err := os.Remove(listPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if instanceDir == "" {
		return nil
	}
	imports, err := blankImports(instanceDir)
	if err != nil || len(imports) == 0 {
		return err
	}
	pkgs, err := listPackages(instanceDir, imports)
	if err != nil {
		return err
	}
	units := make([]moduleUnit, 0, len(pkgs))
	for _, p := range pkgs {
		m := p.Module
		if m == nil || m.Path != p.ImportPath {
			return fmt.Errorf("%s: import the extension's module root, the package that declares New()", p.ImportPath)
		}
		unit := moduleUnit{name: unitNameFromModulePath(m.Path), dir: m.Dir}
		if m.Replace == nil || m.Replace.Version != "" {
			unit.dir = filepath.Join(outDir, unit.name)
			if err := copyModule(m.Dir, unit.dir); err != nil {
				return fmt.Errorf("%s@%s: %w", m.Path, m.Version, err)
			}
		}
		units = append(units, unit)
	}
	return writeModuleUnits(listPath, units)
}

// blankImports lists the blank imports of the installation's root package:
// `import _ "<module>"` turns an extension on, the way a program turns on a
// database/sql driver. Core's own packages are left out, because the
// installation imports those to build core's binaries.
func blankImports(instanceDir string) ([]string, error) {
	pkgs, err := parseDirByPackage(token.NewFileSet(), instanceDir, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var paths []string
	for _, files := range pkgs {
		for _, f := range files {
			for _, imp := range f.Imports {
				p, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					return nil, err
				}
				if imp.Name == nil || imp.Name.Name != "_" || isCorePackage(p) || seen[p] {
					continue
				}
				seen[p] = true
				paths = append(paths, p)
			}
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func isCorePackage(importPath string) bool {
	return importPath == compositionModule || importPath == backendModule || strings.HasPrefix(importPath, backendModule+"/")
}

// listPackages asks the go command, inside the installation's module, where
// each imported package is. It downloads what the cache lacks.
func listPackages(instanceDir string, imports []string) ([]listedPackage, error) {
	goBin, err := goBinary()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(goBin, append([]string{"list", "-find", "-json"}, imports...)...) // #nosec G204 -- GOROOT/bin/go; the arguments are import paths parsed from Go source
	cmd.Dir = instanceDir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, runErr := cmd.Output()
	var pkgs []listedPackage
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var p listedPackage
		if err := dec.Decode(&p); err != nil {
			return nil, err
		}
		if p.Error != nil {
			return nil, fmt.Errorf("%s: %s", p.ImportPath, p.Error.Err)
		}
		pkgs = append(pkgs, p)
	}
	if runErr != nil {
		return nil, fmt.Errorf("go list: %w\n%s", runErr, stderr.String())
	}
	return pkgs, nil
}

// unitNameFromModulePath takes the unit name from the module path's last
// element, skipping a major-version suffix (…/crm/v2 is the unit crm).
func unitNameFromModulePath(modulePath string) string {
	base := path.Base(modulePath)
	if majorVersionSuffix.MatchString(base) {
		return path.Base(path.Dir(modulePath))
	}
	return base
}

// copyModule copies a module out of the read-only cache. os.CopyFS refuses
// anything but regular files and folders, and a module zip holds nothing else.
func copyModule(src, dst string) error {
	return os.CopyFS(dst, os.DirFS(src))
}

// goBinary is the running toolchain's go: a writable PATH entry must not choose
// which go resolves the composed graph.
func goBinary() (string, error) {
	//nolint:staticcheck // SA1019: the replacement, `go env GOROOT`, resolves through PATH
	goRoot := runtime.GOROOT()
	if goRoot == "" {
		return "", errors.New("cannot locate the go toolchain (empty GOROOT)")
	}
	return filepath.Join(goRoot, "bin", "go"), nil
}

func writeModuleUnits(listPath string, units []moduleUnit) error {
	if len(units) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(listPath), 0o755); err != nil { // #nosec G301 -- build output stays readable to another UID
		return err
	}
	var b strings.Builder
	for _, u := range units {
		fmt.Fprintf(&b, "%s\t%s\n", u.name, u.dir)
	}
	return os.WriteFile(listPath, []byte(b.String()), 0o644) // #nosec G306 -- build output
}

func readModuleUnits(root string) ([]moduleUnit, error) {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(moduleUnitsList))) // #nosec G304 -- build output at a fixed path
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var units []moduleUnit
	for line := range strings.SplitSeq(strings.TrimSpace(string(raw)), "\n") {
		name, dir, ok := strings.Cut(line, "\t")
		if !ok {
			return nil, fmt.Errorf("%s: malformed line %q, run make composition again", moduleUnitsList, line)
		}
		units = append(units, moduleUnit{name: name, dir: dir})
	}
	return units, nil
}
