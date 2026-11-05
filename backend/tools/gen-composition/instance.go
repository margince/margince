// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

// An instance outside this repository requires core's modules by version and
// runs this command as a Go tool of its own module:
//
//	tool github.com/margince/margince/backend/tools/gen-composition
//
//	require (
//		github.com/margince/margince <version>                // the web app, scripts, go.work
//		github.com/margince/margince/backend <version>
//		github.com/margince/margince/backend/tools <version>  // this composer
//	)
//
//	go tool gen-composition -instance .
//
// The command fetches each of those modules at the version the instance
// requires, inside the instance's module so its go.sum verifies them, puts the
// core tree together and composes it. The client fetches nothing by hand.
const (
	coreRootModule  = "github.com/margince/margince"
	coreToolsModule = "github.com/margince/margince/backend/tools"
	// treeMarkerFile records which core commit a fetched tree holds, so an
	// unchanged require does not fetch and copy the tree again.
	treeMarkerFile = ".margince-core"
)

// instanceGoModOverride names the instance's go.mod when -instance is given;
// a core checkout reads its own instance/ instead.
var instanceGoModOverride string

// requiredCoreModules are the core modules an instance requires, and where
// each goes in the tree. They must all be at one commit.
var requiredCoreModules = []struct{ path, dir string }{
	{coreRootModule, "."},
	{backendModule, "backend"},
	{coreToolsModule, "backend/tools"},
}

// prepareInstanceTree puts together the core tree the instance requires in
// treeDir and returns it, ready to compose. The composition stub is not
// required by the instance: it is part of the backend's release, so it is
// fetched at the backend's commit.
func prepareInstanceTree(instanceDir, treeDir string) (string, error) {
	instanceDir, err := realAbs(instanceDir)
	if err != nil {
		return "", err
	}
	goModPath := filepath.Join(instanceDir, "go.mod")
	required, err := requiredCoreVersions(goModPath)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(treeDir) {
		treeDir = filepath.Join(instanceDir, treeDir)
	}
	instanceGoModOverride = goModPath

	// Inside the instance's module, so its go.sum checks every download.
	var wanted []string
	for _, m := range requiredCoreModules {
		wanted = append(wanted, m.path+"@"+required[m.path])
	}
	downloaded, err := downloadModules(instanceDir, wanted)
	if err != nil {
		return "", err
	}
	dirs, commit, err := oneCoreCommit(downloaded, required)
	if err != nil {
		return "", err
	}
	if current, _ := os.ReadFile(filepath.Join(treeDir, treeMarkerFile)); strings.TrimSpace(string(current)) == commit { // #nosec G304 -- the marker this function writes
		return realAbs(treeDir)
	}
	stub, err := downloadModules(os.TempDir(), []string{compositionModule + "@" + commit})
	if err != nil {
		return "", err
	}
	if len(stub) != 1 || stub[0].Dir == "" {
		return "", fmt.Errorf("%s@%s: the go command reported no download", compositionModule, commit)
	}
	dirs[compositionModule] = stub[0].Dir

	if err := os.RemoveAll(treeDir); err != nil {
		return "", err
	}
	layout := append(requiredCoreModules[:len(requiredCoreModules):len(requiredCoreModules)], struct{ path, dir string }{compositionModule, "composition"})
	for _, m := range layout {
		if err := os.CopyFS(filepath.Join(treeDir, m.dir), os.DirFS(dirs[m.path])); err != nil {
			return "", fmt.Errorf("%s: %w", m.path, err)
		}
	}
	if err := os.WriteFile(filepath.Join(treeDir, treeMarkerFile), []byte(commit+"\n"), 0o644); err != nil { // #nosec G306 -- build output
		return "", err
	}
	return realAbs(treeDir)
}

// requiredCoreVersions reads the version the instance requires of each core
// module, and refuses an instance that leaves one out.
func requiredCoreVersions(goModPath string) (map[string]string, error) {
	raw, err := os.ReadFile(goModPath) // #nosec G304 -- the instance's go.mod, named by the operator
	if err != nil {
		return nil, err
	}
	mod, err := modfile.ParseLax(goModPath, raw, nil)
	if err != nil {
		return nil, err
	}
	versions := map[string]string{}
	for _, r := range mod.Require {
		versions[r.Mod.Path] = r.Mod.Version
	}
	var missing []string
	for _, m := range requiredCoreModules {
		if versions[m.path] == "" {
			missing = append(missing, m.path)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%s requires no %s; require core at one version:\n"+
			"  go get -tool %s/gen-composition@<v>\n  go get %s@<v> %s@<v>",
			goModPath, strings.Join(missing, ", "), coreToolsModule, coreRootModule, backendModule)
	}
	return versions, nil
}

// oneCoreCommit maps each downloaded core module to its folder and refuses
// versions built from different commits: the web app, the backend and the
// composer are released together and only work together.
func oneCoreCommit(downloaded []downloadedModule, required map[string]string) (map[string]string, string, error) {
	dirs := map[string]string{}
	commits := map[string]string{}
	for _, m := range downloaded {
		commit, err := commitOf(m, required[m.Path])
		if err != nil {
			return nil, "", err
		}
		dirs[m.Path], commits[m.Path] = m.Dir, commit
	}
	var commit string
	for _, m := range requiredCoreModules {
		c, ok := commits[m.path]
		if !ok || dirs[m.path] == "" {
			return nil, "", fmt.Errorf("%s@%s: the go command reported no download", m.path, required[m.path])
		}
		if commit == "" {
			commit = c
			continue
		}
		if !sameCommit(c, commit) {
			return nil, "", fmt.Errorf("core modules are at different commits (%s@%s, %s@%s); require them at one version",
				requiredCoreModules[0].path, required[requiredCoreModules[0].path], m.path, required[m.path])
		}
	}
	return dirs, commit, nil
}

// commitOf is the commit a module version was built from: the origin the go
// command reports, or a pseudo-version's revision.
func commitOf(m downloadedModule, version string) (string, error) {
	if m.Origin != nil && m.Origin.Hash != "" {
		return m.Origin.Hash, nil
	}
	if module.IsPseudoVersion(version) {
		return module.PseudoVersionRev(version)
	}
	return "", fmt.Errorf("%s@%s: cannot tell which commit it is; require a pseudo-version, or a version the module proxy reports an origin for", m.Path, version)
}

// sameCommit compares a full hash with a pseudo-version's 12-character one.
func sameCommit(a, b string) bool {
	n := min(len(a), len(b))
	return n >= 12 && a[:n] == b[:n]
}

// realAbs is path made absolute with every symlink resolved. The composer
// writes relative links and workspace paths between the tree and the units,
// and pnpm resolves a unit's node_modules links from where the files really
// are, so a path through a symlink would climb the wrong number of levels.
func realAbs(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
