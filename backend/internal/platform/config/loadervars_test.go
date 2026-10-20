// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

// Each loader variable is refused as a setting, and each loader flag shows its
// doc in the usage text, both built from LoaderVars.
func TestEveryLoaderVarIsReservedAndDocumented(t *testing.T) {
	empty, err := config.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var usage bytes.Buffer
	config.PrintUsage(&usage, empty)
	vars := config.LoaderVars()
	if len(vars) == 0 {
		t.Fatal("the loader reads no variable of its own; this test checks nothing")
	}
	for _, v := range vars {
		s := config.Define(config.Spec[string]{Key: "x", Env: v.Env, Type: config.String, Doc: "d"})
		if _, err := config.NewRegistry([]config.Any{s}); err == nil {
			t.Errorf("%s is read by the loader and was accepted as a setting", v.Env)
		}
		if v.Flag != "" && !strings.Contains(usage.String(), v.Doc) {
			t.Errorf("--%s shows no doc in the usage text:\n%s", v.Flag, usage.String())
		}
	}
}
