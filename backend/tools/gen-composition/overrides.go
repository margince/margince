// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

const (
	// overridesLayer mirrors core's frontend/: a unit file at
	// frontend/overrides/src/app/x.tsx stands in for core's frontend/src/app/x.tsx.
	overridesLayer = frontendLayer + "/overrides"
	// overridesLockFile records the core file each replacement was written
	// against, by hash. A drift warns and never fails: a pinned unit must not
	// stop a core upgrade the composed typecheck accepts.
	overridesLockFile = frontendLayer + "/overrides.lock"
	// composedOverridesFile tells the bundler which unit file replaces which core
	// file; composedOverridesCheck holds each replacement to the exports of the
	// file it replaces, under tsc. Both sit outside build/composition/, so the
	// absolute paths in them never reach a digest.
	composedOverridesFile  = "build/composition-frontend/overrides.json"
	composedOverridesCheck = "build/composition-frontend/overrides.check.ts"
	// extScreensGenFile is the screen registry's key in composedFiles; an added
	// screen is imported by a path relative to it.
	extScreensGenFile = "frontend/extscreens.gen.ts"
)

// replaceableRoots keeps replacements to UI code. The API client, auth, the
// router and the generated composition stay core's alone.
var replaceableRoots = []string{"src/app/", "src/components/", "src/features/"}

// unreplaceable are the UI files inside those roots that take a password, a
// secret or a consent, or set the data layer's policy. A replacement there
// would see what the user typed before core does.
var unreplaceable = []string{
	"src/app/queryclient.ts",
	"src/components/auth-core.tsx",
	"src/features/privacy/",
	"src/features/settings/settings-passports.tsx",
	"src/features/settings/webhooks.secret.tsx",
	"src/features/sign-in/",
}

// addedScreenPath is the one file a unit may add rather than replace. It is a
// screen in core's fork folder, which app/custom.ts serves at #/x/<key>.
var addedScreenPath = regexp.MustCompile(`^src/custom/([a-z0-9]+(?:-[a-z0-9]+)*)/screen\.tsx$`)

// fileOverride is one core frontend file a unit replaces.
type fileOverride struct {
	Unit   string `json:"unit"`
	Target string `json:"target"` // relative to core's frontend/
	Source string `json:"source"` // absolute
}

// addedScreen is one screen a unit adds under core's src/custom/.
type addedScreen struct {
	Unit   string
	Target string // relative to core's frontend/, as for a replacement
	Source string // relative to the repository root, slash-separated
}

type frontendOverrides struct {
	Replaced []fileOverride
	Added    []addedScreen
	// Warnings name each replacement whose lock line is missing or stale.
	Warnings []string
}

// collectOverrides refuses what the build cannot stand behind. A replacement
// must sit under replaceableRoots and name a file core has. A new file must be
// an added screen. No file has two owners.
func collectOverrides(root string, units []extensionUnit) (frontendOverrides, error) {
	frontendDir := filepath.Join(root, "frontend")
	owner := map[string]string{}
	var all frontendOverrides
	for _, u := range units {
		found, err := unitOverrides(root, u, frontendDir)
		if err != nil {
			return frontendOverrides{}, err
		}
		targets := make([]string, 0, len(found.Replaced)+len(found.Added))
		for _, o := range found.Replaced {
			targets = append(targets, o.Target)
		}
		for _, a := range found.Added {
			targets = append(targets, a.Target)
		}
		for _, target := range targets {
			if other, taken := owner[target]; taken {
				return frontendOverrides{}, fmt.Errorf("%s and %s both override frontend/%s — one file has one owner", other, u.Name, target)
			}
			owner[target] = u.Name
		}
		all.Replaced = append(all.Replaced, found.Replaced...)
		all.Added = append(all.Added, found.Added...)
		all.Warnings = append(all.Warnings, found.Warnings...)
	}
	sort.Slice(all.Replaced, func(i, j int) bool { return all.Replaced[i].Target < all.Replaced[j].Target })
	sort.Slice(all.Added, func(i, j int) bool { return all.Added[i].Target < all.Added[j].Target })
	return all, nil
}

