// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package policy

import (
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// verbOrder is the order the role editor draws its switches in: Read first,
// because the other verbs depend on it.
var verbOrder = []principal.Action{
	principal.ActionRead, principal.ActionCreate, principal.ActionUpdate, principal.ActionDelete,
}

// OfferedVerbs lists the verbs a role may hold on an object, in editor order.
//
// A core object offers the verbs some seeded role holds on it. The seeds leave
// out a verb the product has no surface for. A test holds every literal gate
// check in the code inside the offered set.
// An extension object offers all four, because its unit's own gate decides.
func OfferedVerbs(object string) []principal.Action {
	if !IsCoreObject(object) {
		return slices.Clone(verbOrder)
	}
	var union grant
	for _, doc := range defaults {
		g := doc.Objects[object]
		union.Create = union.Create || g.Create
		union.Read = union.Read || g.Read
		union.Update = union.Update || g.Update
		union.Delete = union.Delete || g.Delete
	}
	held := map[principal.Action]bool{
		principal.ActionRead: union.Read, principal.ActionCreate: union.Create,
		principal.ActionUpdate: union.Update, principal.ActionDelete: union.Delete,
	}
	var out []principal.Action
	for _, verb := range verbOrder {
		if held[verb] {
			out = append(out, verb)
		}
	}
	return out
}

// ShippedGrants is the grant map a system role ships with, core objects only.
// The second result is false for a key that is not seeded.
func ShippedGrants(roleKey string) (map[string]principal.ObjectGrant, bool) {
	doc, ok := defaults[roleKey]
	if !ok {
		return nil, false
	}
	out := make(map[string]principal.ObjectGrant, len(doc.Objects))
	for object, g := range doc.Objects {
		out[object] = principal.ObjectGrant(g)
	}
	return out, true
}

// RequiresRead reports whether a write verb on the object needs Read with it.
// An object that offers no Read, such as the installation reset, is exempt.
func RequiresRead(object string) bool {
	return slices.Contains(OfferedVerbs(object), principal.ActionRead)
}

// OfferedVerbsByObject is OfferedVerbs for every object a role may be granted
// under vocabulary.
func OfferedVerbsByObject(vocabulary Vocabulary) map[string][]principal.Action {
	objects := append(slices.Clone(coreObjects), vocabulary.Registered()...)
	out := make(map[string][]principal.Action, len(objects))
	for _, object := range objects {
		out[object] = OfferedVerbs(object)
	}
	return out
}
