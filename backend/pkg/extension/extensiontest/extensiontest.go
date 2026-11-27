// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package extensiontest checks a unit's record kinds, link types and signal
// types in the unit's own tests, by the rules core's boot applies. A unit
// learns of a declaration core refuses from its test run, not from a server
// that will not start.
//
//margince:extension-surface
package extensiontest

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// Check fails the test when core's boot would refuse the unit's record kinds,
// link types or signal types, with the boot's own message.
func Check(t testing.TB, ext extension.Extension) {
	t.Helper()
	if err := ext.ValidateDeclarations(); err != nil {
		t.Errorf("extension %q: %v", ext.Name, err)
	}
}

// CheckSignal fails the test when the unit declares no signal type kind. It
// also fails when core would refuse fields as the evidence of such a signal.
func CheckSignal(t testing.TB, ext extension.Extension, kind string, fields map[string]any) {
	t.Helper()
	i := slices.IndexFunc(ext.SignalTypes, func(d extension.SignalTypeDeclaration) bool { return d.Kind == kind })
	if i < 0 {
		t.Errorf("extension %q declares no signal type %q", ext.Name, kind)
		return
	}
	if err := ext.SignalTypes[i].CheckFields(fields); err != nil {
		t.Errorf("extension %q: %v", ext.Name, err)
	}
}

// CheckLink fails the test when the unit declares no link type linkType. It
// also fails when a link of it may not start on from and end on to.
func CheckLink(t testing.TB, ext extension.Extension, linkType, from, to string) {
	t.Helper()
	i := slices.IndexFunc(ext.LinkTypes, func(l extension.LinkType) bool { return l.Type == linkType })
	if i < 0 {
		t.Errorf("extension %q declares no link type %q", ext.Name, linkType)
		return
	}
	if !ext.LinkTypes[i].Joins(from, to) {
		t.Errorf("extension %q: link type %q does not join a %s to a %s", ext.Name, linkType, from, to)
	}
}
