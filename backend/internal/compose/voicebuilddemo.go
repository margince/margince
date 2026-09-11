// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The DEMONSTRATION draft: the one line of its own writing a built voice shows
// on the profile card. A voice nobody can read a sentence of is a voice nobody
// can judge, which is what the step exists for.
//
// It shares the evaluation's drafting instruction and profile block — what
// differs is that this call carries no held-out sample to reply to, so it
// answers a fixed hypothetical task instead. It lives beside the evaluation
// passes rather than inside them because it is its own certified site, not a
// mode of theirs.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// voiceDemoDraftSystemFor is the same instruction for a call that carries no
// sample to reply to — only the profile is inside the boundary, so only the
// profile is named by it.
func voiceDemoDraftSystemFor(fence promptfence.Fence) string {
	return voiceEvalDraftSystem + "\n" + fence.Rule("profile")
}

// voiceDemoTask is the scenario a DEMONSTRATION draft answers, for a corpus
// with nothing to spare for held-out scoring.
//
// Plainly hypothetical, and it asks for no particulars: the surface labels
// what comes back a sample, and inventing a counterparty's actual words to
// reply to is the one thing every prompt on this path already refuses.
const voiceDemoTask = "Write a short reply to a colleague who asked how a piece of work is going and when they can expect it. " +
	"Invent no names, dates, figures or commitments — write only what could be said without them."

// demonstrationDraft asks for ONE draft in the built voice, for a corpus that
// could spare no sample to score against.
//
// IT IS NOT AN EVALUATION AND NEVER BECOMES ONE. Nothing was held out, so
// there is nothing this was scored against: it carries no score, never reaches
// the median, and leaves the version as unevaluated as it was. What it is for
// is the reader — a profile with not one line of its own writing on screen is
// a profile nobody can judge, and the voice step exists to be judged.
//
// voiceDemoDraftRequest builds the one request this site sends. Named and
// exported to the certification case for the reason every other site's builder
// is: a case that rebuilt it would measure a copy, and a copy stays green
// through the change that breaks the original. It was inline until the prompt
// census found this prompt reaching a provider with nothing grading the reply.
//
//promptlang:exempt a draft reproducing one member's writing voice from their own profile; their exemplars decide the language, and a rule of ours would answer in the wrong one
//promptvoice:exempt a draft reproducing one member's own writing voice; imposing our register is the one thing it must not do.
func voiceDemoDraftRequest(personality string, artifact ai.VoiceArtifact) model.Request {
	fence := promptfence.New()
	profileBlock := voiceDraftPromptBlock(personality, artifact.Markdown, artifact.Exemplars, artifact.Stats, fence)
	return model.Request{
		System:         voiceDemoDraftSystemFor(fence),
		Messages:       []model.Message{{Role: chatRoleUser, Content: profileBlock + "\n\n" + voiceDemoTask}},
		MaxTokens:      voiceDemoDraftMaxTokens,
		ResponseSchema: replyDraftSchema,
		SecretStripper: ai.NewSecretStripper(),
	}
}

// voiceDemoDraftMaxTokens bounds one demonstration draft: a subject and a short
// body, which is what the profile card shows.
const voiceDemoDraftMaxTokens = 1200

func demonstrationDraft(ctx context.Context, brain completer, artifact ai.VoiceArtifact, personality string) ([]map[string]any, error) {
	resp, err := ai.Ask(ctx, brain, voiceDemoDraftRequest(personality, artifact), func(text string) error {
		_, readErr := readVoiceEvalDraft(text)
		return readErr
	})
	if err != nil {
		return nil, fmt.Errorf("voice demonstration draft: %w", err)
	}
	// readVoiceEvalDraft already refuses an empty subject or body, so what
	// comes back here is a draft or an error, never a blank card.
	reply, err := readVoiceEvalDraft(resp.Text)
	if err != nil {
		return nil, fmt.Errorf("voice demonstration draft: %w", err)
	}
	return []map[string]any{{
		"prompt":               voiceDemoTask,
		voiceDraftFieldSubject: reply.subject,
		voiceDraftFieldBody:    reply.body,
		// Null, not zero: a number here would report a score this build did
		// not take, and zero is the score a failed draft gets.
		"voice_score": nil,
	}}, nil
}
