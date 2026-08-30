// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension"
)

// Pack codes in this file use ISO user-assigned codes (z*) unique per
// test: the jurisdiction registry is process-global, so a code
// registered here stays registered for the test binary's lifetime.
type fakePack struct {
	code    jurisdiction.Code
	classes []jurisdiction.RetentionClass
}

func (p fakePack) Code() jurisdiction.Code { return p.code }

func (p fakePack) Retention() jurisdiction.Retention {
	if p.classes == nil {
		return nil
	}
	return fakeRetention{classes: p.classes}
}

type fakeRetention struct{ classes []jurisdiction.RetentionClass }

func (r fakeRetention) Classes() []jurisdiction.RetentionClass { return r.classes }

// composable fills the fields the boot requires of EVERY unit but which most
// tests here are not about — a name is the subject of one test, a description
// of none. It leaves whatever the caller set: a fixture testing the refusal of
// an empty name or version passes its own and gets it back unchanged.
//
// Without it each of these tests would restate a sentence it does not care
// about, and the next required field would mean editing all of them again.
func composable(e extension.Extension) extension.Extension {
	if e.Version == "" {
		e.Version = "0.0.1"
	}
	if e.Description == "" {
		e.Description = "A unit composed by a test."
	}
	return e
}

// composableAll applies composable to a whole composed set.
func composableAll(exts []extension.Extension) []extension.Extension {
	out := make([]extension.Extension, 0, len(exts))
	for _, e := range exts {
		out = append(out, composable(e))
	}
	return out
}

func TestRegisterExtensionsAppliesDeclaredCapabilities(t *testing.T) {
	err := RegisterExtensions(composableAll([]extension.Extension{{
		Name:          "reg-ok",
		Version:       "0.0.1",
		Jurisdictions: []jurisdiction.Pack{fakePack{code: "zx"}},
	}}), nil, nil)
	if err != nil {
		t.Fatalf("RegisterExtensions: %v", err)
	}
	if _, ok := jurisdiction.For("zx"); !ok {
		t.Fatal("the declared pack did not reach the jurisdiction registry")
	}
}

func TestRegisterExtensionsRejectsAnInvalidUnitName(t *testing.T) {
	err := RegisterExtensions(composableAll([]extension.Extension{{Name: "Bad_Name"}}), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not a valid unit name") {
		t.Fatalf("err = %v, want the unit-name rejection", err)
	}
}

func TestRegisterExtensionsRejectsADuplicateUnit(t *testing.T) {
	err := RegisterExtensions(composableAll([]extension.Extension{
		{Name: "twice", Version: "0.0.1"},
		{Name: "twice", Version: "0.0.2"},
	}), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "composed twice") {
		t.Fatalf("err = %v, want the duplicate-unit rejection", err)
	}
}

func TestRegisterExtensionsRejectsAMissingVersion(t *testing.T) {
	// NOT through composable: the empty version is this test's subject, and a
	// helper that filled it would assert the refusal of a unit that has one.
	err := RegisterExtensions([]extension.Extension{
		{Name: "unversioned", Description: "A unit composed by a test."},
	}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "version is empty") {
		t.Fatalf("err = %v, want the missing-version rejection", err)
	}
}

func TestRegisterExtensionsPreflightsDuplicateJurisdictions(t *testing.T) {
	err := RegisterExtensions(composableAll([]extension.Extension{
		{Name: "first", Version: "0.0.1", Jurisdictions: []jurisdiction.Pack{fakePack{code: "zv"}}},
		{Name: "second", Version: "0.0.1", Jurisdictions: []jurisdiction.Pack{fakePack{code: "zv"}}},
	}), nil, nil)
	if err == nil || !strings.Contains(err.Error(), `both declare jurisdiction "zv"`) {
		t.Fatalf("err = %v, want the duplicate-jurisdiction rejection", err)
	}
	if _, ok := jurisdiction.For("zv"); ok {
		t.Fatal("a duplicate-declared pack landed although the set was rejected")
	}
}

// TestRegisterExtensionsRejectsAnUnknownRetentionClass: the class set is
// closed (vocabulary registration is deferred, ADR-0069 §13) — a typo'd
// or invented class would be a statutory floor that looks registered
// while no engine ever consults it, so the boot refuses it.
func TestRegisterExtensionsRejectsAnUnknownRetentionClass(t *testing.T) {
	err := RegisterExtensions(composableAll([]extension.Extension{{
		Name:    "typo-floor",
		Version: "0.0.1",
		Jurisdictions: []jurisdiction.Pack{fakePack{
			code:    "zu",
			classes: []jurisdiction.RetentionClass{{Name: "comercial_correspondence", Keep: jurisdiction.Period{Years: 6}}},
		}},
	}}), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not in the closed class set") {
		t.Fatalf("err = %v, want the closed-set rejection", err)
	}
	if _, ok := jurisdiction.For("zu"); ok {
		t.Fatal("the pack landed although its retention class failed validation")
	}
}

// TestRegisterExtensionsRejectsANegativePeriod: a negative floor would
// anchor its cutoff in the future and silently shrink statutory
// protection — the boot refuses the set.
func TestRegisterExtensionsRejectsANegativePeriod(t *testing.T) {
	err := RegisterExtensions(composableAll([]extension.Extension{{
		Name:    "future-floor",
		Version: "0.0.1",
		Jurisdictions: []jurisdiction.Pack{fakePack{
			code:    "zt",
			classes: []jurisdiction.RetentionClass{{Name: jurisdiction.CommercialCorrespondence, Keep: jurisdiction.Period{Years: -6}}},
		}},
	}}), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "negative component") {
		t.Fatalf("err = %v, want the negative-component rejection", err)
	}
	if _, ok := jurisdiction.For("zt"); ok {
		t.Fatal("the pack landed although its floor failed validation")
	}
}

