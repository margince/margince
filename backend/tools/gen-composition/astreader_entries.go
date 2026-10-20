// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Reading a unit's declared ENTRIES into the manifest: the places on core
// screens where the unit asks for a way into its own screen.

import (
	"fmt"
	"go/ast"
	"sort"
	"strings"

	"github.com/margince/margince/backend/pkg/extension"
)

// declaredEntry is one entry as the manifest and the SPA registry carry it.
type declaredEntry struct {
	Slot   string `json:"slot"`
	Label  string `json:"label"`
	Icon   string `json:"icon"`
	Target string `json:"target"`
	Object string `json:"object"`
}

// readEntries reads the Entries slice literal. Each entry passes the published
// Entry.Validate the boot runs. Its object must be one the unit's own
// operations gate on, which the boot checks as well.
func (r *unitReader) readEntries(expr ast.Expr, file *ast.File) ([]declaredEntry, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "Entries must be a slice literal")
	}
	ext := importAlias(file, extensionPkgPath)
	gated := map[string]bool{}
	for _, v := range r.verbs {
		gated[v.verb.RbacObject] = true
	}
	out := make([]declaredEntry, 0, len(lit.Elts))
	seen := map[string]bool{}
	for _, elt := range lit.Elts {
		entry, err := r.readEntry(elt, ext)
		if err != nil {
			return nil, err
		}
		decl := extension.Entry{
			Slot: extension.EntrySlot(entry.Slot), Label: entry.Label,
			Icon: extension.EntryIcon(entry.Icon), Target: entry.Target, Object: entry.Object,
		}
		if err := decl.Validate(extension.Name(r.unit)); err != nil {
			return nil, r.errPos(elt, "%v", err)
		}
		if !gated[entry.Object] {
			return nil, r.errPos(elt, "entry in slot %q is gated on %q, which none of the unit's operations gates on — no grant of it lets a reader use the unit", entry.Slot, entry.Object)
		}
		key := entry.Slot + "\x00" + entry.Target
		if seen[key] {
			return nil, r.errPos(elt, "two entries in slot %q with target %q", entry.Slot, entry.Target)
		}
		seen[key] = true
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Slot != out[j].Slot {
			return out[i].Slot < out[j].Slot
		}
		return out[i].Target < out[j].Target
	})
	return out, nil
}

// readEntry reads one extension.Entry literal.
func (r *unitReader) readEntry(elt ast.Expr, ext string) (declaredEntry, error) {
	lit, ok := elt.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "Entry")) {
		return declaredEntry{}, r.errAt(elt, "an Entries entry must be an extension.Entry literal")
	}
	var entry declaredEntry
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return declaredEntry{}, r.errAt(e, "Entry fields must be keyed")
		}
		k, ok := kv.Key.(*ast.Ident)
		if !ok {
			return declaredEntry{}, r.errAt(kv.Key, "Entry fields must be keyed by name")
		}
		var err error
		switch k.Name {
		case "Slot":
			entry.Slot, err = r.constValue(kv.Value, ext)
		case "Icon":
			entry.Icon, err = r.constValue(kv.Value, ext)
		case "Label":
			entry.Label, err = r.stringLit(kv.Value, "Entry.Label")
		case "Target":
			entry.Target, err = r.stringLit(kv.Value, "Entry.Target")
		case "Object":
			entry.Object, err = r.stringLit(kv.Value, "Entry.Object")
		default:
			err = r.errAt(kv, "Entry field %s is not derivable by this generator", k.Name)
		}
		if err != nil {
			return declaredEntry{}, err
		}
	}
	return entry, nil
}

// checkEntryLabels refuses an entry whose label is missing from the unit's own
// copy in any locale. A reader of that language would see a raw key.
func checkEntryLabels(manifests []derivedManifest, merged map[string]map[string]string) error {
	for _, m := range manifests {
		prefix := localeKeyPrefix(m.Unit.Name)
		for _, entry := range m.Manifest.Entries {
			if !strings.HasPrefix(entry.Label, prefix) {
				return fmt.Errorf("extensions/%s: entry label %q is outside the unit's copy namespace %q", m.Unit.Name, entry.Label, prefix)
			}
			for _, locale := range composedLocales {
				if _, ok := merged[locale][entry.Label]; !ok {
					return fmt.Errorf("extensions/%s: entry label %q is missing from %s/%s/%s.json", m.Unit.Name, entry.Label, frontendLayer, localesLayer, locale)
				}
			}
		}
	}
	return nil
}
