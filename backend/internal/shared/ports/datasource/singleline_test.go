// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

import "testing"

type nameBody struct {
	Name  string  `json:"name" singleline:"true"`
	Title *string `json:"title,omitempty" singleline:"true"`
	Notes *string `json:"notes,omitempty"`
}

func TestSingleLineFieldsRefuseControlsBreaksAndOverrides(t *testing.T) {
	for name, text := range map[string]string{
		"SOH":              "Deal \x01",
		"BEL":              "a\x07b",
		"ANSI escape":      "\x1b[31mred",
		"line feed":        "line1\nline2",
		"carriage return":  "a\rb",
		"tab":              "a\tb",
		"NUL":              "a\x00b",
		"DEL":              "a\x7fb",
		"C1 control":       "a\u0085b",
		"line separator":   "a\u2028b",
		"right-to-left":    "evil\u202etxt",
		"isolate override": "a\u2066b",
	} {
		refused := RejectUnsafeSingleLine(&nameBody{Name: text})
		if refused == nil || refused.Field != "name" {
			t.Errorf("%s: refusal = %v, want one naming name", name, refused)
		}
		bad := text
		if refused := RejectUnsafeSingleLine(&nameBody{Name: "ok", Title: &bad}); refused == nil || refused.Field != "title" {
			t.Errorf("%s in a pointer field: refusal = %v, want one naming title", name, refused)
		}
	}
}

func TestSingleLineFieldsKeepOrdinaryNamesAndMultilineFieldsKeepBreaks(t *testing.T) {
	notes := "first\nsecond\ttabbed"
	for _, name := range []string{"Müller & Söhne GmbH", "李雷", "Zoë 😀", "O'Reilly — Dept. 5", "a\u200db"} {
		if refused := RejectUnsafeSingleLine(&nameBody{Name: name, Notes: &notes}); refused != nil {
			t.Errorf("%q refused: %v", name, refused)
		}
	}
}

func TestMalformedTextIsFoundInTheRawBody(t *testing.T) {
	for body, want := range map[string]bool{
		`{"name":"fine"}`:                  false,
		`{"name":"😀"}`:                     false,
		`{"name":"\\ud800"}`:               false,
		`{"name":"é"}`:                     false,
		"{\"name\":\"bad \xff\xfe\xc3(\"}": true,
		`{"name":"\ud800"}`:                true,
		`{"name":"\ud800x"}`:               true,
		`{"name":"\udc00"}`:                true,
		`{"name":"\ud800A"}`:               true,
		`{"a":"😀","b":"\udead"}`:           true,
	} {
		if got := HasMalformedText([]byte(body)); got != want {
			t.Errorf("HasMalformedText(%q) = %v, want %v", body, got, want)
		}
	}
}
