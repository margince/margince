// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config_test

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/platform/config"
)

func TestScalarTypesParseTheTextEverySourceShares(t *testing.T) {
	if v, err := config.Int8.Parse("-12"); err != nil || v != -12 {
		t.Fatalf("Int8 -12: %v %v", v, err)
	}
	if _, err := config.Int8.Parse("300"); err == nil {
		t.Fatal("Int8 must refuse 300: out of range")
	}
	if _, err := config.Uint16.Parse("-1"); err == nil {
		t.Fatal("Uint16 must refuse a negative")
	}
	if v, err := config.Duration.Parse("90s"); err != nil || v != 90*time.Second {
		t.Fatalf("Duration 90s: %v %v", v, err)
	}
	if _, err := config.Bool.Parse("yes"); err == nil {
		t.Fatal("Bool must refuse yes rather than read it as false")
	}
	if _, err := config.NonEmpty.Parse(""); err == nil {
		t.Fatal("NonEmpty must refuse the empty string")
	}
	if v, err := config.Float32.Parse("0.25"); err != nil || v != 0.25 {
		t.Fatalf("Float32 0.25: %v %v", v, err)
	}
}

func TestDurationIsTextInYAMLToo(t *testing.T) {
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(`90s`), &n); err != nil {
		t.Fatal(err)
	}
	v, err := config.Duration.DecodeYAML(n.Content[0])
	if err != nil || v != 90*time.Second {
		t.Fatalf("got %v %v", v, err)
	}
}

func TestNumberTypesCompareAndTextTypesDoNot(t *testing.T) {
	if less, ok := config.Int.Less(1, 2); !ok || !less {
		t.Fatal("Int must compare")
	}
	if _, ok := config.String.Less("a", "b"); ok {
		t.Fatal("String must not offer Min/Max")
	}
}
