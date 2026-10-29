// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	// overlaysLayer mirrors core's backend module inside a unit: a file at
	// overlays/internal/modules/deals/x.go is compiled into core's package deals.
	overlaysLayer = "overlays"
	// overlaysLockFile records the core function each decoration was written
	// against, by the hash of its source. A drift warns and never fails, as
	// overridesLockFile does: a pinned unit must not stop a core upgrade.
	overlaysLockFile = "overlays.lock"
	// overlayBuildTag keeps an overlay file out of the unit's own build: it
	// compiles only inside core's package, where its types are.
	overlayBuildTag = "//go:build margince_overlay"
	// decorateDirective marks a function that replaces core's function of the
	// same name; core's stays callable as <name>__core.
	decorateDirective = "//margince:decorate"
	coreSuffix        = "__core"
	// composedOverlayFile is the map `go build -overlay` reads. It is always
	// written, empty when no unit has overlays, so every composed go command
	// can pass the flag.
	composedOverlayDir  = "build/overlay"
	composedOverlayFile = composedOverlayDir + "/overlay.json"
)

// overlayableRoot keeps overlays to the modules' business logic. Auth, the
// database layer, compose and the generated contracts stay core's alone.
const overlayableRoot = "internal/modules/"

// decoration is one core function a unit's overlay replaces.
type decoration struct {
	unit    string
	pkg     string // relative to the backend module, slash-separated
	funcKey string // "(*Store).CreateDeal" or "Name"
}

func (d decoration) id() string { return d.pkg + "." + d.funcKey }

// declaredOverlays lists the "<package>.<function>" core functions a unit's
// overlays/ decorates, for its manifest.
func declaredOverlays(unitDir string) ([]string, error) {
	var out []string
	err := eachOverlayFile(unitDir, func(path, pkg string, file *ast.File) error {
		for _, key := range decoratedFuncs(file) {
			out = append(out, pkg+"."+key)
		}
		return nil
	})
	slices.Sort(out)
	return out, err
}

// eachOverlayFile parses every .go file under a unit's overlays/, with the
// core package it compiles into.
func eachOverlayFile(unitDir string, fn func(path, pkg string, file *ast.File) error) error {
	layer := filepath.Join(unitDir, overlaysLayer)
	return filepath.WalkDir(layer, func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) && p == layer {
			return fs.SkipAll
		}
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(layer, filepath.Dir(p))
		if err != nil {
			return err
		}
		return fn(p, filepath.ToSlash(rel), file)
	})
}

func decoratedFuncs(file *ast.File) []string {
	var out []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Doc != nil && slices.ContainsFunc(fn.Doc.List, func(c *ast.Comment) bool {
			return strings.TrimSpace(c.Text) == decorateDirective
		}) {
			out = append(out, funcKey(fn))
		}
	}
	return out
}

// emitReplacements writes the frontend files and backend functions units
// compile over core. It prints each lock drift as a warning.
func emitReplacements(root string, composed composition) error {
	if err := emitOverrides(root, composed.Overrides.Replaced); err != nil {
		return err
	}
	units := make([]extensionUnit, 0, len(composed.Manifests))
	for _, m := range composed.Manifests {
		units = append(units, m.Unit)
	}
	overlayWarnings, err := composeOverlays(root, units)
	if err != nil {
		return err
	}
	for _, warning := range slices.Concat(composed.Overrides.Warnings, overlayWarnings) {
		fmt.Fprintln(os.Stderr, "gen-composition: warning:", warning)
	}
	return nil
}

