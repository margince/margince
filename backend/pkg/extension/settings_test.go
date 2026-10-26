// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension_test

import (
	"testing"
	"testing/fstest"

	"github.com/margince/margince/backend/pkg/extension"
)

func TestSettingsFileReadsTheDeclaredValues(t *testing.T) {
	fsys := fstest.MapFS{extension.SettingsFile: {Data: []byte(`{"vat_rate": 0.19, "labels": ["a"]}`)}}

	got, err := extension.SettingsFrom(fsys)
	if err != nil {
		t.Fatalf("a declared file was refused: %v", err)
	}
	if len(got) != 2 || string(got["vat_rate"]) != "0.19" {
		t.Fatalf("read %v, want both declared settings", got)
	}
}

func TestSettingsFileRefusesWhatCoreCannotStore(t *testing.T) {
	for name, body := range map[string]string{
		"a document that is not an object": `[1]`,
		"a key core cannot store":          `{"Vat-Rate": 1}`,
		"a key with a dot":                 `{"a.b": 1}`,
		"trailing content":                 `{"a": 1} {"b": 2}`,
	} {
		fsys := fstest.MapFS{extension.SettingsFile: {Data: []byte(body)}}
		if _, err := extension.SettingsFrom(fsys); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestSettingsFileMustExistWhenDeclared(t *testing.T) {
	if _, err := extension.SettingsFrom(fstest.MapFS{}); err == nil {
		t.Fatal("a declared settings layer with no file was accepted")
	}
}

func TestSettingKeyIsTheUnitsNamespaceAndTheKey(t *testing.T) {
	got, err := extension.Name("finance-demo").SettingKey("vat_rate")
	if err != nil || got != "ext_finance_demo.vat_rate" {
		t.Fatalf("got %q, %v", got, err)
	}
}
