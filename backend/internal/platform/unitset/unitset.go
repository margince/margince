// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package unitset reads which extensions an instance composes, and where each
// one's code is. The composer and every census of the composed set read it
// here, so a census counts the units a build composes and no others.
package unitset

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

const (
	// DevelopmentOnlyMarker ends a require line whose unit only development and
	// test builds compose.
	DevelopmentOnlyMarker = "margince:development-only"
	// FetchedDir holds the versioned units the composer copies out of the
	// module cache, one folder per unit name.
	FetchedDir = "build/extensions"
	// ManifestFile is the manifest the composer writes into each unit's folder.
	ManifestFile = "manifest.generated.json"
)

// Core's own modules. An instance requires them, and none is an extension.
const (
	RootModule        = "github.com/margince/margince"
	BackendModule     = "github.com/margince/margince/backend"
	ToolsModule       = "github.com/margince/margince/backend/tools"
	CompositionModule = "github.com/margince/margince/composition"
)

var majorVersionSuffix = regexp.MustCompile(`^v[0-9]+$`)

// Unit is one extension the instance requires, and where its code is.
type Unit struct {
	Name       string
	ModulePath string
	Version    string
	// Dir is the folder the unit is composed from: a replace target, or its
	// copy under FetchedDir.
	Dir string
	// Download is the module@version fetched for it: the requirement itself,
	// or what the instance's replace points it at.
	Download        string
	Fetched         bool
	DevelopmentOnly bool
}

// ManifestPath is where the composer writes the unit's manifest.
func (u Unit) ManifestPath() string {
	return filepath.Join(u.Dir, ManifestFile)
}

// IsCoreModule reports whether a module path is one of core's own modules,
// which an instance requires but never composes as an extension.
func IsCoreModule(modulePath string) bool {
	switch modulePath {
	case RootModule, BackendModule, ToolsModule, CompositionModule:
		return true
	}
	return false
}

// Selected lists the units the instance at root/instance composes. A
// production build passes includeDevelopment false.
func Selected(root string, includeDevelopment bool) ([]Unit, error) {
	units, err := Read(root, filepath.Join(root, "instance", "go.mod"))
	if err != nil {
		return nil, err
	}
	return Select(units, includeDevelopment), nil
}

// Select drops the development-only units unless includeDevelopment is set.
// Selection precedes fetching and every generated artifact, so an excluded
// example adds no code, contract, screen or migration to a production build.
func Select(units []Unit, includeDevelopment bool) []Unit {
	selected := make([]Unit, 0, len(units))
	for _, unit := range units {
		if !unit.DevelopmentOnly || includeDevelopment {
			selected = append(selected, unit)
		}
	}
	return selected
}

// Read lists every extension the go.mod at goModPath requires: each module
// except core's own and the `// indirect` dependencies. A missing go.mod is a
// core-only build and lists nothing. root is where FetchedDir sits.
func Read(root, goModPath string) ([]Unit, error) {
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
	if err := validateDevelopmentMarkers(mod); err != nil {
		return nil, fmt.Errorf("%s: %w", goModPath, err)
	}
	replaces := map[string]*modfile.Replace{}
	for _, r := range mod.Replace {
		replaces[r.Old.Path+"@"+r.Old.Version] = r
	}
	seen := map[string]string{}
	var units []Unit
	for _, r := range mod.Require {
		if r.Indirect || IsCoreModule(r.Mod.Path) {
			continue
		}
		name := nameFromModulePath(r.Mod.Path)
		if other, dup := seen[name]; dup {
			return nil, fmt.Errorf("%s: %s and %s are both the unit %q; one name, one unit", goModPath, other, r.Mod.Path, name)
		}
		seen[name] = r.Mod.Path
		unit := resolve(root, goModPath, name, r.Mod, replaces)
		for _, comment := range r.Syntax.Suffix {
			if strings.TrimSpace(strings.TrimPrefix(comment.Token, "//")) == DevelopmentOnlyMarker {
				unit.DevelopmentOnly = true
			}
		}
		units = append(units, unit)
	}
	return units, nil
}

// resolve applies the instance's replace the way Go does: one for this exact
// version wins over one for every version. A replace onto a folder is composed
// in place; onto another module version, that version is fetched.
func resolve(root, goModPath, name string, req module.Version, replaces map[string]*modfile.Replace) Unit {
	u := Unit{Name: name, ModulePath: req.Path, Version: req.Version, Download: req.Path + "@" + req.Version}
	r := replaces[req.Path+"@"+req.Version]
	if r == nil {
		r = replaces[req.Path+"@"]
	}
	if r != nil && r.New.Version == "" {
		u.Dir = r.New.Path
		if !filepath.IsAbs(u.Dir) {
			u.Dir = filepath.Join(filepath.Dir(goModPath), u.Dir)
		}
		u.Dir = filepath.Clean(u.Dir)
		return u
	}
	if r != nil {
		u.Download = r.New.Path + "@" + r.New.Version
	}
	u.Dir, u.Fetched = filepath.Join(root, filepath.FromSlash(FetchedDir), name), true
	return u
}

// nameFromModulePath takes the unit name from the module path's last element,
// skipping a major-version suffix (…/crm/v2 is the unit crm).
func nameFromModulePath(modulePath string) string {
	base := path.Base(modulePath)
	if majorVersionSuffix.MatchString(base) {
		return path.Base(path.Dir(modulePath))
	}
	return base
}
