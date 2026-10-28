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

	"github.com/margince/margince/backend/pkg/extension"
)

const (
	// copiedUnitsList names each unit the last run copied into extensions/. The
	// next run removes those, and never a unit that belongs to the tree.
	copiedUnitsList = "build/extensions.list"
	backendModule   = "github.com/margince/margince/backend"
)

var majorVersionSuffix = regexp.MustCompile(`^v[0-9]+$`)

// skippedUnitDirs are folders a unit's module zip never holds, or that an
// installation fills locally: copying them would bring in a stale install.
var skippedUnitDirs = map[string]bool{".git": true, ".hg": true, ".svn": true, ".bzr": true, "node_modules": true}

// listedModule is the part of `go list -m -json` and `go mod download -json`
// this composer reads. Dir is where the go command resolved the module, any
// replace applied.
type listedModule struct {
	Path    string
	Version string
	Dir     string
	Error   *struct {
		Err string
	}
}

// resolveModuleUnits copies every extension the installation's go.mod requires
// into extensions/<name>/, where presence turns a unit on as for any other.
// The go command resolves each one, so go.sum and the checksum database verify
// a download, and a replace onto a folder copies that folder.
func resolveModuleUnits(root, instanceDir string) error {
	if err := removeCopiedUnits(root); err != nil {
		return err
	}
	if instanceDir == "" {
		return nil
	}
	required, err := requiredExtensions(filepath.Join(instanceDir, "go.mod"))
	if err != nil || len(required) == 0 {
		return err
	}
	modules, err := locateModules(instanceDir, required)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(modules))
	for _, m := range modules {
		name := unitNameFromModulePath(m.Path)
		dst := filepath.Join(root, "extensions", name)
		if _, err := os.Stat(dst); err == nil {
			return fmt.Errorf("%s: extensions/%s already exists, so two units would share one name; remove one", m.Path, name)
		}
		if err := copyModule(m.Dir, dst); err != nil {
			return fmt.Errorf("%s@%s: %w", m.Path, m.Version, err)
		}
		names = append(names, name)
	}
	return writeCopiedUnits(root, names)
}

// requiredExtensions lists what the installation's go.mod requires itself:
// each such module is an extension, except core's backend and the composition
// this run writes. A `// indirect` line is a dependency, never an extension.
func requiredExtensions(goModPath string) ([]string, error) {
	raw, err := os.ReadFile(goModPath) // #nosec G304 -- the installation's own go.mod, named by the operator
	if err != nil {
		return nil, err
	}
	mod, err := modfile.ParseLax(goModPath, raw, nil)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, r := range mod.Require {
		if r.Indirect || r.Mod.Path == backendModule || r.Mod.Path == compositionModule {
			continue
		}
		paths = append(paths, r.Mod.Path)
	}
	return paths, nil
}

// locateModules asks the go command, inside the installation's module, where
// each module is, and downloads the ones the cache lacks. Like `go get`, the
// download records the checked sums in the installation's go.sum.
func locateModules(instanceDir string, paths []string) ([]listedModule, error) {
	listed, err := goModules(instanceDir, append([]string{"list", "-mod=mod", "-m", "-json"}, paths...))
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, m := range listed {
		if m.Dir == "" {
			missing = append(missing, m.Path)
		}
	}
	if len(missing) == 0 {
		return listed, nil
	}
	downloaded, err := goModules(instanceDir, append([]string{"mod", "download", "-json"}, missing...))
	if err != nil {
		return nil, err
	}
	dirs := make(map[string]string, len(downloaded))
	for _, d := range downloaded {
		dirs[d.Path] = d.Dir
	}
	for i := range listed {
		if listed[i].Dir == "" {
			listed[i].Dir = dirs[listed[i].Path]
		}
	}
	return listed, nil
}

func goModules(instanceDir string, args []string) ([]listedModule, error) {
	goBin, err := goBinary()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(goBin, args...) // #nosec G204 -- GOROOT/bin/go; the arguments are module paths parsed from go.mod by modfile
	cmd.Dir = instanceDir
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
		if m.Error != nil {
			return nil, fmt.Errorf("%s: %s", m.Path, m.Error.Err)
		}
		modules = append(modules, m)
	}
	if runErr != nil {
		return nil, fmt.Errorf("go %s: %w\n%s", args[0], runErr, stderr.String())
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

// copyModule copies the files Go puts in a module's zip, leaving out
// skippedUnitDirs, nested modules and links. Both ends are opened as roots,
// so no path in the source can lead the copy outside either folder.
func copyModule(src, dst string) (err error) {
	if err := os.MkdirAll(dst, 0o755); err != nil { // #nosec G301 -- build output stays readable to another UID
		return err
	}
	from, err := os.OpenRoot(src)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, from.Close()) }()
	to, err := os.OpenRoot(dst)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, to.Close()) }()
	return fs.WalkDir(from.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p == "." {
				return nil
			}
			if _, statErr := from.Stat(path.Join(p, "go.mod")); skippedUnitDirs[d.Name()] || statErr == nil {
				return fs.SkipDir
			}
			return to.Mkdir(p, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		content, err := from.ReadFile(p)
		if err != nil {
			return err
		}
		return to.WriteFile(p, content, 0o644)
	})
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

func writeCopiedUnits(root string, names []string) error {
	if len(names) == 0 {
		return nil
	}
	listPath := filepath.Join(root, filepath.FromSlash(copiedUnitsList))
	if err := os.MkdirAll(filepath.Dir(listPath), 0o755); err != nil { // #nosec G301 -- build output stays readable to another UID
		return err
	}
	return os.WriteFile(listPath, []byte(strings.Join(names, "\n")+"\n"), 0o644) // #nosec G306 -- build output
}

// removeCopiedUnits removes what the last run copied, so an import the
// installation dropped no longer turns its unit on.
func removeCopiedUnits(root string) error {
	listPath := filepath.Join(root, filepath.FromSlash(copiedUnitsList))
	raw, err := os.ReadFile(listPath) // #nosec G304 -- build output at a fixed path
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for name := range strings.FieldsSeq(string(raw)) {
		if err := extension.Name(name).Validate(); err != nil {
			return fmt.Errorf("%s: %w, run make composition again", copiedUnitsList, err)
		}
		if err := os.RemoveAll(filepath.Join(root, "extensions", name)); err != nil {
			return err
		}
	}
	return os.Remove(listPath)
}
