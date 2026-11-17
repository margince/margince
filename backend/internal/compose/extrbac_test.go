// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/pkg/extension"
)

// TestExtensionRbacObjectsComeFromTheDeclaredOperations: the objects a boot
// registers are derived from the composed verb set, never declared in Go — so a
// unit gains a vocabulary entry by declaring x-rbac-object in its fragment and
// no other way.
func TestExtensionRbacObjectsComeFromTheDeclaredOperations(t *testing.T) {
	gated := unitVerb("crm-demo", "demo_sync", extension.TierAutoExecute, extension.ScopeRead)
	gated.RbacObject = "ext_crm_demo_widget"
	gated.RbacAction = extension.RbacRead
	// A second operation on the SAME object: one unit's screens share it, so the
	// registration must be de-duplicated or the boot refuses itself.
	alsoGated := unitVerb("crm-demo", "demo_read", extension.TierAutoExecute, extension.ScopeRead)
	alsoGated.RbacObject = "ext_crm_demo_widget"
	alsoGated.RbacAction = extension.RbacRead
	ungated := unitVerb("yogi", "yogi_quote", extension.TierAutoExecute, extension.ScopeRead)

	got, err := extensionRbacObjects([]extension.Verb{gated, alsoGated, ungated})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "ext_crm_demo_widget" {
		t.Fatalf("derived %v, want exactly [ext_crm_demo_widget]", got)
	}
	// A unit owning no records declares nothing, which is the live tree's state.
	if got, err := extensionRbacObjects([]extension.Verb{ungated}); err != nil || len(got) != 0 {
		t.Fatalf("derived %v (err %v) from an operation declaring no object", got, err)
	}
	if got, err := extensionRbacObjects(nil); err != nil || len(got) != 0 {
		t.Fatalf("derived %v (err %v) from no operations", got, err)
	}
}

// TestTwoUnitsDerivingOneRbacObjectAreRefusedByName: `ext_` + the unit name with
// hyphens underscored is NOT injective — unit `crm` object `demo_widget` and
// unit `crm-demo` object `widget` both derive `ext_crm_demo_widget`, and both
// clear Verb.Validate because each really is inside its own unit's namespace.
// The vocabulary would refuse the second registration naming neither unit, which
// reads as one unit declaring an object twice and sends an operator to the wrong
// file.
func TestTwoUnitsDerivingOneRbacObjectAreRefusedByName(t *testing.T) {
	crm := unitVerb("crm", "crm_sync", extension.TierAutoExecute, extension.ScopeRead)
	crm.RbacObject = "ext_crm_demo_widget"
	crm.RbacAction = extension.RbacRead
	crmDemo := unitVerb("crm-demo", "demo_sync", extension.TierAutoExecute, extension.ScopeRead)
	crmDemo.RbacObject = "ext_crm_demo_widget"
	crmDemo.RbacAction = extension.RbacRead
	// Both are individually valid; that is the whole difficulty.
	for _, v := range []extension.Verb{crm, crmDemo} {
		if err := v.Validate(); err != nil {
			t.Fatalf("%s's declaration must be individually valid: %v", v.Unit, err)
		}
	}

	_, err := extensionRbacObjects([]extension.Verb{crm, crmDemo})
	if err == nil {
		t.Fatal("two units deriving one object name were accepted")
	}
	for _, want := range []string{`"crm"`, `"crm-demo"`, "ext_crm_demo_widget"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %s: %v", want, err)
		}
	}
	// And the boot refuses the pair — EARLIER than this check, and for the
	// reason underneath it. The two unit names that can derive one object name
	// are exactly the two whose namespaces open one another (`ext_crm`,
	// `ext_crm_demo`), and reserveNamespace turns that pair away before any
	// capability is read: the ambiguity was never confined to RBAC objects — an
	// identifier like ext_crm_demo_widget names a table, a ledger subject and an
	// event subject either unit could own. The check above remains the one that
	// catches two units declaring one object they did not both DERIVE.
	_, bootErr := RegisterExtensions(composableAll([]extension.Extension{
		{Name: "crm", Version: "0.1.0"}, {Name: "crm-demo", Version: "0.1.0"},
	}), []extension.Verb{crm, crmDemo}, nil)
	if bootErr == nil || !strings.Contains(bootErr.Error(), "one opens the other") {
		t.Fatalf("boot err = %v, want the overlapping-namespace refusal", bootErr)
	}
	for _, want := range []string{`"crm"`, `"crm-demo"`} {
		if !strings.Contains(bootErr.Error(), want) {
			t.Errorf("the boot refusal does not name %s: %v", want, bootErr)
		}
	}
}

