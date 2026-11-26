// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package recordkinds

import (
	"slices"
	"sort"

	"github.com/margince/margince/backend/pkg/extension"
)

// LinkType is one registered way two core records relate.
type LinkType struct {
	// Name is the stored and wire form: a core word, or `ext:<unit>:<type>`.
	Name string
	// Unit is the extension that declared it, empty for core's own.
	Unit  extension.Name
	Label string
	// From and To are never empty: the declaration's empty list is filled in
	// with every end at registration.
	From, To []string
}

// IsLinkEnd reports whether a link may start or end on a record of this kind.
func IsLinkEnd(kind string) bool { return slices.Contains(extension.LinkEnds, kind) }

// Allows reports whether the type may join a record of kind from to one of kind to.
func (t LinkType) Allows(from, to string) bool {
	return slices.Contains(t.From, from) && slices.Contains(t.To, to)
}

// coreLinkTypes are the types core declares. They sit in the same registry an
// extension's do, so a screen, a filter and a refusal ask one place.
var coreLinkTypes = []LinkType{
	{Name: "follow_up_of", Label: "Follow-up of"},
	{Name: "renews", Label: "Renews"},
	{Name: "related_to", Label: "Related to"},
}

var linkTypes = map[string]LinkType{}

func init() {
	for _, t := range coreLinkTypes {
		t.From, t.To = extension.LinkEnds, extension.LinkEnds
		linkTypes[t.Name] = t
	}
}

// RegisterLinkTypes records one unit's types. A unit declaring one twice is
// refused at boot, before this runs.
func RegisterLinkTypes(unit extension.Name, types []extension.LinkType) {
	mu.Lock()
	defer mu.Unlock()
	for _, t := range types {
		name := extension.LinkTypeValue(unit, t.Type)
		linkTypes[name] = LinkType{
			Name: name, Unit: unit, Label: t.Label,
			From: endsOrAll(t.From), To: endsOrAll(t.To),
		}
	}
}

func endsOrAll(ends []string) []string {
	if len(ends) == 0 {
		return extension.LinkEnds
	}
	return ends
}

// LookupLinkType answers a type by name, and false for one nobody declared.
func LookupLinkType(name string) (LinkType, bool) {
	mu.RLock()
	defer mu.RUnlock()
	t, ok := linkTypes[name]
	return t, ok
}

// LinkTypes lists core's types in their declared order, then each extension's by name.
func LinkTypes() []LinkType {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]LinkType, 0, len(linkTypes))
	for _, t := range coreLinkTypes {
		out = append(out, linkTypes[t.Name])
	}
	var declared []LinkType
	for _, t := range linkTypes {
		if t.Unit != "" {
			declared = append(declared, t)
		}
	}
	sort.Slice(declared, func(i, j int) bool { return declared[i].Name < declared[j].Name })
	return append(out, declared...)
}

// resetLinkTypes drops every extension's type and keeps core's. The caller holds mu.
func resetLinkTypes() {
	for name, t := range linkTypes {
		if t.Unit != "" {
			delete(linkTypes, name)
		}
	}
}
