// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func TestARunStartedByHandNamesItsProviderAndAnyCategories(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		in    provider.QueueInput
		want  string
		field string
	}{
		{"a padded provider is trimmed", provider.QueueInput{Provider: " surfe "}, "surfe", ""},
		{"an omitted category list is the connection's own", provider.QueueInput{Provider: "surfe"}, "surfe", ""},
		{"no provider", provider.QueueInput{}, "", "provider"},
		{"spaces for a provider", provider.QueueInput{Provider: "  "}, "", "provider"},
		{"an empty category list", provider.QueueInput{Provider: "surfe", Categories: []provider.Category{}}, "", "categories"},
	} {
		got, err := namedPurchase(tc.in)
		if tc.field == "" {
			if err != nil || got != tc.want {
				t.Errorf("%s -> %q, %v, want %q", tc.name, got, err, tc.want)
			}
			continue
		}
		fault, ok := errors.AsType[apperrors.FieldFault](err)
		if !ok {
			t.Errorf("%s -> %v, want a field refusal", tc.name, err)
			continue
		}
		if field, code, message := fault.FieldFault(); field != tc.field || code != "required" || message == "" || message != err.Error() {
			t.Errorf("%s -> %q %q %q, want a required refusal naming %s with words", tc.name, field, code, message, tc.field)
		}
	}
}

func TestAnEmptyConfigurationRefusalNamesConfiguration(t *testing.T) {
	t.Parallel()
	refusal := &EmptyConfigurationError{}
	field, code, message := refusal.FieldFault()
	if field != "configuration" || code != "required" || message != refusal.Error() || message == "" {
		t.Errorf("FieldFault = %q %q %q, want a required refusal naming configuration", field, code, message)
	}
}

func TestAPatchThatNamesNothingOrAVersionBelowZeroIsUnusable(t *testing.T) {
	t.Parallel()
	mode := "on_demand"
	for _, tc := range []struct {
		name    string
		patch   ConfigPatch
		ifMatch int64
		want    error
	}{
		{"a patch naming no setting", ConfigPatch{}, 0, &EmptyConfigurationError{}},
		{"a version below zero", ConfigPatch{Mode: &mode}, -1, &NegativeVersionError{}},
		{"a patch naming a setting", ConfigPatch{Mode: &mode}, 3, nil},
	} {
		err := refuseUnusablePatch(tc.patch, tc.ifMatch)
		switch tc.want.(type) {
		case nil:
			if err != nil {
				t.Errorf("%s -> %v, want nil", tc.name, err)
			}
		case *EmptyConfigurationError:
			if _, ok := errors.AsType[*EmptyConfigurationError](err); !ok {
				t.Errorf("%s -> %v, want EmptyConfigurationError", tc.name, err)
			}
		case *NegativeVersionError:
			if _, ok := errors.AsType[*NegativeVersionError](err); !ok {
				t.Errorf("%s -> %v, want NegativeVersionError", tc.name, err)
			}
		}
	}
}
