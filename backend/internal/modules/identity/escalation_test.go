// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The administration objects are named through their constants, never as bare
// string literals at a call site.
//
// A misspelt literal is the defect worth a gate: `auth.Require(ctx, "user_admn",
// …)` compiles, resolves to the zero grant, and refuses every caller. In
// production that reads as a feature that stopped working rather than as a typo,
// and the refusal it produces is indistinguishable from a correct one.
//
// The constants make the same mistake a compile error — but only for code that
// uses them, which is why this fails on the literal instead of trusting that
// nobody will write one.
func TestIdentityNamesItsRbacObjectsThroughConstants(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing the package: %v", err)
	}
	// The declarations themselves are where the strings belong.
	const owner = "escalation.go"
	literal := regexp.MustCompile(`"(user_admin|role_admin|team_admin)"`)

	var found []string
	for _, name := range sources {
		if name == owner {
			continue
		}
		body, readErr := os.ReadFile(name)
		if readErr != nil {
			t.Fatalf("reading %s: %v", name, readErr)
		}
		for _, hit := range literal.FindAllString(string(body), -1) {
			found = append(found, name+": "+hit)
		}
	}
	if len(found) > 0 {
		t.Errorf("an administration object is named as a bare string literal at %v — use the "+
			"objectUserAdmin/objectRoleAdmin/objectTeamAdmin constants in %s, so a misspelling is a "+
			"compile error rather than a gate that silently refuses everybody", found, owner)
	}

	// The corpus has to contain the declarations, or this scan is reading a
	// package that no longer holds them and reports PASS either way.
	body, err := os.ReadFile(owner)
	if err != nil {
		t.Fatalf("reading %s: %v", owner, err)
	}
	for _, name := range []string{"objectUserAdmin", "objectRoleAdmin", "objectTeamAdmin"} {
		if !regexp.MustCompile(name + `\s*= "`).Match(body) {
			t.Errorf("%s no longer declares %s, so the literals this test forbids have nowhere "+
				"to come from and it is guarding nothing", owner, name)
		}
	}
}

// A role-editor holder may only turn ON a verb they already hold.
//
// The guard this exercises is what stops the editor being a ladder: a delegated
// role_admin.update holder who could write a verb they lack would grant it to
// whichever role they can already be assigned, and hold it by proxy a moment
// later. Unit-lane because the comparison is between two values in hand — no
// stored document decides it, which is itself the property worth pinning.
func TestTheRoleEditorRefusesAVerbTheCallerDoesNotHold(t *testing.T) {
	delegated := Identity{
		Roles: []string{"custom"},
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				// Holds read and update on people, and nothing else.
				"person": {Read: true, Update: true},
			},
		},
	}
	literalAdmin := Identity{Roles: []string{roleAdmin}}

	for _, tt := range []struct {
		name    string
		actor   Identity
		object  string
		grant   storedGrant
		refused bool
	}{
		{"a verb the caller holds", delegated, "person", storedGrant{Read: true}, false},
		{"both verbs the caller holds", delegated, "person", storedGrant{Read: true, Update: true}, false},
		{"a verb the caller lacks", delegated, "person", storedGrant{Delete: true}, true},
		{"create, which the caller lacks", delegated, "person", storedGrant{Create: true}, true},
		{"one held verb beside one lacked", delegated, "person", storedGrant{Read: true, Delete: true}, true},
		// An object the caller holds nothing on at all is the same question with
		// every verb missing, and the commonest shape of the mistake.
		{"any verb on an unheld object", delegated, "system_reset", storedGrant{Delete: true}, true},
		// Turning everything OFF grants nobody anything, so it is allowed even
		// on an object the caller cannot write. Narrowing is not escalation.
		{"the empty grant on an unheld object", delegated, "system_reset", storedGrant{}, false},
		// The literal admin is its own ceiling and skips the comparison.
		{"the literal admin writes anything", literalAdmin, "system_reset", storedGrant{Delete: true}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := refuseUnlessCallerHoldsGrant(tt.actor, tt.object, tt.grant)
			if tt.refused && !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("writing %+v on %q = %v, want permission denied — the editor would "+
					"hand its holder a verb they do not have", tt.grant, tt.object, err)
			}
			if !tt.refused && err != nil {
				t.Errorf("writing %+v on %q = %v, want admitted", tt.grant, tt.object, err)
			}
		})
	}
}
