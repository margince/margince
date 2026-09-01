// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"encoding/json"
	"testing"
)

// What one reduction has to accept, and what it must not repair.
//
// A model asked for structured output usually gives it; when one does not, it
// WRAPS — a sentence before the fence, a sentence after it, an uppercase tag.
// Refusing those costs the reader the feature outright rather than degrading it.
//
// The line this file holds is between recovering a document the model buried
// and repairing one it got wrong. The second is where a reduction starts
// inventing answers, so the last cases are the ones that must still fail.
func TestUnfenceRecoversABuriedDocumentAndRepairsNothing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		text  string
		want  string
		valid bool
	}{
		{
			name:  "bare JSON is returned as it stands",
			text:  `{"a":1}`,
			want:  `{"a":1}`,
			valid: true,
		}, {
			name:  "the fence it has always stripped",
			text:  "```json\n{\"a\":1}\n```",
			want:  `{"a":1}`,
			valid: true,
		}, {
			// The shape that costs the feature: the model explains itself, then
			// answers. The old trim removed neither sentence and the document
			// never reached the parser.
			name:  "a sentence before the fence",
			text:  "Here is the profile you asked for:\n```json\n{\"a\":1}\n```",
			want:  `{"a":1}`,
			valid: true,
		}, {
			name:  "a sentence after the fence",
			text:  "```json\n{\"a\":1}\n```\nLet me know if you would like it adjusted.",
			want:  `{"a":1}`,
			valid: true,
		}, {
			name:  "an uppercase language tag",
			text:  "```JSON\n{\"a\":1}\n```",
			want:  `{"a":1}`,
			valid: true,
		}, {
			name:  "no fence at all, just prose around it",
			text:  "Sure. {\"a\":1} — that is the profile.",
			want:  `{"a":1}`,
			valid: true,
		}, {
			// TWO BLOCKS: an illustrative fragment, then the answer. Taking the
			// first hands back the example — which parses, and is not what the
			// model was asked for.
			name:  "an example before the answer is not the answer",
			text:  "For example ```json\n{\"a\":1}\n```\nHere is yours:\n```json\n{\"a\":1,\"b\":2,\"c\":3}\n```",
			want:  `{"a":1,"b":2,"c":3}`,
			valid: true,
		}, {
			// And the other way round: the answer, then a quoted key. Taking
			// the last hands back the footnote.
			name:  "a fragment quoted after the answer is not the answer",
			text:  "```json\n{\"a\":1,\"b\":2,\"c\":3}\n```\nThe key you asked about: ```json\n{\"b\":2}\n```",
			want:  `{"a":1,"b":2,"c":3}`,
			valid: true,
		}, {
			// REPAIRING, not recovering. A document with a bare word where a
			// value belongs is the model getting the answer wrong, and a
			// reduction that guessed at it would put words in its mouth.
			// The invalid cases name their WANT too. Without it a regression
			// that mangled the text could pass by leaving it invalid — and
			// invalid is the easy half to keep true.
			name:  "a bare word where a value belongs stays invalid",
			text:  `{"a": fk}`,
			want:  `{"a": fk}`,
			valid: false,
		}, {
			name:  "a truncated document stays invalid",
			text:  `{"a": {"b":`,
			want:  `{"a": {"b":`,
			valid: false,
		}, {
			// A BRACE PAIR IN THE PROSE, on both sides of the answer. Read as
			// the span from the first brace to the last, this is the whole
			// sentence — which is not a document, so the document that IS here
			// was refused.
			name:  "an aside in braces does not hide the answer",
			text:  `Here is the answer {as requested}: {"a":1} — let me know {if that helps}`,
			want:  `{"a":1}`,
			valid: true,
		}, {
			// And a brace INSIDE a value is not structure. Followed as one, the
			// document closes early and a prefix of it is offered.
			name:  "a brace inside a string is not a closing brace",
			text:  `Sure: {"pattern":"^{2}$","a":1}`,
			want:  `{"pattern":"^{2}$","a":1}`,
			valid: true,
		}, {
			name:  "prose with no document in it stays what it was",
			text:  "I cannot produce that profile.",
			want:  "I cannot produce that profile.",
			valid: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Unfence(tc.text)
			if json.Valid([]byte(got)) != tc.valid {
				t.Fatalf("Unfence(%q) = %q, which is %s JSON — want the other",
					tc.text, got, map[bool]string{true: "valid", false: "invalid"}[!tc.valid])
			}
			if tc.want != "" && got != tc.want {
				t.Errorf("Unfence(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}