func unitOverrides(root string, u extensionUnit, frontendDir string) (frontendOverrides, error) {
	layer := filepath.Join(u.Dir, filepath.FromSlash(overridesLayer))
	if _, err := os.Stat(layer); errors.Is(err, fs.ErrNotExist) {
		return frontendOverrides{}, nil
	}
	if u.Frontend == nil {
		return frontendOverrides{}, fmt.Errorf("extensions/%s: %s/ needs %s/package.json — an override resolves its own imports through the unit's package", u.Name, overridesLayer, frontendLayer)
	}
	lock, err := readLock(u.Dir, overridesLockFile)
	if err != nil {
		return frontendOverrides{}, fmt.Errorf("extensions/%s: %w", u.Name, err)
	}
	var found frontendOverrides
	err = filepath.WalkDir(layer, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(layer, p)
		if err != nil {
			return err
		}
		target := filepath.ToSlash(rel)
		core, err := fileSHA256(filepath.Join(frontendDir, filepath.FromSlash(target)))
		switch {
		case errors.Is(err, fs.ErrNotExist) && addedScreenPath.MatchString(target):
			source, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			found.Added = append(found.Added, addedScreen{Unit: u.Name, Target: target, Source: filepath.ToSlash(source)})
			return nil
		case errors.Is(err, fs.ErrNotExist):
			return fmt.Errorf("extensions/%s: %s/%s names frontend/%s, which this core does not have — a new page goes at %s/src/custom/<folder>/screen.tsx", u.Name, overridesLayer, target, target, overridesLayer)
		case err != nil:
			return err
		case addedScreenPath.MatchString(target):
			return fmt.Errorf("extensions/%s: %s/%s: core's fork already has frontend/%s — add the screen under another folder", u.Name, overridesLayer, target, target)
		case !hasPrefixIn(replaceableRoots, target):
			return fmt.Errorf("extensions/%s: %s/%s: only files under frontend/%s can be replaced", u.Name, overridesLayer, target, strings.Join(replaceableRoots, ", frontend/"))
		case hasPrefixIn(unreplaceable, target):
			return fmt.Errorf("extensions/%s: %s/%s: frontend/%s takes a credential or a consent, and stays core's", u.Name, overridesLayer, target, target)
		}
		want, locked := lock[target]
		delete(lock, target)
		switch {
		case !locked:
			found.Warnings = append(found.Warnings, fmt.Sprintf("extensions/%s: %s names no hash for %s — after reading core's file, add the line:\n  %s %s", u.Name, overridesLockFile, target, target, core))
		case want != core:
			found.Warnings = append(found.Warnings, fmt.Sprintf("extensions/%s: core's frontend/%s changed since the override was written — re-read it, check %s/%s, then set the lock line to:\n  %s %s",
				u.Name, target, overridesLayer, target, target, core))
		}
		found.Replaced = append(found.Replaced, fileOverride{Unit: u.Name, Target: target, Source: p})
		return nil
	})
	if err != nil {
		return frontendOverrides{}, err
	}
	for _, stale := range slices.Sorted(maps.Keys(lock)) {
		found.Warnings = append(found.Warnings, fmt.Sprintf("extensions/%s: %s pins %s, but %s/ has no such file — remove the line", u.Name, overridesLockFile, stale, overridesLayer))
	}
	return found, nil
}

func hasPrefixIn(prefixes []string, target string) bool {
	return slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(target, p) })
}

// readLock reads a unit's "<target> sha256:<hex>" lines; # starts a comment.
func readLock(unitDir, name string) (map[string]string, error) {
	raw, err := os.ReadFile(filepath.Join(unitDir, filepath.FromSlash(name))) // #nosec G304 -- a unit's own lock at a fixed path
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	lock := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(string(raw)))
	for line := 1; sc.Scan(); line++ {
		text, _, _ := strings.Cut(sc.Text(), "#")
		fields := strings.Fields(text)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s:%d: want <path> sha256:<hex>", name, line)
		}
		if _, twice := lock[fields[0]]; twice {
			return nil, fmt.Errorf("%s:%d: %s is pinned twice", name, line, fields[0])
		}
		lock[fields[0]] = fields[1]
	}
	return lock, sc.Err()
}

func fileSHA256(file string) (string, error) {
	raw, err := os.ReadFile(file) // #nosec G304 -- a core frontend file an override names
	if err != nil {
		return "", err
	}
	return bytesSHA256(raw), nil
}

func bytesSHA256(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// emitOverrides writes the bundler's map and the type check, replacing both so
// an override removed from its unit leaves nothing that would still apply it.
func emitOverrides(root string, replaced []fileOverride) error {
	mapFile := filepath.Join(root, filepath.FromSlash(composedOverridesFile))
	if err := os.MkdirAll(filepath.Dir(mapFile), 0o755); err != nil { // #nosec G301 -- build output stays readable to another UID
		return err
	}
	if replaced == nil {
		replaced = []fileOverride{}
	}
	encoded, err := json.MarshalIndent(replaced, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(mapFile, append(encoded, '\n'), 0o644); err != nil { // #nosec G306 -- build output
		return err
	}
	var b strings.Builder
	b.WriteString("// GENERATED by gen-composition. Each override must keep every export of the\n")
	b.WriteString("// core file it replaces, with a type core's callers still accept.\n")
	b.WriteString("export type Keeps<_Override extends Original, Original> = true;\n")
	for i, o := range replaced {
		fmt.Fprintf(&b, "import type * as Original%d from %s;\n", i, tsString("@margince-core/"+strings.TrimSuffix(o.Target, filepath.Ext(o.Target))))
		fmt.Fprintf(&b, "import type * as Override%d from %s;\n", i, tsString(strings.TrimSuffix(o.Source, filepath.Ext(o.Source))))
		fmt.Fprintf(&b, "export type Check%d = Keeps<typeof Override%d, typeof Original%d>;\n", i, i, i)
	}
	return os.WriteFile(filepath.Join(root, filepath.FromSlash(composedOverridesCheck)), []byte(b.String()), 0o644) // #nosec G306 -- build output
}

// addedScreenSpecifier is the import extscreens.gen.ts reaches an added screen by.
func addedScreenSpecifier(a addedScreen) string {
	dir := path.Join("build/composition", path.Dir(extScreensGenFile))
	up := strings.Repeat("../", strings.Count(dir, "/")+1)
	return up + strings.TrimSuffix(a.Source, ".tsx")
}
