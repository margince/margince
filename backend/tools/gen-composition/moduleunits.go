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

	"golang.org/x/mod/modfile"

	"github.com/margince/margince/backend/pkg/extension"
)

const (
	// instanceDirName holds the instance: core's own thin instance in a
	// checkout, a client's instance in the core tree that client put together.
	instanceDirName = "instance"
	// fetchedUnitsDir holds the versioned extensions copied out of the module
	// cache. The cache is read-only, and the frontend install writes beside a
	// unit's frontend/.
	fetchedUnitsDir = "build/extensions"
	backendModule   = "github.com/margince/margince/backend"
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

// downloadedModule is the part of `go mod download -json` this composer reads.
type downloadedModule struct {
	Path  string
	Dir   string
	Error string
}

// instanceGoMod is the go.mod that decides what a tree composes. A client's
// core tree links instance/ to the client's own folder.
func instanceGoMod(root string) string {
	return filepath.Join(root, instanceDirName, "go.mod")
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
	downloaded, err := downloadModules(filepath.Dir(instanceGoMod(root)), wanted)
	if err != nil {
		return err
	}
	for _, m := range downloaded {
		name := unitNameFromModulePath(m.Path)
		if err := extension.Name(name).Validate(); err != nil {
			return fmt.Errorf("%s: %w", m.Path, err)
		}
		dst := filepath.Join(outDir, name)
		if err := os.CopyFS(dst, os.DirFS(m.Dir)); err != nil {
			return fmt.Errorf("%s: %w", m.Path, err)
		}
		if err := dropReplaces(filepath.Join(dst, "go.mod")); err != nil {
			return fmt.Errorf("%s: %w", m.Path, err)
		}
	}
	return nil
}

// dropReplaces removes a downloaded unit's replace lines from its copy. Go
// never applies a dependency's replace, but the composed workspace would. A
// path like ../../backend points nowhere from the copy and conflicts with the
// workspace's own backend.
func dropReplaces(goModPath string) error {
	raw, err := os.ReadFile(goModPath) // #nosec G304 -- the go.mod of a copy this run made
	if err != nil {
		return err
	}
	mod, err := modfile.Parse(goModPath, raw, nil)
	if err != nil {
		return err
	}
	for _, r := range append([]*modfile.Replace(nil), mod.Replace...) {
		if err := mod.DropReplace(r.Old.Path, r.Old.Version); err != nil {
			return err
		}
	}
	out, err := mod.Format()
	if err != nil {
		return err
	}
	return os.WriteFile(goModPath, out, 0o644) // #nosec G306 G703 -- build output under build/extensions/<name>, the name checked by extension.Name
}

// downloadModules runs `go mod download` inside the instance's module, so its
// go.sum and the checksum database check every module@version it is given.
func downloadModules(instanceDir string, versions []string) ([]downloadedModule, error) {
	goBin, err := goBinary()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(goBin, append([]string{"mod", "download", "-json"}, versions...)...) // #nosec G204 -- GOROOT/bin/go; the arguments are module paths parsed from go.mod by modfile
	cmd.Dir = instanceDir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, runErr := cmd.Output()
	var modules []downloadedModule
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var m downloadedModule
		if err := dec.Decode(&m); err != nil {
			return nil, fmt.Errorf("go mod download: %w\n%s", err, stderr.String())
		}
		if m.Error != "" {
			return nil, fmt.Errorf("%s: %s", m.Path, m.Error)
		}
		modules = append(modules, m)
	}
	if runErr != nil {
		return nil, fmt.Errorf("go mod download: %w\n%s", runErr, stderr.String())
	}
	return modules, nil
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
