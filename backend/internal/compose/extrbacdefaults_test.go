// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/pkg/extension"
)

func verbWithDefaults(tool string, defaults ...extension.RbacDefault) extension.Verb {
	v := unitVerb("crm-demo", tool, extension.TierAutoExecute, extension.ScopeRead)
	v.RbacObject = "ext_crm_demo_widget"
	v.RbacAction = extension.RbacRead
	v.RbacDefaults = defaults
	return v
}

var widgetDefaults = []extension.RbacDefault{
	{Role: "admin", Actions: []extension.RbacAction{extension.RbacRead, extension.RbacCreate}},
	{Role: "read_only", Actions: []extension.RbacAction{extension.RbacRead}},
}

// The defaults a boot applies are the ones the composed verbs declare, one entry
// per (object, role), however many operations share the object.
func TestExtensionRbacDefaultsComeFromTheDeclaredOperations(t *testing.T) {
	first := verbWithDefaults("demo_sync", widgetDefaults...)
	second := verbWithDefaults("demo_read", widgetDefaults...)
	bare := verbWithDefaults("demo_peek")
	ungated := unitVerb("yogi", "yogi_quote", extension.TierAutoExecute, extension.ScopeRead)

	got, err := extensionRbacDefaults([]extension.Verb{first, second, bare, ungated})
	if err != nil {
		t.Fatal(err)
	}
	want := []identity.RbacDefault{
		{Object: "ext_crm_demo_widget", Role: "admin", Actions: []string{"read", "create"}},
		{Object: "ext_crm_demo_widget", Role: "read_only", Actions: []string{"read"}},
	}
	if len(got) != len(want) {
		t.Fatalf("derived %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].Object != want[i].Object || got[i].Role != want[i].Role || strings.Join(got[i].Actions, ",") != strings.Join(want[i].Actions, ",") {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if got, err := extensionRbacDefaults(nil); err != nil || len(got) != 0 {
		t.Fatalf("derived %v (err %v) from no operations", got, err)
	}
}

// Two operations on one object may not state different defaults. The first one
// read would win, and the contract would say two things.
func TestTwoOperationsStatingDifferentDefaultsForOneObjectAreRefused(t *testing.T) {
	first := verbWithDefaults("demo_sync", widgetDefaults...)
	second := verbWithDefaults("demo_read",
		extension.RbacDefault{Role: "admin", Actions: []extension.RbacAction{extension.RbacRead}})

	_, err := extensionRbacDefaults([]extension.Verb{first, second})
	if err == nil {
		t.Fatal("conflicting defaults for one object were accepted")
	}
	for _, want := range []string{"ext_crm_demo_widget", first.OperationID, second.OperationID} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
}

// The role key is the one fact the contract reader cannot check, because the
// role set is identity's. The boot's preflight is where it is refused.
func TestADefaultNamingARoleThatIsNotASystemRoleIsRefusedAtBoot(t *testing.T) {
	v := verbWithDefaults("demo_sync",
		extension.RbacDefault{Role: "field_partner", Actions: []extension.RbacAction{extension.RbacRead}})
	_, err := extensionRbacDefaults([]extension.Verb{v})
	if err == nil || !strings.Contains(err.Error(), "not a system role") {
		t.Fatalf("err = %v, want a refusal naming the role", err)
	}
}
