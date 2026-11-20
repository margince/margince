// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestSubjectInRendersABoundTaskInTheReadersLanguage(t *testing.T) {
	subject := Literal("automation.new_lead")
	message := ForWrite(context.Background(), &Message{Subject: Sentence("automation.new_lead")}, subject, "")
	if got := message.SubjectIn(subject, textlang.German); got != "Beim neuen Lead nachfassen" {
		t.Fatalf("a German reader got %q", got)
	}
	if got := message.SubjectIn("Renamed by hand", textlang.German); got != "Renamed by hand" {
		t.Fatalf("a subject edited after the write was replaced with %q", got)
	}
	var none *Message
	if got := none.SubjectIn("Call Alice", textlang.German); got != "Call Alice" {
		t.Fatalf("a subject with no descriptor changed to %q", got)
	}
}
