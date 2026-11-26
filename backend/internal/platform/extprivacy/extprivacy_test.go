// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extprivacy

import (
	"testing"

	"github.com/margince/margince/backend/pkg/extension/privacy"
	"github.com/margince/margince/backend/pkg/extension/reach"
)

// CountDependents asks sweepreach which columns a merge moves by casting the
// merged kind, so the two closed sets must spell every mergeable kind alike.
func TestEveryMergeableKindIsSpelledAlikeInReach(t *testing.T) {
	for kind, want := range map[privacy.RecordKind]reach.Record{
		privacy.Contact: reach.Contact, privacy.Lead: reach.Lead, privacy.Company: reach.Company,
	} {
		if reach.Record(kind) != want {
			t.Errorf("privacy kind %q casts to reach %q, want %q", kind, reach.Record(kind), want)
		}
	}
}

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
