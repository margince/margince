// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

func settingUnitSource(settings string) string {
	return `package x

import "github.com/margince/margince/backend/pkg/extension"

func New() extension.Extension {
	return extension.Extension{
		Name:        "x",
		Version:     "0.1.0",
		Description: "A unit composed by a test.",
		Settings: []extension.Setting{
` + settings + `
		},
	}
}
`
}

func TestSettingsDeriveIntoTheManifestSortedByKey(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", settingUnitSource(
		"\t\t\t{Key: \"region\", Label: \"Region\", Kind: extension.SettingText},\n"+
			"\t\t\t{Key: \"live\", Label: \"Live\", Hint: \"Send for real\", Kind: extension.SettingSwitch},\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(derived)
	live, region := strings.Index(s, `"key": "live"`), strings.Index(s, `"key": "region"`)
	if live < 0 || region < 0 || live > region {
		t.Fatalf("the manifest misses a setting or does not sort them by key:\n%s", s)
	}
	if !strings.Contains(s, `"kind": "switch"`) || !strings.Contains(s, `"hint": "Send for real"`) {
		t.Errorf("the manifest misses the kind or the hint:\n%s", s)
	}
}

// Generation refuses what the boot would, through the same published Validate.
func TestASettingTheBootWouldRefuseIsRefusedAtGeneration(t *testing.T) {
	for name, setting := range map[string]string{
		"a key with a capital": "\t\t\t{Key: \"Live\", Label: \"Live\", Kind: extension.SettingSwitch},\n",
		"no label":             "\t\t\t{Key: \"live\", Kind: extension.SettingSwitch},\n",
		"one key twice": "\t\t\t{Key: \"live\", Label: \"A\", Kind: extension.SettingSwitch},\n" +
			"\t\t\t{Key: \"live\", Label: \"B\", Kind: extension.SettingSwitch},\n",
		"an unknown field": "\t\t\t{Key: \"live\", Label: \"Live\", Kind: extension.SettingSwitch, Default: \"on\"},\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := deriveSynthetic(t, "x", settingUnitSource(setting)); err == nil {
				t.Fatal("generation accepted a setting the boot refuses")
			}
		})
	}
}
