// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a derivation predicate may name, and what it is told when it names
// something else.
//
// A handle minted from a plan is edited by hand — it is a URL — so a refusal
// here is read by somebody who can act on it, and the vocabulary is the whole
// value of the sentence. Two resolvers, because a predicate that pins a VALUE
// and one that asserts a column is UNSET admit different things: a threshold is
// a comparison over a number and cannot be unset.

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
)

// The report that carries a threshold, which is the case the two resolvers
// disagree about. Named once so a spec rename fails here rather than silently
// testing a report with no thresholds and proving nothing.
func quietProjectsSpec(t *testing.T) reportSpec {
	t.Helper()
	spec, ok := prebuiltReports["projects-gone-quiet"]
	if !ok {
		t.Fatal("projects-gone-quiet is absent from the catalog; these cases need a report with a threshold")
	}
	if len(spec.thresholds) == 0 {
		t.Fatal("projects-gone-quiet carries no threshold any more, so the unset case below proves nothing")
	}
	return spec
}

// A THRESHOLD CANNOT BE UNSET, and the refusal says so instead of letting the
// call through to die later.
//
// The reported defect: an isNull predicate carrying a threshold compiled,
// because derivationWhere switches on `threshold != nil` BEFORE `isNull` — so
// the flag was never read, the threshold arm ran with an empty value, and the
// caller was told to "send it as a number" about a value they had not sent.
func TestUnsettingAThresholdIsRefusedWithTheKeysThatCanBeUnset(t *testing.T) {
	t.Parallel()
	spec := quietProjectsSpec(t)
	threshold := slices.Sorted(maps.Keys(spec.thresholds))[0]

	_, err := resolveNullPredicate(spec, threshold)

	var refusal *FieldNotAllowedError
	if !errors.As(err, &refusal) {
		t.Fatalf("unsetting the threshold %q answered %v, not a field refusal", threshold, err)
	}
	if refusal.Slot != nullPredicateKey {
		t.Errorf("the refusal names the slot %q; a handle's own argument is %q",
			refusal.Slot, nullPredicateKey)
	}
	if slices.Contains(refusal.Allowed, threshold) {
		t.Errorf("the refusal lists %q among the keys that CAN be unset, which is the name it just "+
			"refused: %v", threshold, refusal.Allowed)
	}
	if len(refusal.Allowed) == 0 {
		t.Error("the refusal named no keys at all, so a caller has nothing to pick from")
	}
}

// A dimension and a filter can both be unset, and both resolve to the column
// the predicate binds.
func TestUnsettingADimensionOrAFilterResolves(t *testing.T) {
	t.Parallel()
	spec := quietProjectsSpec(t)

	for _, name := range []string{
		slices.Sorted(maps.Keys(spec.dimensions))[0],
		slices.Sorted(maps.Keys(spec.filters))[0],
	} {
		pred, err := resolveNullPredicate(spec, name)
		if err != nil {
			t.Fatalf("unsetting %q was refused: %v", name, err)
		}
		if !pred.isNull {
			t.Errorf("unsetting %q produced a predicate that does not assert absence: %+v", name, pred)
		}
		if pred.field != name {
			t.Errorf("the predicate names %q, not the %q it was asked for", pred.field, name)
		}
		if pred.threshold != nil {
			t.Errorf("unsetting %q carried a threshold, which derivationWhere reads FIRST and would "+
				"bind instead of the absence", name)
		}
	}
}

// A VALUE predicate admits the threshold the unset one refuses, and that
// asymmetry is why the vocabularies differ.
func TestAValuePredicateAdmitsAThresholdAndNamesTheWiderSet(t *testing.T) {
	t.Parallel()
	spec := quietProjectsSpec(t)
	threshold := slices.Sorted(maps.Keys(spec.thresholds))[0]

	pred, err := resolveValuePredicate(spec, threshold, "30")
	if err != nil {
		t.Fatalf("a value over the threshold %q was refused: %v", threshold, err)
	}
	if pred.threshold == nil {
		t.Errorf("the predicate over %q carries no threshold, so nothing would compare the number", threshold)
	}

	// And the two vocabularies differ by exactly the thresholds, which is the
	// claim that makes two sets worth having rather than one.
	value, nullable := allowedPredicateNames(spec), allowedNullableNames(spec)
	if !slices.Contains(value, threshold) {
		t.Errorf("a value predicate's vocabulary omits the threshold %q it accepts: %v", threshold, value)
	}
	if slices.Contains(nullable, threshold) {
		t.Errorf("the unset vocabulary lists the threshold %q, which resolveNullPredicate refuses — "+
			"naming a name the resolver rejects is the same defect as naming none: %v", threshold, nullable)
	}
	for _, name := range nullable {
		if !slices.Contains(value, name) {
			t.Errorf("%q can be unset but is not in the value vocabulary, so the two sets are not "+
				"nested the way thresholds-only-wider says they are", name)
		}
	}
}

// An unknown name is refused with the vocabulary, not with "use a name this
// report declares" and no names — which is what the three sites did before.
func TestAnUnknownPredicateIsRefusedWithTheVocabulary(t *testing.T) {
	t.Parallel()
	spec := quietProjectsSpec(t)

	for _, tc := range []struct {
		what   string
		refuse func() error
		slot   string
	}{
		{"a value predicate", func() error {
			_, err := resolveValuePredicate(spec, "not_a_field", "x")
			return err
		}, ""},
		{"an unset predicate", func() error {
			_, err := resolveNullPredicate(spec, "not_a_field")
			return err
		}, nullPredicateKey},
	} {
		t.Run(tc.what, func(t *testing.T) {
			t.Parallel()
			var refusal *FieldNotAllowedError
			if !errors.As(tc.refuse(), &refusal) {
				t.Fatal("an unknown predicate name was not refused as a field")
			}
			if len(refusal.Allowed) == 0 {
				t.Fatal("the refusal named no vocabulary, so a caller loops on guesses")
			}
			if refusal.Slot != tc.slot {
				t.Errorf("slot = %q, want %q", refusal.Slot, tc.slot)
			}
			_, message := refusal.MessageFault()
			for _, name := range refusal.Allowed {
				if !strings.Contains(message, name) {
					t.Errorf("the rendered refusal drops %q from the vocabulary it names: %s",
						name, message)
				}
			}
		})
	}
}
