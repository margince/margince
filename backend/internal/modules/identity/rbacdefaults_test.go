// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestADefaultTheFillCouldNotApplyIsRefusedBeforeAnythingIsRegistered(t *testing.T) {
	const object = "ext_rbacdef_note"
	cases := []struct {
		name     string
		defaults []RbacDefault
		contains string
	}{
		{"a role that is not a system role", []RbacDefault{{Object: object, Role: "field_partner", Actions: []string{"read"}}}, "not a system role"},
		{"an action outside the four", []RbacDefault{{Object: object, Role: "rep", Actions: []string{"write"}}}, `"write"`},
		{"an object outside the extension grammar", []RbacDefault{{Object: "deal", Role: "rep", Actions: []string{"read"}}}, "not a valid extension object"},
		{"a role listed twice for one object", []RbacDefault{
			{Object: object, Role: "rep", Actions: []string{"read"}},
			{Object: object, Role: "rep", Actions: []string{"create"}},
		}, "twice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(ResetRbacObjectsForTest)
			if err := RegisterRbacObjects(object); err != nil {
				t.Fatal(err)
			}
			err := RegisterRbacDefaults(tc.defaults...)
			if err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("RegisterRbacDefaults = %v, want a refusal naming %q", err, tc.contains)
			}
			if got := registeredRbacDefaults(); len(got) != 0 {
				t.Errorf("a refused set left %d defaults registered; the boot must not half-apply", len(got))
			}
		})
	}
}

func TestADefaultOnAnObjectNoUnitRegisteredIsRefused(t *testing.T) {
	t.Cleanup(ResetRbacObjectsForTest)
	err := RegisterRbacDefaults(RbacDefault{Object: "ext_rbacdef_unregistered", Role: "rep", Actions: []string{"read"}})
	if err == nil || !strings.Contains(err.Error(), "no unit registered") {
		t.Fatalf("RegisterRbacDefaults = %v, want a refusal for an unregistered object", err)
	}
}

// The fill writes under no user's authority. A human context and a context
// with no actor are both refused before the database is touched.
func TestTheFillIsRefusedToAnyCallerButTheSystem(t *testing.T) {
	human := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:x"})
	for name, ctx := range map[string]context.Context{
		"a human":  human,
		"no actor": context.Background(),
	} {
		t.Run(name, func(t *testing.T) {
			filled, err := (&Service{}).FillRbacDefaults(ctx)
			if !errors.Is(err, apperrors.ErrPermissionDenied) || filled != 0 {
				t.Fatalf("FillRbacDefaults = (%d, %v), want (0, permission denied)", filled, err)
			}
		})
	}
}

// The document stores all four verbs. An action left out is stored as false.
func TestADefaultSpellsEveryVerbTheDocumentStores(t *testing.T) {
	g, err := RbacDefault{Object: "ext_rbacdef_note", Role: "rep", Actions: []string{"read", "update"}}.grant()
	if err != nil {
		t.Fatal(err)
	}
	if want := (storedGrant{Read: true, Update: true}); g != want {
		t.Fatalf("grant = %+v, want %+v", g, want)
	}
}