// TestRegisterExtensionsRejectsAnUnknownAnchor: anchors are a closed
// set — an invented anchor would be a floor whose start the engine
// silently misreads.
func TestRegisterExtensionsRejectsAnUnknownAnchor(t *testing.T) {
	err := RegisterExtensions(composableAll([]extension.Extension{{
		Name:    "odd-anchor",
		Version: "0.0.1",
		Jurisdictions: []jurisdiction.Pack{fakePack{
			code:    "zs",
			classes: []jurisdiction.RetentionClass{{Name: jurisdiction.CommercialCorrespondence, Keep: jurisdiction.Period{Years: 6}, Anchor: "fiscal_year_end"}},
		}},
	}}), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "closed anchor set") {
		t.Fatalf("err = %v, want the closed-anchor rejection", err)
	}
	if _, ok := jurisdiction.For("zs"); ok {
		t.Fatal("the pack landed although its anchor failed validation")
	}
}

// TestRegisterExtensionsRejectsADuplicateRetentionClass: one class, one
// floor — a pack declaring the same class twice with different
// Keep/Anchor values would leave the engine picking one silently.
func TestRegisterExtensionsRejectsADuplicateRetentionClass(t *testing.T) {
	err := RegisterExtensions(composableAll([]extension.Extension{{
		Name:    "double-floor",
		Version: "0.0.1",
		Jurisdictions: []jurisdiction.Pack{fakePack{
			code: "zr",
			classes: []jurisdiction.RetentionClass{
				{Name: jurisdiction.CommercialCorrespondence, Keep: jurisdiction.Period{Years: 6}},
				{Name: jurisdiction.CommercialCorrespondence, Keep: jurisdiction.Period{Years: 8}, Anchor: jurisdiction.AnchorCalendarYearEnd},
			},
		}},
	}}), nil, nil)
	if err == nil || !strings.Contains(err.Error(), `declares retention class "commercial_correspondence" twice`) {
		t.Fatalf("err = %v, want the duplicate-class rejection", err)
	}
	if _, ok := jurisdiction.For("zr"); ok {
		t.Fatal("the pack landed although its retention classes failed validation")
	}
}

// TestRegisterExtensionsPreflightsTools: a governed tool reaching boot
// (even outside the generator path) is validated through the published
// Tool.Validate, and a verb declared twice in one unit is rejected — the
// fail-closed boundary holds at boot, not only at gen time.
func TestRegisterExtensionsPreflightsTools(t *testing.T) {
	badTier := RegisterExtensions(composableAll([]extension.Extension{{
		Name:    "bad-tier-tool",
		Version: "0.0.1",
		Tools:   []extension.Tool{{Name: "ping", Handle: servedHandle}},
	}}), []extension.Verb{unitVerb("bad-tier-tool", "ping", "dynamic", extension.ScopeRead)}, nil)
	if badTier == nil || !strings.Contains(badTier.Error(), "not one an extension may request") {
		t.Fatalf("err = %v, want the tier rejection", badTier)
	}

	dup := RegisterExtensions(composableAll([]extension.Extension{{
		Name:    "dup-tool",
		Version: "0.0.1",
		Tools: []extension.Tool{
			{Name: "ping", Handle: servedHandle},
			{Name: "ping"},
		},
	}}), []extension.Verb{unitVerb("dup-tool", "ping", extension.TierAutoExecute, extension.ScopeRead)}, nil)
	if dup == nil || !strings.Contains(dup.Error(), `declares tool "ping" twice`) {
		t.Fatalf("err = %v, want the duplicate-tool rejection", dup)
	}

	// And the direction that only exists now that governance is contract-borne:
	// behavior for a verb NO contract operation declares. The generator refuses
	// it at the declaration's line; a set assembled outside that path has to be
	// refused here, or a capability nothing published would be registered into
	// the same registry the core tools ride.
	undeclared := RegisterExtensions(composableAll([]extension.Extension{{
		Name:    "undeclared-tool",
		Version: "0.0.1",
		Tools:   []extension.Tool{{Name: "ping", Handle: servedHandle}},
	}}), nil, nil)
	if undeclared == nil || !strings.Contains(undeclared.Error(), "no operation in its contract fragments declares it") {
		t.Fatalf("err = %v, want the undeclared-behavior rejection", undeclared)
	}
}

// TestNoCapabilityAppliesWhenTheSetIsInvalid: validation and application
// are separate phases — a clean unit's capabilities must not land when a
// later unit fails validation, or a crash-looping process would leave
// half-registered state behind.
func TestNoCapabilityAppliesWhenTheSetIsInvalid(t *testing.T) {
	err := RegisterExtensions(composableAll([]extension.Extension{
		{Name: "clean", Version: "0.0.1", Jurisdictions: []jurisdiction.Pack{fakePack{code: "zy"}}},
		{Name: "Invalid Name"},
	}), nil, nil)
	if err == nil {
		t.Fatal("RegisterExtensions succeeded, want the invalid unit to abort it")
	}
	if _, ok := jurisdiction.For("zy"); ok {
		t.Fatal("the clean unit's pack landed although the composed set failed validation")
	}
}
