// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFieldEnumReadsValuesAndTheSentenceFromThePropertyOrItsEnumSchema(t *testing.T) {
	var all schemas
	const doc = `
Basis:
  type: string
  enum: [all, software]
  description: Which part of the value counts.
Body:
  type: object
  properties:
    own:
      type: string
      enum: [a, b]
      description: |
        The property's own sentence.

        A second paragraph the served schema does not pay for.
    named:
      $ref: '#/Basis'
    listed:
      type: array
      items: {type: string, enum: [x]}
    free: {type: string}
`
	if err := yaml.Unmarshal([]byte(strings.ReplaceAll(doc, "#/Basis", "#/components/schemas/Basis")), &all); err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]struct{ values, note string }{
		"own":    {`"a" "b"`, "The property's own sentence."},
		"named":  {`"all" "software"`, "Which part of the value counts."},
		"listed": {`"x"`, ""},
	} {
		values, _, note, err := fieldEnum(all["Body"].Properties[field], all)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(values, " "); got != want.values || note != want.note {
			t.Errorf("%s: got %s %q, want %s %q", field, got, note, want.values, want.note)
		}
	}
	if values, _, _, _ := fieldEnum(all["Body"].Properties["free"], all); len(values) != 0 {
		t.Errorf("a free string reported values %v", values)
	}
}

func TestFirstParagraphKeepsWholeSentencesWithinTheBound(t *testing.T) {
	sentence := strings.Repeat("word ", 20) + "end. "
	got := firstParagraph(strings.Repeat(sentence, 6))
	if len(got) > maxNoteBytes || !strings.HasSuffix(got, "end.") {
		t.Errorf("got %d bytes ending %q", len(got), got[len(got)-10:])
	}
}

func TestFieldEnumKeepsAResponseOnlyValueOutOfTheRequestValues(t *testing.T) {
	var all schemas
	const doc = `
Body:
  type: object
  properties:
    scoped:
      type: string
      enum: [owner, team, managed]
      x-response-only-values: [managed]
`
	if err := yaml.Unmarshal([]byte(doc), &all); err != nil {
		t.Fatal(err)
	}
	values, hidden, _, err := fieldEnum(all["Body"].Properties["scoped"], all)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(values, " ") + "|" + strings.Join(hidden, " "); got != `"owner" "team"|"managed"` {
		t.Errorf("got %s", got)
	}
}
