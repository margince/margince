// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const germanNote = "Wir haben über die Verlängerung gesprochen. Sie möchten das Angebot bis Freitag sehen, " +
	"und der Einkauf ist nicht mehr gegen den Preis."

func TestALoggedNoteIsLabelledByTheFunctionCaptureUses(t *testing.T) {
	body, subject := germanNote, "Gespräch"
	col, holder, args := languageInsertFragment(LogActivityInput{Kind: "note", Body: &body, Subject: &subject}, []any{"a", "b"})
	if col != ", language" || holder != ", $3" || len(args) != 3 {
		t.Fatalf("fragment = %q %q %v, want the language column bound third", col, holder, args)
	}
	if args[2] != string(textlang.OfMessage(body, subject)) || args[2] != "de" {
		t.Fatalf("the note is labelled %v, want capture's own reading, de", args[2])
	}
}

func TestOnlyProseAMemberWritesIsLabelledAtLogTime(t *testing.T) {
	body := germanNote
	for _, kind := range []string{"note", "meeting", "call"} {
		if col, _, _ := languageInsertFragment(LogActivityInput{Kind: kind, Body: &body}, nil); col == "" {
			t.Errorf("a hand-logged %s is not labelled", kind)
		}
	}
	for _, kind := range []string{"email", "task"} {
		if col, _, args := languageInsertFragment(LogActivityInput{Kind: kind, Body: &body}, nil); col != "" || len(args) != 0 {
			t.Errorf("a logged %s is labelled at log time; it keeps the label its own path gave it", kind)
		}
	}
	short := "ok"
	if col, _, _ := languageInsertFragment(LogActivityInput{Kind: "note", Body: &short}, nil); col != "" {
		t.Error("a note whose words name no language is labelled anyway")
	}
}
