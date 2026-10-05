// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestParseReopenWindowRefusesWhatIsNotAWindow(t *testing.T) {
	cases := map[string][2]string{
		"missing end":   {"2026-10-01T00:00:00Z", ""},
		"not a time":    {"yesterday", "2026-10-02T00:00:00Z"},
		"ends first":    {"2026-10-02T00:00:00Z", "2026-10-01T00:00:00Z"},
		"empty between": {"2026-10-01T00:00:00Z", "2026-10-01T00:00:00Z"},
	}
	for name, c := range cases {
		if _, err := parseReopenWindow(c[0], c[1]); err == nil {
			t.Errorf("%s: accepted %q..%q", name, c[0], c[1])
		}
	}
	w, err := parseReopenWindow("2026-10-01T00:00:00+02:00", "2026-10-02T00:00:00Z")
	if err != nil || !w.From.Before(w.To) {
		t.Fatalf("a valid window was refused: %v", err)
	}
}

func TestReopenParkedRefusesAStrayPositionalArgument(t *testing.T) {
	err := runReopenParked(context.Background(), nil,
		[]string{"--from", "2026-10-01T00:00:00Z", "--to", "2026-10-02T00:00:00Z", "yesterday"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), `"yesterday"`) {
		t.Fatalf("a stray argument was not refused by name: %v", err)
	}
}
