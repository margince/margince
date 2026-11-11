// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extprivacy

import "testing"

func TestEmptyValueBlanksWhatItCanAndRefusesTheRest(t *testing.T) {
	cases := []struct {
		dataType string
		nullable bool
		want     string
		refused  bool
	}{
		{"text", true, "NULL", false},
		{"integer", true, "NULL", false},
		{"text", false, "''", false},
		{"character varying", false, "''", false},
		{"bytea", false, `'\x'::bytea`, false},
		{"jsonb", false, "'{}'::jsonb", false},
		{"integer", false, "", true},
		{"timestamp with time zone", false, "", true},
	}
	for _, c := range cases {
		got, err := emptyValue(c.dataType, c.nullable)
		if c.refused != (err != nil) || got != c.want {
			t.Errorf("emptyValue(%q, nullable=%v) = %q, %v; want %q, refused=%v", c.dataType, c.nullable, got, err, c.want, c.refused)
		}
	}
}
