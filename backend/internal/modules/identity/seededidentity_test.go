// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/identity/internal/policy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seededIdentity builds a caller holding exactly what the named seeded roles
// hold — resolved through policy.Parse and policy.Merge, the same two functions
// the login path runs.
//
// Written because `Identity{Roles: []string{"admin"}}` stopped describing an
// admin. While these surfaces gated on the role NAME that fixture was complete;
// now they gate on a grant, and an identity carrying a name and no permissions
// is a caller who holds nothing — so a test meaning "an admin asks" silently
// became "a stranger asks", and every assertion about what happens next was
// answered by the refusal instead of by the behaviour.
//
// Seeded through the real documents rather than a hand-built grant map for the
// reason the rulebook gives: a fixture that supplies its own version of
// production proves nothing about production. A grant map written here would
// keep passing after the seed stopped granting it.
func seededIdentity(t *testing.T, roles ...string) Identity {
	t.Helper()
	byRole := make(map[string]policy.Document, len(roles))
	for _, key := range roles {
		doc, err := policy.Parse(policy.MustDefaultJSON(key))
		if err != nil {
			t.Fatalf("the seeded document for %q does not parse: %v", key, err)
		}
		byRole[key] = doc
	}
	return Identity{
		UserID:      ids.UserID{UUID: ids.NewV7()},
		Roles:       roles,
		SeatType:    string(principal.SeatFull),
		Permissions: policy.Merge(byRole),
	}
}
