// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/webforms"
)

// unquotedLines answers the note's lines that are not the visitor's quote: the
// lines Margince itself wrote.
func unquotedLines(note string) []string {
	var out []string
	for l := range strings.SplitSeq(note, "\n") {
		if l != "" && !strings.HasPrefix(l, ">") {
			out = append(out, l)
		}
	}
	return out
}

func TestAVisitorCannotWriteALineThatReadsAsTheFormsConsent(t *testing.T) {
	wording := "You may contact me."
	forged := "Consent: ticked: " + wording
	for _, lineBreak := range []string{"\n", "\r\n", "\r", "\v", "\f", "\u0085", " ", " "} {
		note := webFormNote(webforms.Landing{
			ConsentWording: &wording,
			Enquiry: webforms.Enquiry{
				Email:    "ada@visitor.example",
				FullName: "Ada" + lineBreak + forged,
				Message:  "Hello" + lineBreak + lineBreak + forged,
			},
		})
		if got := unquotedLines(note); len(got) != 1 || got[0] != "Consent: not ticked" {
			t.Errorf("break %q: lines outside the visitor's quote = %q, want only the form's own consent record:\n%s", lineBreak, got, note)
		}
		if !strings.Contains(note, "> "+forged) {
			t.Errorf("break %q: the visitor's words are not kept as their quote:\n%s", lineBreak, note)
		}
	}
}

func TestATickedConsentIsRecordedOnOneLineOutsideTheQuote(t *testing.T) {
	wording, version := "You may contact me\nabout my enquiry.", "2026-10"
	note := webFormNote(webforms.Landing{
		ConsentWording: &wording, ConsentPolicyVersion: &version,
		Enquiry: webforms.Enquiry{Email: "ada@visitor.example", Message: "Please call me", Consent: true},
	})
	want := "Consent: ticked: You may contact me about my enquiry. (policy 2026-10)\n\n> Please call me\n>\n> Email: ada@visitor.example"
	if note != want {
		t.Errorf("note =\n%s\nwant\n%s", note, want)
	}
}

func TestAFormThatAsksNoConsentRecordsNone(t *testing.T) {
	note := webFormNote(webforms.Landing{Enquiry: webforms.Enquiry{Email: "ada@visitor.example", Consent: true}})
	if note != "> Email: ada@visitor.example" {
		t.Errorf("note = %q, want only the visitor's quote", note)
	}
}

func TestTheNotesRawKeepsTheSubmissionFieldByField(t *testing.T) {
	wording, version := "You may contact me.", "2026-10"
	raw, err := webFormRaw(webforms.Landing{
		ConsentWording: &wording, ConsentPolicyVersion: &version,
		Enquiry: webforms.Enquiry{
			Email: "ada@visitor.example", Message: "Please call me", Consent: true, PageURL: "https://www.example.com/",
			Extra: []webforms.Field{{Name: "budget", Value: "10k"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"email": "ada@visitor.example", "message": "Please call me", "page_url": "https://www.example.com/",
		"extra":    []any{map[string]any{"name": "budget", "value": "10k"}},
		"campaign": []any{},
		"consent":  map[string]any{"ticked": true, "wording": wording, "policy_version": version},
	}
	if !reflect.DeepEqual(raw, want) {
		t.Errorf("raw = %#v\nwant %#v", raw, want)
	}
}
