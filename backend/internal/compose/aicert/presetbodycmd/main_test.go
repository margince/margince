// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"errors"
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
		if code := run(tc.args, &out, &errs); code != tc.want || out.Len() != 0 || errs.Len() == 0 {
			t.Errorf("%s: exit %d, stdout %q, stderr %q; want exit %d, nothing printed and a reason given",
				tc.name, code, out.String(), errs.String(), tc.want)
		}
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestABodyThatCannotBeWrittenIsAFailure(t *testing.T) {
	var errs bytes.Buffer
	if code := run([]string{"../../../../../config/presets/openrouter_cloud_eu.yaml"}, brokenWriter{}, &errs); code != 1 ||
		!strings.Contains(errs.String(), "writing the body") {
		t.Errorf("exit %d, stderr %q; want exit 1 naming the write", code, errs.String())
	}
}

// A usage error whose report cannot be written still fails, as a plain failure.
func TestAReportThatCannotBeWrittenStillFails(t *testing.T) {
	var out bytes.Buffer
	if code := run(nil, &out, brokenWriter{}); code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
}
