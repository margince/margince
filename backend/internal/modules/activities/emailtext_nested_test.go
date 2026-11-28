// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"encoding/json"
	"os"
	"testing"
)

func TestNestedForwardsMatchTheBrowserCorpus(t *testing.T) {
	body, err := os.ReadFile("../../shared/kernel/mailquote/testdata/nested-forwards.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Name, Subject, Body, Main, Trimmed string }
	if err := json.Unmarshal(body, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 4 {
		t.Fatal("nested-forward corpus is incomplete")
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			parts := SplitEmailBody(tc.Body, tc.Subject)
			if parts.Main != tc.Main || parts.Trimmed != tc.Trimmed {
				t.Fatalf("main=%q tail=%q; want main=%q tail=%q", parts.Main, parts.Trimmed, tc.Main, tc.Trimmed)
			}
		})
	}
}