// TestOverlappingUnitNamespacesAreRefused: two units whose namespaces open one
// another cannot both compose, whatever they declare.
//
// It is the hazard the generator's derived-identifier check already names —
// "unit a-b table c and unit a table b_c both derive ext_a_b_c" — seen from the
// other end. That one refuses the two units DECLARING one table; this refuses
// the pair existing at all, because a unit NAMING the ambiguous identifier at
// run time (a ledger row's subject, an event's subject) has no honest owner and
// nothing downstream holds the split.
func TestOverlappingUnitNamespacesAreRefused(t *testing.T) {
	for name, set := range map[string][]extension.Extension{
		"a hyphen that becomes an underscore": {
			{Name: "crm", Version: "0.1.0"}, {Name: "crm-demo", Version: "0.1.0"},
		},
		// Declaration order must not decide it: the longer name first is the
		// same clash read the other way round.
		"the longer name first": {
			{Name: "crm-demo", Version: "0.1.0"}, {Name: "crm", Version: "0.1.0"},
		},
	} {
		if _, err := RegisterExtensions(composableAll(set), nil, nil); err == nil {
			t.Errorf("%s: the boot composed two units whose namespaces overlap", name)
		}
	}

	// Names that merely SHARE A PREFIX compose fine — `ext_crmdemo_x` can never
	// be read as a table of `ext_crm`, because every derivation joins with an
	// underscore. A refusal here would be this check overreaching.
	if _, err := RegisterExtensions(composableAll([]extension.Extension{
		{Name: "crm", Version: "0.1.0"}, {Name: "crmdemo", Version: "0.1.0"},
	}), nil, nil); err != nil {
		t.Errorf("two units with distinct namespaces were refused: %v", err)
	}
}

func TestNewRbacObjectsWidensTheVocabulary(t *testing.T) {
	t.Parallel()
	const object = "ext_crm_demo_widget"
	if (identity.RbacObjects{}).Grantable(object) {
		t.Fatal("the object is grantable in a vocabulary that does not register it")
	}
	rbac, err := NewRbacObjects([]identity.RbacObject{object}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rbac.Grantable(object) {
		t.Fatal("a registered object is not grantable, so an authority requirement naming it can never be satisfied")
	}
	// Empty is not an error: it is what most units compose to, and a boot must
	// not have to special-case it.
	if _, err := NewRbacObjects(nil, nil); err != nil {
		t.Fatalf("NewRbacObjects(nil) = %v, want nil", err)
	}
}

// TestNewRbacObjectsRefusesANameOutsideTheNamespace: the refusal is the
// module's own (identity's internal policy grammar), and it has to arrive here
// wrapped rather than swallowed. A boot that registered "deal" would let a
// dropped-in directory widen what a core role document grants.
func TestNewRbacObjectsRefusesANameOutsideTheNamespace(t *testing.T) {
	t.Parallel()
	rbac, err := NewRbacObjects([]identity.RbacObject{"ext_crm_demo_widget", "deal"}, nil)
	if err == nil || !strings.Contains(err.Error(), "compose:") {
		t.Fatalf("err = %v, want a compose-prefixed refusal", err)
	}
	if rbac.Grantable("ext_crm_demo_widget") {
		t.Fatal("a name in the failing set registered anyway; the vocabulary is not validate-then-apply")
	}
}

// TestRegisterExtensionsRegistersTheDeclaredObjects is the boot path end to end.
// Nothing else builds a composition's vocabulary. Without it the vocabulary
// would silently be core-only again.
func TestRegisterExtensionsRegistersTheDeclaredObjects(t *testing.T) {
	gated := unitVerb("crm-demo", "demo_sync", extension.TierAutoExecute, extension.ScopeRead)
	gated.RbacObject = "ext_crm_demo_widget"
	gated.RbacAction = extension.RbacRead
	composed, err := RegisterExtensions(composableAll([]extension.Extension{{Name: "crm-demo", Version: "0.1.0"}}), []extension.Verb{gated}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !composed.RbacObjects().Grantable("ext_crm_demo_widget") {
		t.Fatal("the boot did not register the object its composed set declares")
	}
	if !installedRbacObjects().Grantable("ext_crm_demo_widget") {
		t.Fatal("the identity services built without the composition do not see the object it declares")
	}
	// And the verb set is recorded, because the route mounting reads it later —
	// after the Server is assembled, which is after this ran.
	if got := composed.Verbs(); len(got) != 1 || got[0].Tool != "demo_sync" {
		t.Fatalf("Verbs() = %v, want the registered set", got)
	}
}

// TestRegisterExtensionsRefusesAnObjectOutsideTheNamespace: the boot aborts
// rather than serving a set whose vocabulary it could not apply.
func TestRegisterExtensionsRefusesAnObjectOutsideTheNamespace(t *testing.T) {
	// Verb.Validate refuses a CROSS-UNIT object, so the escape this checks is
	// the one it cannot see: a name correctly inside the unit's namespace that
	// the VOCABULARY still refuses (a doubled underscore is not a legal SQL
	// identifier segment). The two rules are owned by different packages on
	// purpose, and this is the case that proves the second one still runs.
	squatting := unitVerb("crm-demo", "demo_sync", extension.TierAutoExecute, extension.ScopeRead)
	squatting.RbacObject = "ext_crm_demo__widget"
	composed, err := RegisterExtensions(composableAll([]extension.Extension{{Name: "crm-demo", Version: "0.1.0"}}), []extension.Verb{squatting}, nil)
	if err == nil {
		t.Fatal("the boot accepted an object the vocabulary refuses")
	}
	if composed != nil {
		t.Fatal("the boot failed and still answered a composition")
	}
}