// composeOverlays rewrites build/overlay for every unit's overlays/. It
// returns a warning for each lock line that is missing, stale or extra.
func composeOverlays(root string, units []extensionUnit) ([]string, error) {
	out := filepath.Join(root, filepath.FromSlash(composedOverlayDir))
	if err := os.RemoveAll(out); err != nil {
		return nil, err
	}
	// Keyed by real path: go matches overlay keys against the paths it
	// resolves, so a checkout reached through a symlink would apply nothing.
	backend, err := filepath.EvalSymlinks(filepath.Join(root, "backend"))
	if err != nil {
		return nil, err
	}
	replace := map[string]string{}
	owners := map[string]string{}
	var decorations []decoration
	var warnings []string
	for _, u := range units {
		found, warned, err := composeUnitOverlays(u, backend, out, replace, owners)
		if err != nil {
			return nil, err
		}
		decorations = append(decorations, found...)
		warnings = append(warnings, warned...)
	}
	if err := rewriteDecoratedCore(decorations, backend, out, replace); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(map[string]map[string]string{"Replace": replace}, "", "  ")
	if err != nil {
		return nil, err
	}
	return warnings, writeBuildFile(filepath.Join(root, filepath.FromSlash(composedOverlayFile)), append(encoded, '\n'))
}

