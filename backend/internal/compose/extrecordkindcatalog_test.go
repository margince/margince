// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

func TestCheckReadColumnTypes(t *testing.T) {
	const tz, id = "timestamp with time zone", "uuid"
	kind := func(label string) recordkinds.Declared {
		return recordkinds.Declared{Name: "ext:u:k", NaturalKey: []string{"code"}, Timeline: &extension.RecordKindTimeline{
			LabelField: label, AtColumn: "seen_at", Subjects: []extension.TimelineSubject{{Entity: "contact", Column: "contact_id"}},
		}}
	}
	ok := map[string]string{"code": "text", "seen_at": tz, "contact_id": id}
	with := func(col, typ string) map[string]string {
		m := map[string]string{}
		for k, v := range ok {
			m[k] = v
		}
		if typ == "" {
			delete(m, col)
		} else {
			m[col] = typ
		}
		return m
	}
	cases := []struct {
		name string
		cols map[string]string
		want string
	}{
		{"complete", ok, ""},
		{"varchar title", with("code", "character varying(40)"), ""},
		{"missing time", with("seen_at", ""), "does not exist"},
		{"missing subject", with("contact_id", ""), "does not exist"},
		{"time is text", with("seen_at", "text"), "not timestamp"},
		{"subject is text", with("contact_id", "text"), "not uuid"},
		{"title is integer", with("code", "integer"), "title column"},
	}
	for _, c := range cases {
		err := checkReadColumnTypes(kind("code"), "ext.t", c.cols)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: %v", c.name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
}
