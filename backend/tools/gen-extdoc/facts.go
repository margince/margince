// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/pkg/extension"
)

// The doors a unit has on a record, in the order the page lists them.
const (
	doorLand = iota
	doorMove
	doorCreate
	doorRead
	doorFind
	doorFilter
	doorNotice
	doorHear
	doorErased
	doorMerged
	doorButton
	doorNote
	doorCard
	doorCount
)

// knownKinds are the columns every page has, in this order. A kind the code
// knows that is not here comes after them, sorted.
var knownKinds = []string{"company", "deal", "contact", "activity", "signal"}

// doors maps a record kind to the doors a unit has on it.
type doors map[string]*[doorCount]bool

func (d doors) set(kind string, door int) {
	if d[kind] == nil {
		d[kind] = new([doorCount]bool)
	}
	d[kind][door] = true
}

// deriveDoors reads each door off the code that provides it.
func deriveDoors(src *pkgSource) doors {
	d := doors{}
	isKind := func(kind string) bool {
		return slices.Contains(knownKinds, kind) || slices.Contains(src.recordKinds, kind)
	}
	for _, kind := range src.recordKinds {
		d.set(kind, doorLand)
		if hasField(src.fields(title(kind)+"Fields"), "StageID") {
			d.set(kind, doorMove)
		}
	}
	for _, repo := range src.coreRepos() {
		kind, ok := strings.CutSuffix(strings.ToLower(repo), "repo")
		if !ok || !isKind(kind) {
			continue
		}
		for _, m := range src.methods(repo) {
			switch m {
			case "Create":
				d.set(kind, doorCreate)
			case "Get", "List":
				d.set(kind, doorRead)
			}
		}
	}
	for name := range src.types {
		kind, ok := strings.CutSuffix(strings.ToLower(name), "query")
		if !ok || !isKind(kind) {
			continue
		}
		fields := src.fields(name)
		if hasField(fields, "SourceSystem") {
			d.set(kind, doorFind)
		}
		if hasField(fields, "CustomFields") {
			d.set(kind, doorFilter)
		}
	}
	for _, kind := range extension.NoticeRecordTypes() {
		d.set(kind, doorNotice)
	}
	deriveEvents(d)
	deriveSlots(d)
	return d
}

func deriveEvents(d doors) {
	for _, typ := range events.Types() {
		kind, verb, err := events.SplitType(typ)
		if err != nil {
			continue
		}
		if _, known := d[kind]; !known && !slices.Contains(knownKinds, kind) {
			continue
		}
		d.set(kind, doorHear)
		switch verb {
		case "erased":
			d.set(kind, doorErased)
		case "merged":
			d.set(kind, doorMerged)
		}
	}
}

// deriveSlots reads the page slots, named "<place>.<kind>". The Log activity
// form belongs to the activity.
func deriveSlots(d doors) {
	for _, slot := range extension.EntrySlots() {
		place, kind, found := strings.Cut(string(slot), ".")
		switch {
		case !found && slot == extension.SlotLogActivity:
			d.set("activity", doorButton)
		case place == "record_header":
			d.set(kind, doorButton)
		case place == "record_mark":
			d.set(kind, doorNote)
		}
	}
	for _, slot := range extension.PanelSlots() {
		if place, kind, found := strings.Cut(string(slot), "."); found && place == "record_panel" {
			d.set(kind, doorCard)
		}
	}
}

func title(s string) string { return strings.ToUpper(s[:1]) + s[1:] }

// columns is every kind the doors name that has a door of its own, the known
// kinds first.
func (d doors) columns() []string {
	var extra []string
	for kind := range d {
		if !slices.Contains(knownKinds, kind) {
			extra = append(extra, kind)
		}
	}
	slices.Sort(extra)
	return append(slices.Clone(knownKinds), extra...)
}

// extensionFields are the fields of extension.Extension that bring something
// to the core, each with the first sentence of its comment.
type extensionField struct{ name, doc string }

func deriveFields(src *pkgSource) ([]extensionField, error) {
	var out []extensionField
	for _, f := range src.fields("Extension") {
		for _, n := range f.Names {
			switch n.Name {
			case "Name", "Version", "Description":
				continue
			}
			doc := firstSentence(f.Doc.Text())
			if doc == "" {
				return nil, fmt.Errorf("gen-extdoc: Extension.%s has no doc comment to describe it by", n.Name)
			}
			out = append(out, extensionField{n.Name, doc})
		}
	}
	return out, nil
}

// slotRow is one slot of a core page with the comment beside its constant.
type slotRow struct{ name, kind, doc string }

func deriveSlotRows(src *pkgSource) ([]slotRow, error) {
	var rows []slotRow
	add := func(name, kind string) error {
		c, ok := src.consts[name]
		if !ok || c.doc == "" {
			return fmt.Errorf("gen-extdoc: slot %q has no constant with a doc comment", name)
		}
		rows = append(rows, slotRow{name, kind, firstSentence(c.doc)})
		return nil
	}
	for _, s := range extension.EntrySlots() {
		kind := "button"
		if strings.HasPrefix(string(s), "record_mark.") {
			kind = "small note"
		}
		if err := add(string(s), kind); err != nil {
			return nil, err
		}
	}
	for _, s := range extension.PanelSlots() {
		if err := add(string(s), "card"); err != nil {
			return nil, err
		}
	}
	return rows, nil
}
