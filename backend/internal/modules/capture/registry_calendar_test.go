// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"testing"
)

func TestCalendarWriteGrantMatchesFrontend(t *testing.T) {
	source, err := os.ReadFile("../../../../frontend/src/screens/connector-status.ts")
	if err != nil {
		t.Fatal(err)
	}
	table := regexp.MustCompile(`(?s)const CALENDAR_WRITE_SCOPES[^=]*= (\{.*?\});`).FindSubmatch(source)
	if len(table) != 2 {
		t.Fatal("calendar grant table missing")
	}
	// TypeScript allows trailing commas; JSON is the parsed table's common subset.
	literal := regexp.MustCompile(`,\s*([}\]])`).ReplaceAll(table[1], []byte("$1"))
	literal = regexp.MustCompile(`(?m)^(\s*)([A-Za-z][A-Za-z0-9_]*):`).ReplaceAll(literal, []byte(`${1}"${2}":`))
	var grants map[string][]string
	if err := json.Unmarshal(literal, &grants); err != nil {
		t.Fatal(err)
	}
	if len(calendarWriteScopes) == 0 || len(grants) != len(calendarWriteScopes) {
		t.Fatalf("frontend providers %v do not match backend %v", grants, calendarWriteScopes)
	}
	for provider, scopes := range calendarWriteScopes {
		if len(scopes) == 0 || !slices.Equal(scopes, grants[provider]) {
			t.Errorf("%s: backend scopes %v, frontend scopes %v", provider, scopes, grants[provider])
		}
		if calendarWriteGranted(provider, nil) {
			t.Errorf("%s accepts no scope", provider)
		}
		for _, scope := range scopes {
			if !calendarWriteGranted(provider, []string{scope}) {
				t.Errorf("%s refuses declared scope %s", provider, scope)
			}
		}
	}
	if calendarWriteGranted("unknown", []string{"Calendars.ReadWrite"}) {
		t.Error("unknown provider accepted")
	}
}