// composeUnitOverlays adds one unit's files to the overlay and returns the
// core functions they decorate, with a warning per lock line that drifted.
func composeUnitOverlays(u extensionUnit, backend, out string, replace, owners map[string]string) ([]decoration, []string, error) {
	if _, err := os.Stat(filepath.Join(u.Dir, overlaysLayer)); errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	lock, err := readLock(u.Dir, overlaysLockFile)
	if err != nil {
		return nil, nil, fmt.Errorf("extensions/%s: %w", u.Name, err)
	}
	var found []decoration
	var warnings []string
	err = eachOverlayFile(u.Dir, func(path, pkg string, file *ast.File) error {
		if err := unitOverlayFile(u, path, pkg, backend, out, replace); err != nil {
			return err
		}
		for _, key := range decoratedFuncs(file) {
			d := decoration{unit: u.Name, pkg: pkg, funcKey: key}
			if other, taken := owners[d.id()]; taken {
				return fmt.Errorf("%s and %s both decorate %s — one function has one decorator", other, u.Name, d.id())
			}
			owners[d.id()] = u.Name
			warning, err := checkFunctionLock(lock, backend, d)
			if err != nil {
				return fmt.Errorf("extensions/%s: %w", u.Name, err)
			}
			if warning != "" {
				warnings = append(warnings, fmt.Sprintf("extensions/%s: %s", u.Name, warning))
			}
			delete(lock, d.id())
			found = append(found, d)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	for _, stale := range slices.Sorted(maps.Keys(lock)) {
		warnings = append(warnings, fmt.Sprintf("extensions/%s: %s pins %s, which no overlay decorates — remove the line", u.Name, overlaysLockFile, stale))
	}
	return found, warnings, nil
}

// unitOverlayFile copies one overlay file, without its build tag, to where the
// build reads it, and maps it into the core package it names.
func unitOverlayFile(u extensionUnit, path, pkg, backend, out string, replace map[string]string) error {
	if !strings.HasPrefix(pkg+"/", overlayableRoot) {
		return fmt.Errorf("extensions/%s: %s/%s: only packages under backend/%s can be overlaid", u.Name, overlaysLayer, pkg, overlayableRoot)
	}
	if info, err := os.Stat(filepath.Join(backend, filepath.FromSlash(pkg))); err != nil || !info.IsDir() {
		return fmt.Errorf("extensions/%s: %s/%s names core package %s, which this core does not have", u.Name, overlaysLayer, pkg, pkg)
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- a unit's own overlay file
	if err != nil {
		return err
	}
	rest, tagged := withoutBuildTag(raw)
	if !tagged {
		return fmt.Errorf("extensions/%s: %s must carry %q above its package clause, so the unit's own build leaves it out", u.Name, path, overlayBuildTag)
	}
	name := "zz_overlay_" + strings.ReplaceAll(u.Name, "-", "_") + "_" + filepath.Base(path)
	copied := filepath.Join(out, "units", u.Name, filepath.FromSlash(pkg), filepath.Base(path))
	if err := writeBuildFile(copied, rest); err != nil {
		return err
	}
	replace[filepath.Join(backend, filepath.FromSlash(pkg), name)] = copied
	return nil
}

// withoutBuildTag drops the overlay build tag from the lines above the package
// clause, where Go reads constraints, and reports whether it was there.
func withoutBuildTag(raw []byte) ([]byte, bool) {
	lines := bytes.Split(raw, []byte("\n"))
	for i, line := range lines {
		text := strings.TrimSpace(string(line))
		if strings.HasPrefix(text, "package ") {
			return raw, false
		}
		if text == overlayBuildTag {
			return bytes.Join(slices.Delete(lines, i, i+1), []byte("\n")), true
		}
	}
	return raw, false
}

// funcKey names a function the way a lock line and an error do.
func funcKey(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	switch recv := fn.Recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if id, ok := recv.X.(*ast.Ident); ok {
			return "(*" + id.Name + ")." + fn.Name.Name
		}
	case *ast.Ident:
		return "(" + recv.Name + ")." + fn.Name.Name
	}
	return fn.Name.Name
}

// checkFunctionLock compares a decorated function's source with the hash the
// unit's lock pins, and returns a warning naming the line to write when they
// disagree.
func checkFunctionLock(lock map[string]string, backend string, d decoration) (string, error) {
	_, src, err := findCoreFunc(backend, d.pkg, d.funcKey)
	if err != nil {
		return "", err
	}
	got := bytesSHA256(src)
	want, locked := lock[d.id()]
	switch {
	case !locked:
		return fmt.Sprintf("%s names no hash for %s — after reading core's function, add the line:\n  %s %s", overlaysLockFile, d.id(), d.id(), got), nil
	case want != got:
		return fmt.Sprintf("core's %s changed since the overlay was written — re-read it, check the overlay, then set the lock line to:\n  %s %s", d.id(), d.id(), got), nil
	}
	return "", nil
}

// findCoreFunc finds a function in a core package and returns its file and
// its source, from `func` to its closing brace.
func findCoreFunc(backend, pkg, key string) (string, []byte, error) {
	dir := filepath.Join(backend, filepath.FromSlash(pkg))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, fmt.Errorf("an overlay names core package %s, which this core does not have", pkg)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path) // #nosec G304 -- a core source file
		if err != nil {
			return "", nil, err
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, raw, 0)
		if err != nil {
			return "", nil, err
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && funcKey(fn) == key {
				return path, raw[fset.Position(fn.Pos()).Offset:fset.Position(fn.End()).Offset], nil
			}
		}
	}
	return "", nil, fmt.Errorf("an overlay decorates %s.%s, which this core does not have", pkg, key)
}

// rewriteDecoratedCore renames each decorated function to <name>__core in a
// copy of its file. Core's original stays callable, and every other line of
// the file compiles from core's own source.
func rewriteDecoratedCore(decorations []decoration, backend, out string, replace map[string]string) error {
	byFile := map[string][]string{}
	for _, d := range decorations {
		path, _, err := findCoreFunc(backend, d.pkg, d.funcKey)
		if err != nil {
			return err
		}
		byFile[path] = append(byFile[path], d.funcKey)
	}
	for _, path := range slices.Sorted(maps.Keys(byFile)) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && slices.Contains(byFile[path], funcKey(fn)) {
				fn.Name.Name += coreSuffix
			}
		}
		var buf bytes.Buffer
		if err := format.Node(&buf, fset, file); err != nil {
			return err
		}
		rel, err := filepath.Rel(backend, path)
		if err != nil {
			return err
		}
		copied := filepath.Join(out, "core", rel)
		if err := writeBuildFile(copied, buf.Bytes()); err != nil {
			return err
		}
		replace[path] = copied
	}
	return nil
}

func writeBuildFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- build output stays readable to another UID
		return err
	}
	return os.WriteFile(path, content, 0o644) // #nosec G306 -- build output
}
