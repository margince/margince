// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capturelabel

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
)

// The stamp is a property of the code, not of the process that computed it:
// one that moved per process would re-offer every declined message after every
// restart.
func TestTheRulesetStampIsStableAcrossProcesses(t *testing.T) {
	first, second := ai.PromptDigest(rulesetPrompt), ai.PromptDigest(rulesetPrompt)
	if first != second {
		t.Fatalf("the stamp moved between two computations in one process: %q then %q", first, second)
	}
	if first != Ruleset {
		t.Errorf("Ruleset = %q but recomputing the same builder gives %q", Ruleset, first)
	}
}

// Half of what the site asks is how a message is laid out, so the stamp must
// move when the user turn does and not only when the system prompt does.
func TestTheRulesetMovesWithTheUserTurn(t *testing.T) {
	for i := range rulesetSamples {
		changed := slices.Clone(rulesetSamples)
		changed[i].Inbound = false
		moved := ai.PromptDigest(func(fence promptfence.Fence) string {
			return SystemFor(fence) + "\n" + Prompt(fence, changed)
		})
		if moved == Ruleset {
			t.Errorf("rendering sample %d differently left the stamp where it was, so the user turn is not digested", i)
		}
	}
}

// A chat message carries no subject line. Its channel is our own record, so
// that line sits outside the fenced span, where the sender cannot write it.
func TestAChatMessageNamesItsChannelOutsideTheFenceAndHasNoSubject(t *testing.T) {
	fence := promptfence.New()
	chat := activities.UnlabeledMessage{
		ID: ids.NewV7(), Kind: activities.KindMessage, ChannelProvider: "openchannel",
		Body: "Are you free at 8:00 PM Monday?", Inbound: true,
	}
	mail := activities.UnlabeledMessage{ID: ids.NewV7(), Kind: "email", Subject: "Re: the offer", Body: "Yes."}
	prompt := Prompt(fence, []activities.UnlabeledMessage{chat, mail})

	chatHead, chatSpan := spanOf(t, fence, prompt, "m1")
	if !strings.HasSuffix(chatHead, "channel: openchannel\ninbound: yes\n") {
		t.Errorf("the chat's channel and direction lines do not sit right above its span; got %q", chatHead)
	}
	if strings.Contains(chatSpan, "Subject:") || strings.Contains(chatSpan, "channel:") {
		t.Errorf("the chat's fenced text carries a subject or channel line: %q", chatSpan)
	}
	mailHead, mailSpan := spanOf(t, fence, prompt, "m2")
	if mailHead != "inbound: no\n" || !strings.Contains(mailSpan, "Subject: Re: the offer\n") {
		t.Errorf("the mail lost its subject or gained a channel line: head %q span %q", mailHead, mailSpan)
	}
	if !strings.Contains(SystemFor(fence), "chat message") {
		t.Error("the system prompt does not tell the model that some messages are chat")
	}
}

// spanOf returns the text between the previous span's close and the span for
// ref, and the span's own text.
func spanOf(t *testing.T, fence promptfence.Fence, prompt, ref string) (head, span string) {
	t.Helper()
	opening := fence.WrapAttr("source_id", ref, "")
	open, _, ok := strings.Cut(opening, ">")
	if !ok {
		t.Fatalf("the fence opens no span: %q", opening)
	}
	before, after, ok := strings.Cut(prompt, open+">")
	if !ok {
		t.Fatalf("the prompt has no span for %s", ref)
	}
	if i := strings.LastIndex(before, fence.Close()); i >= 0 {
		before = strings.TrimPrefix(before[i+len(fence.Close()):], "\n")
	}
	span, _, _ = strings.Cut(after, fence.Close())
	return before, span
}
