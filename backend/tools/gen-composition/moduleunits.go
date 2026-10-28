// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"golang.org/x/mod/modfile"
)

const (
	// defaultInstanceDir is core's own thin instance, the go.mod a checkout
	// composes. A client instance is the same file with real versions.
	defaultInstanceDir = "instance"
	// fetchedUnitsDir holds the versioned extensions copied out of the module
	// cache. The cache is read-only, and the frontend install writes beside a
	// unit's frontend/.
	fetchedUnitsDir = "build/extensions"
	// unitDirsList names the folder of every composed unit, one per line, for
	// the lanes outside this tool that walk the units.
	unitDirsList  = "build/extensions.list"
	backendModule = "github.com/margince/margince/backend"
)

var majorVersionSuffix = regexp.MustCompile(`^v[0-9]+$`)

// unitSource is one extension the instance requires, and where its code is.
type unitSource struct {
	name       string
	modulePath string
	version    string
	dir        string
	fetched    bool
}

// listedModule is the part of `go mod download -json` this composer reads.
type listedModule struct {
	Path  string
	Dir   string
	Error string
}

func instanceGoMod(root string) string {
	dir := *instanceDir
	if dir == "" {
		dir = filepath.Join(root, defaultInstanceDir)
	}
	return filepath.Join(dir, "go.mod")
}

// instanceUnits lists the extensions to compose: every module the instance's
// go.mod requires, except core's backend and composition and the `// indirect`
// dependencies. A replace onto a folder is composed where it sits; any other
// requirement is read from its copy under fetchedUnitsDir.
func instanceUnits(root string) ([]unitSource, error) {
	goModPath := instanceGoMod(root)
	raw, err := os.ReadFile(goModPath) // #nosec G304 -- the instance's go.mod, named by the operator or core's own
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	mod, err := modfile.Parse(goModPath, raw, nil)
	if err != nil {
		return nil, err
	}
	local := map[string]string{}
	for _, r := range mod.Replace {
		if r.New.Version != "" {
			continue
		}
		dir := r.New.Path
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(filepath.Dir(goModPath), dir)
		}
		local[r.Old.Path] = filepath.Clean(dir)
	}
	seen := map[string]string{}
	var units []unitSource
	for _, r := range mod.Require {
		if r.Indirect || r.Mod.Path == backendModule || r.Mod.Path == compositionModule {
			continue
		}
		name := unitNameFromModulePath(r.Mod.Path)
		if other, dup := seen[name]; dup {
			return nil, fmt.Errorf("%s: %s and %s are both the unit %q; one name, one unit", goModPath, other, r.Mod.Path, name)
		}
		seen[name] = r.Mod.Path
		u := unitSource{name: name, modulePath: r.Mod.Path, version: r.Mod.Version, dir: local[r.Mod.Path]}
		if u.dir == "" {
			u.dir, u.fetched = filepath.Join(root, filepath.FromSlash(fetchedUnitsDir), name), true
		}
		units = append(units, u)
	}
	return units, nil
}

// fetchUnits copies every versioned extension out of the module cache. The
// download runs inside the instance's module, so its go.sum and the checksum
// database verify each one.
func fetchUnits(root string) error {
	outDir := filepath.Join(root, filepath.FromSlash(fetchedUnitsDir))
	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	units, err := instanceUnits(root)
	if err != nil {
		return err
	}
	var wanted []string
	for _, u := range units {
		if u.fetched {
			wanted = append(wanted, u.modulePath+"@"+u.version)
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	downloaded, err := goModules(filepath.Dir(instanceGoMod(root)), append([]string{"mod", "download", "-json"}, wanted...))
	if err != nil {
		return err
	}
	for _, m := range downloaded {
		if err := os.CopyFS(filepath.Join(outDir, unitNameFromModulePath(m.Path)), os.DirFS(m.Dir)); err != nil {
			return fmt.Errorf("%s: %w", m.Path, err)
		}
	}
	return nil
}

func goModules(dir string, args []string) ([]listedModule, error) {
	goBin, err := goBinary()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(goBin, args...) // #nosec G204 -- GOROOT/bin/go; the arguments are module paths parsed from go.mod by modfile
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, runErr := cmd.Output()
	var modules []listedModule
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var m listedModule
		if err := dec.Decode(&m); err != nil {
			return nil, err
		}
		if m.Error != "" {
			return nil, fmt.Errorf("%s: %s", m.Path, m.Error)
		}
		modules = append(modules, m)
	}
	if runErr != nil {
		return nil, fmt.Errorf("go %s: %w\n%s", args[0], runErr, stderr.String())
	}
	return modules, nil
}

func writeUnitDirs(root string, units []extensionUnit) error {
	var b strings.Builder
	for _, u := range units {
		b.WriteString(u.Dir + "\n")
	}
	listPath := filepath.Join(root, filepath.FromSlash(unitDirsList))
	if err := os.MkdirAll(filepath.Dir(listPath), 0o755); err != nil { // #nosec G301 -- build output stays readable to another UID
		return err
	}
	return os.WriteFile(listPath, []byte(b.String()), 0o644) // #nosec G306 -- build output
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
