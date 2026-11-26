// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"errors"
	"strings"
	"testing"
)

func TestASettingDeclarationIsRefusedWhenItCannotBeShown(t *testing.T) {
	ok := Setting{Key: "region", Label: "Region", Kind: SettingText}
	if err := ok.Validate(); err != nil {
		t.Fatalf("a plain text setting: %v", err)
	}
	for name, s := range map[string]Setting{
		"key with a capital":         {Key: "Region", Label: "Region", Kind: SettingText},
		"no label":                   {Key: "region", Kind: SettingText},
		"multi-line label":           {Key: "region", Label: "a\nb", Kind: SettingText},
		"unknown kind":               {Key: "region", Label: "Region", Kind: "slider"},
		"label too long":             {Key: "region", Label: strings.Repeat("x", maxSettingLabel+1), Kind: SettingSwitch},
		"hint with a newline":        {Key: "region", Label: "Region", Hint: "a\nb", Kind: SettingSwitch},
		"switch starting on yes":     {Key: "live", Label: "Live", Kind: SettingSwitch, Initial: "yes"},
		"text starting on two lines": {Key: "region", Label: "Region", Kind: SettingText, Initial: "a\nb"},
	} {
		if err := s.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestASettingValueMustFitItsKind(t *testing.T) {
	on := Setting{Key: "live", Label: "Live", Kind: SettingSwitch}
	if on.Default() != "false" || on.CheckValue("true") != nil || on.CheckValue("yes") == nil {
		t.Errorf("a switch holds only true or false and starts false")
	}
	text := Setting{Key: "region", Label: "Region", Kind: SettingText}
	if text.Default() != "" || text.CheckValue("eu-west") != nil {
		t.Errorf("a text setting starts empty and takes a short line")
	}
	for _, v := range []string{"a\nb", strings.Repeat("x", maxSettingText+1)} {
		if err := text.CheckValue(v); !errors.Is(err, ErrInvalid) {
			t.Errorf("CheckValue(%q) = %v, want ErrInvalid", v, err)
		}
	}
}

func TestADeclaredInitialValueIsTheValueBeforeAnAdminChangesIt(t *testing.T) {
	on := Setting{Key: "live", Label: "Live", Kind: SettingSwitch, Initial: "true"}
	region := Setting{Key: "region", Label: "Region", Kind: SettingText, Initial: "eu-west"}
	for _, s := range []Setting{on, region} {
		if err := s.Validate(); err != nil {
			t.Fatalf("%s: %v", s.Key, err)
		}
	}
	if on.Default() != "true" || region.Default() != "eu-west" {
		t.Errorf("Default() = %q, %q, want the declared initial values", on.Default(), region.Default())
	}
}
