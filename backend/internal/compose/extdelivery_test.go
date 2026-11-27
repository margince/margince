// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The parts of a delivery's identity, the roster door and the shared custom
// value choice that hold with no database. The database lane covers the rest.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

func heldNone(context.Context, map[string]any) ([]string, error) { return nil, nil }

func TestOnlyAValueThatDiffersIsWrittenAndAUsersValueIsNamed(t *testing.T) {
	current := map[string]any{"cf_a": "same", "cf_b": "old", "cf_c": "typed"}
	offered := map[string]any{"cf_a": "same", "cf_b": "new", "cf_c": "offered", "cf_d": []string{"x"}}
	userOwns := func(_ context.Context, patch map[string]any) ([]string, error) {
		if _, unchanged := patch["cf_a"]; unchanged {
			t.Error("an unchanged value was put to the ownership check")
		}
		return []string{"cf_c"}, nil
	}
	write, withheld, err := customValuesToWrite(context.Background(), current, offered, userOwns)
	if err != nil {
		t.Fatal(err)
	}
	if len(write) != 2 || write["cf_b"] != "new" || write["cf_d"] == nil {
		t.Errorf("write = %v, want cf_b and cf_d only", write)
	}
	if !slices.Equal(withheld, []string{"cf_c"}) {
		t.Errorf("withheld = %v, want cf_c", withheld)
	}
}

func TestARepeatOfTheStoredValuesWritesNothing(t *testing.T) {
	stored := map[string]any{"cf_tags": []any{"a", "b"}}
	write, withheld, err := customValuesToWrite(context.Background(), stored, map[string]any{"cf_tags": []string{"a", "b"}}, heldNone)
	if err != nil || len(write) != 0 || len(withheld) != 0 {
		t.Errorf("write = %v, withheld = %v, err = %v, want nothing for a repeat", write, withheld, err)
	}
}

func TestTheRosterRefusesAUnitThatDeclaredNoNeed(t *testing.T) {
	composeUnit(t, extension.Extension{Name: "plain", Version: "1.0.0"})
	_, err := extensionCore{unit: "plain"}.Members().List(context.Background(), extension.MemberQuery{Role: "admin"})
	if !errors.Is(err, extension.ErrNotDeclared) {
		t.Fatalf("err = %v, want ErrNotDeclared", err)
	}
}

func TestTheRosterRefusesATickEvenForAUnitThatDeclaredTheNeed(t *testing.T) {
	composeUnit(t, extension.Extension{
		Name: "roster", Version: "1.0.0",
		Needs: []extension.Need{{Capability: extension.CapabilityMembers, Reason: "Finds an administrator."}},
	})
	_, err := extensionCore{unit: "roster", unattended: true}.Members().List(context.Background(), extension.MemberQuery{})
	if !errors.Is(err, extension.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestAnUnattendedPortOpensOnlyForADeliveryRunningAsAServiceMember(t *testing.T) {
	if err := (extensionCore{unattended: true}).refuseUnattended(); !errors.Is(err, extension.ErrForbidden) {
		t.Errorf("unattended with no member: err = %v, want ErrForbidden", err)
	}
	if err := (extensionCore{unattended: true, actsAsSeat: true}).refuseUnattended(); err != nil {
		t.Errorf("a delivery as a service member: err = %v, want the port open", err)
	}
}

func TestACallerNamesTheServiceMemberOnlyForADeliveryRunningAsOne(t *testing.T) {
	member := ids.NewV7()
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, UserID: member})
	rt := unattendedRuntimeFor(ctx, "unit", "1.0.0", "subscription/x", extensionRuntimeBinding{})
	if rt.Caller() != (extension.Caller{}) {
		t.Errorf("caller = %+v, want nobody for an unattended run", rt.Caller())
	}
	rt.actsAsSeat = true
	if got := rt.Caller(); got.Type != extension.CallerHuman || got.UserID != member.String() {
		t.Errorf("caller = %+v, want the service member %s", got, member)
	}
}
