// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestDestinationCopiesCannotRelabelEditedGeneratedProse(t *testing.T) {
	original := ForWrite(context.Background(), &Message{Subject: Prose(Generated, textlang.English)}, "Generated instruction", "")
	destination := ForWrite(context.Background(), ForDestination(original), "Human replacement", "")
	if destination != nil && len(destination.GeneratedParts("Human replacement", "")) != 0 {
		t.Fatal("a destination copy authenticated replacement prose")
	}
	if got := original.GeneratedParts("Generated instruction", ""); len(got) != 1 {
		t.Fatal("copying retired the original provenance")
	}
}

func TestOnlyBoundEffectsCarryProvenanceAcrossAnApproval(t *testing.T) {
	unbound := &Message{Version: 1, Subject: Prose(Generated, textlang.English)}
	if BoundEffect(unbound) != nil {
		t.Fatal("unstaged prose gained effect provenance")
	}
	bound := ForWrite(context.Background(), unbound, "Generated instruction", "")
	if effect := BoundEffect(bound); effect == nil || !effect.Subject.Matches("Generated instruction") {
		t.Fatal("the staged effect lost its original binding")
	}
}

func TestDisplayMetadataFailureKeepsTheDomainWords(t *testing.T) {
	original := strings.Repeat("immutable fact ", 80)
	metadata := &Message{Subject: Sentence("lead.reply", Text(original))}
	subject := "Review qualification for " + original
	fitted, title, body := FitForWrite(context.Background(), metadata, subject, "Evidence", 200, 200)
	if fitted != nil || title != subject || body != "Evidence" {
		t.Fatal("display failure changed the domain words")
	}
	if ForWrite(context.Background(), metadata, subject, "Evidence") != nil {
		t.Fatal("invalid display metadata was stored")
	}
}

func TestContextProvenanceCannotAuthenticateUnboundGeneratedWords(t *testing.T) {
	message := &Message{Subject: Prose(Generated, textlang.English), Body: Sentence("automation.new_lead")}
	ctx := WithMessage(context.Background(), message)
	carried := MessageIn(ctx)
	bound := ForWrite(ctx, carried, "Human replacement", Literal("automation.new_lead"))
	if bound == nil || bound.Subject != nil || bound.Body == nil {
		t.Fatalf("context authenticated unbound prose or lost a verified template: %+v", bound)
	}
	if message.Subject == nil {
		t.Fatal("context filtering mutated its caller")
	}
}
