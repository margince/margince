// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config_test

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

func TestTheOverviewAndTraceNameASecretsSourceButNotItsValue(t *testing.T) {
	cfg, err := load(t, fixture{env: []string{"MARGINCE_DSN=postgres://secret", "MARGINCE_LOG_LEVEL=debug"}}, level, dsn)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, e := range cfg.Entries() {
		switch e.Key {
		case "dsn":
			seen++
			if e.Value != nil || e.Origin.Name != "MARGINCE_DSN" || !e.Secret {
				t.Fatalf("dsn entry %+v", e)
			}
		case "log.level":
			seen++
			if e.Value == nil || *e.Value != "debug" {
				t.Fatalf("log.level entry %+v", e)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("overview lists %d of the 2 settings", seen)
	}
	trace := strings.Join(cfg.TraceLines(), "\n")
	if strings.Contains(trace, "postgres://secret") || !strings.Contains(trace, "dsn = <secret> (env MARGINCE_DSN)") ||
		!strings.Contains(trace, "log.level = debug (env MARGINCE_LOG_LEVEL)") {
		t.Fatalf("trace:\n%s", trace)
	}
}

type overviewWindow struct {
	Hours int `json:"hours" yaml:"hours"`
}

// A compound value and default read as an operator writes them, through the
// type's own format, whichever source set the value.
func TestTheOverviewWritesCompoundValuesAsAnOperatorWould(t *testing.T) {
	weights := config.Define(config.Spec[map[string]int]{
		Key: "overview.weights", Env: "X_OVERVIEW_WEIGHTS", Type: config.Map(config.Int),
		Default: config.Some(map[string]int{"a": 1}), Doc: "d",
	})
	window := config.Define(config.Spec[overviewWindow]{
		Key: "overview.window", Env: "X_OVERVIEW_WINDOW", Type: config.Struct[overviewWindow](),
		Default: config.Some(overviewWindow{Hours: 2}), Doc: "d",
	})
	cfg, err := load(t, fixture{env: []string{"X_OVERVIEW_WEIGHTS=a:3,b:4"}}, weights, window)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]string{
		"overview.weights": {"a:3,b:4", "a:1"},
		"overview.window":  {`{"hours":2}`, `{"hours":2}`},
	}
	for _, e := range cfg.Entries() {
		w, ok := want[e.Key]
		if !ok {
			continue
		}
		delete(want, e.Key)
		if e.Value == nil || *e.Value != w[0] || e.Default == nil || *e.Default != w[1] {
			t.Errorf("%s: value %v default %v, want %q and %q", e.Key, deref(e.Value), deref(e.Default), w[0], w[1])
		}
	}
	if len(want) != 0 {
		t.Fatalf("the overview lists none of %v", want)
	}
}

// A setting this program does not load names its declaration only: no value
// and no source, since another program's sources are not visible here.
func TestAnUnreadSettingHasNoValueAndNoSource(t *testing.T) {
	e := config.Unread(level)
	if e.Value != nil || e.Origin.Source != config.SourceUnset || e.Default == nil || *e.Default != "info" || e.Env != "MARGINCE_LOG_LEVEL" {
		t.Fatalf("unread entry %+v", e)
	}
	if secret := config.Unread(dsn); secret.Value != nil || secret.Default != nil || !secret.Secret {
		t.Fatalf("unread secret %+v", secret)
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
