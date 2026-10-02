// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestTheCommandPrintsAPresetsBody(t *testing.T) {
	var out, errs bytes.Buffer
	if code := run([]string{"../../../../../config/presets/openrouter_cloud_eu.yaml"}, &out, &errs); code != 0 {
		t.Fatalf("exit %d: %s", code, errs.String())
	}
	if !strings.Contains(out.String(), `"mistralai/mistral-embed-2312"`) {
		t.Errorf("the body lost the preset's embeddings binding: %s", out.String())
	}
}

func TestTheCommandRefusesWhatIsNotAPreset(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want int
	}{
		{"no argument", nil, 2},
		{"a missing file", []string{"no-such-preset.yaml"}, 1},
		{"a file with no routing", []string{"main.go"}, 1},
	} {
		var out, errs bytes.Buffer
		if code := run(tc.args, &out, &errs); code != tc.want || out.Len() != 0 {
			t.Errorf("%s: exit %d with %q on stdout, want exit %d and nothing printed", tc.name, code, out.String(), tc.want)
		}
	}
}
