// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

func addr() *config.Setting[string] {
	return config.Define(config.Spec[string]{
		Key: "addr", Env: "MARGINCE_ADDR", Flag: "addr",
		Type: config.NonEmpty, Default: config.Some(":8080"), Doc: "Listen address.",
	})
}

func TestRegistryRefusesWhatCannotBeDescribedHonestly(t *testing.T) {
	cases := map[string]config.Any{
		"wrong key form":  config.Define(config.Spec[int]{Key: "Log.Level", Env: "X_ENV", Type: config.Int, Doc: "d"}),
		"wrong env form":  config.Define(config.Spec[int]{Key: "x", Env: "margince_x", Type: config.Int, Doc: "d"}),
		"wrong flag form": config.Define(config.Spec[int]{Key: "x", Env: "X_ENV", Flag: "X_y", Type: config.Int, Doc: "d"}),
		"no doc":          config.Define(config.Spec[int]{Key: "x", Env: "X_ENV", Type: config.Int}),
		"no env":          config.Define(config.Spec[int]{Key: "x", Type: config.Int, Doc: "d"}),
		"no type":         config.Define(config.Spec[int]{Key: "x", Env: "X_ENV", Doc: "d"}),
		"secret default": config.Define(config.Spec[string]{
			Key: "x", Env: "X_ENV", Type: config.String,
			Secret: true, Default: config.Some("s"), Doc: "d",
		}),
		"min on text": config.Define(config.Spec[string]{
			Key: "x", Env: "X_ENV", Type: config.String,
			Min: config.Some("a"), Doc: "d",
		}),
		"default out of range": config.Define(config.Spec[int]{
			Key: "x", Env: "X_ENV", Type: config.Int,
			Default: config.Some(0), Min: config.Some(1), Doc: "d",
		}),
		"default fails validate": config.Define(config.Spec[int]{
			Key: "x", Env: "X_ENV", Type: config.Int,
			Default: config.Some(3), Validate: func(int) error { return errors.New("odd") }, Doc: "d",
		}),
		"stored secret in the clear": config.DefineStored(config.Spec[string]{
			Key: "x.key", Env: "X_ENV", Type: config.String, Secret: true, Doc: "d",
		}, config.Store{RBACObject: "installation_settings", AuditVerb: "update"}),
		"sealed but not secret": config.DefineStored(config.Spec[string]{
			Key: "x.key", Env: "X_ENV", Type: config.String, Doc: "d",
		}, config.Store{RBACObject: "installation_settings", AuditVerb: "update", SecretReference: true}),
		"sealed and read ungated": config.DefineStored(config.Spec[string]{
			Key: "x.key", Env: "X_ENV", Type: config.String, Secret: true, Doc: "d",
		}, config.Store{RBACObject: "installation_settings", AuditVerb: "update", SecretReference: true, MachineryApplied: true}),
	}
	for name, s := range cases {
		if _, err := config.NewRegistry([]config.Any{s}); err == nil {
			t.Errorf("%s: registry accepted it", name)
		}
	}
}

func TestRegistryRefusesTheSameNameTwiceInAnySource(t *testing.T) {
	for name, second := range map[string]config.Any{
		"key":  config.Define(config.Spec[string]{Key: "addr", Env: "OTHER_ENV", Type: config.String, Doc: "d"}),
		"env":  config.Define(config.Spec[string]{Key: "other", Env: "MARGINCE_ADDR", Type: config.String, Doc: "d"}),
		"flag": config.Define(config.Spec[string]{Key: "other", Env: "OTHER_ENV", Flag: "addr", Type: config.String, Doc: "d"}),
	} {
		_, err := config.NewRegistry([]config.Any{addr()}, []config.Any{second})
		if err == nil || !strings.Contains(err.Error(), "twice") {
			t.Errorf("duplicate %s: %v", name, err)
		}
	}
}

func TestReservedNamesAreNotSettings(t *testing.T) {
	s := config.Define(config.Spec[string]{Key: "env", Env: "MARGINCE_ENV", Type: config.String, Doc: "d"})
	if _, err := config.NewRegistry([]config.Any{s}); err == nil {
		t.Fatal("MARGINCE_ENV is read by the loader itself and is never a setting")
	}
}
