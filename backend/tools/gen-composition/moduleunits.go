// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/mod/modfile"

	"github.com/margince/margince/backend/internal/platform/unitset"
	"github.com/margince/margince/backend/pkg/extension"
)

const (
	// instanceDirName holds the instance: core's own thin instance in a
	// checkout, a client's instance in the core tree that client put together.
	instanceDirName = "instance"
	// fetchedUnitsDir holds the versioned extensions copied out of the module
	// cache. The cache is read-only, and the frontend install writes beside a
	// unit's frontend/.
	fetchedUnitsDir = unitset.FetchedDir
	// fetchedUnitsIndex lists each copied unit as `name | module@version`.
	// The frontend notice plugin and scripts/backend-notices.sh read it to
	// find third-party source that sits inside the repository tree.
	fetchedUnitsIndex = fetchedUnitsDir + "/fetched-units.txt"
	backendModule     = unitset.BackendModule
)

// downloadedModule is the part of `go mod download -json` this composer reads.
type downloadedModule struct {
	Path   string
	Dir    string
	Error  string
	Origin *struct{ Hash string }
}

// instanceGoMod is the go.mod that decides what a tree composes. A client's
// core tree links instance/ to the client's own folder.
func instanceGoMod(root string) string {
	if instanceGoModOverride != "" {
		return instanceGoModOverride
	}
	return filepath.Join(root, instanceDirName, "go.mod")
}

// instanceUnits lists every extension the instance's go.mod requires; see
// unitset.Read.
func instanceUnits(root string) ([]unitset.Unit, error) {
	return unitset.Read(root, instanceGoMod(root))
}

// fetchUnits copies every versioned extension out of the module cache. The
// download runs inside the instance's module, so its go.sum and the checksum
// database verify each one.
func fetchUnits(root string) error {
	outDir := filepath.Join(root, filepath.FromSlash(fetchedUnitsDir))
	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	units, err := selectedInstanceUnits(root)
	if err != nil {
		return err
	}
	var wanted []string
	unitOf, downloadOf := map[string]string{}, map[string]string{}
	for _, u := range units {
		if u.Fetched {
			wanted = append(wanted, u.Download)
			modPath, _, _ := strings.Cut(u.Download, "@")
			unitOf[modPath] = u.Name
			downloadOf[modPath] = u.Download
		}
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil { // #nosec G301 -- build output the image copies, like the units in it
		return err
	}
	if len(wanted) == 0 {
		return writeFetchedIndex(root, "")
	}
	downloaded, err := downloadModules(filepath.Dir(instanceGoMod(root)), wanted)
	if err != nil {
		return err
	}
	var index strings.Builder
	for _, m := range downloaded {
		name := unitOf[m.Path]
		if err := extension.Name(name).Validate(); err != nil {
			return fmt.Errorf("%s: %w", m.Path, err)
		}
		dst := filepath.Join(outDir, name)
		if err := os.CopyFS(dst, os.DirFS(m.Dir)); err != nil {
			return fmt.Errorf("%s: %w", m.Path, err)
		}
		if err := dropReplaces(filepath.Join(dst, "go.mod"), filepath.Join(root, "backend")); err != nil {
			return fmt.Errorf("%s: %w", m.Path, err)
		}
		fmt.Fprintf(&index, "%s | %s\n", name, downloadOf[m.Path])
	}
	return writeFetchedIndex(root, index.String())
}

// writeFetchedIndex always writes the index, empty when nothing was fetched,
// so a reader can tell "no downloaded units" from "an index nobody wrote".
func writeFetchedIndex(root, content string) error {
	return os.WriteFile(filepath.Join(root, filepath.FromSlash(fetchedUnitsIndex)), []byte(content), 0o644) // #nosec G306 -- a build listing, not a secret
}

// dropReplaces removes a downloaded unit's replace lines from its copy: the
// composed workspace would apply them, and ../../backend points nowhere from
// there. A copy that requires the backend gets one replace back, onto the
// tree's backend. Its require names the version it was cut against, and
// nothing else maps that version onto the workspace member.
func dropReplaces(goModPath, backendDir string) error {
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
	for _, r := range mod.Require {
		if r.Mod.Path != backendModule {
			continue
		}
		rel, err := filepath.Rel(filepath.Dir(goModPath), backendDir)
		if err != nil {
			return err
		}
		if err := mod.AddReplace(backendModule, "", filepath.ToSlash(rel), ""); err != nil {
			return err
		}
		break
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
