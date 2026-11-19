// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messagingpolicy

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// Unit is what one extension's policy folder declares.
type Unit struct {
	// Policies are the unit's packs, in folder order.
	Policies []Policy
	// PerCountry says the folder holds one folder per pack instead of one
	// index.md, so the unit declares a list of rule sets.
	PerCountry bool
}

// sharedFolders sit at the top of a per-country policy folder and are no pack.
var sharedFolders = map[string]bool{"regional": true, "fixtures": true, "research": true}

// CompileUnit compiles a policy folder in either layout. With an index.md at
// the top it is one pack, as Compile reads it. Without one, every other folder
// is a pack named for its country, with its own index.md, and the packs share
// fallback.md, regional/ and fixtures/ at the top.
func CompileUnit(fsys fs.FS) (Unit, error) {
	_, err := fs.Stat(fsys, IndexFile)
	switch {
	case err == nil:
		policy, err := Compile(fsys)
		if err != nil {
			return Unit{}, err
		}
		return Unit{Policies: []Policy{policy}}, nil
	case !errors.Is(err, fs.ErrNotExist):
		return Unit{}, err
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return Unit{}, err
	}
	out := Unit{PerCountry: true}
	listed := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() || sharedFolders[entry.Name()] {
			continue
		}
		if _, err := fs.Stat(fsys, path.Join(entry.Name(), IndexFile)); err != nil {
			return Unit{}, fmt.Errorf("%s/ has no %s; a pack folder lists its layers there", entry.Name(), IndexFile)
		}
		policy, err := compilePack(fsys, entry.Name())
		if err != nil {
			return Unit{}, err
		}
		for _, source := range policy.Sources {
			listed[source] = true
		}
		out.Policies = append(out.Policies, policy)
	}
	if len(out.Policies) == 0 {
		return Unit{}, fmt.Errorf("the policy folder has no %s and no pack folder holding one", IndexFile)
	}
	if err := unlistedShared(fsys, listed); err != nil {
		return Unit{}, err
	}
	if err := sameSharedLayers(fsys, out.Policies); err != nil {
		return Unit{}, err
	}
	return out, nil
}

// sameSharedLayers refuses a pack that leaves out a shared layer or lists the
// shared layers in another order than the others. A pack that skipped the
// regional layer would escape the baseline every member state shares.
func sameSharedLayers(fsys fs.FS, policies []Policy) error {
	onDisk, err := fs.Glob(fsys, "regional/*.md")
	if err != nil {
		return err
	}
	if _, err := fs.Stat(fsys, "fallback.md"); err == nil {
		onDisk = append([]string{"fallback.md"}, onDisk...)
	}
	want := sharedOf(policies[0])
	for _, policy := range policies {
		got := sharedOf(policy)
		if !slices.Equal(got, want) {
			return fmt.Errorf("%s lists the shared layers %v and %s lists %v; every pack lists the same ones in one order",
				policy.Sources[0], got, policies[0].Sources[0], want)
		}
	}
	for _, layer := range onDisk {
		if !slices.Contains(want, layer) {
			return fmt.Errorf("%s is a shared layer and %s does not list it; every pack lists every shared layer",
				layer, policies[0].Sources[0])
		}
	}
	return nil
}

// sharedOf answers the layers a pack compiled from outside its own folder.
func sharedOf(policy Policy) []string {
	var out []string
	for _, source := range policy.Sources[1:] {
		if source == "fallback.md" || strings.HasPrefix(source, "regional/") {
			out = append(out, source)
		}
	}
	return out
}

// unlistedShared refuses a file at the top of a per-country folder, or under
// regional/, that no pack lists. Each pack's own folder was checked when it
// compiled, and fixtures/ and research/ hold no layer.
func unlistedShared(fsys fs.FS, listed map[string]bool) error {
	return fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case entry.IsDir() && name != "." && name != "regional" && !strings.Contains(name, "/"):
			return fs.SkipDir
		case entry.IsDir() || !strings.HasSuffix(name, ".md") || listed[name]:
			return nil
		}
		return fmt.Errorf("%s is in the policy folder and no pack's %s lists it; list it or move it to research/",
			name, IndexFile)
	})
}
