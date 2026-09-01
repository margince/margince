// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftvoice

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
)

// Reader is the seam a surface binds to load the acting user's voice. It is an
// interface rather than *ai.VoiceStore so the two composers can take one
// without taking a pool: what they need is a read, and a store handed to them
// whole would also hand them the writes their packages promise not to have.
type Reader interface {
	ActiveVoiceForActor(ctx context.Context) (ai.VoiceProfile, ai.VoiceProfileVersion, bool, error)
}

// Context is a loaded profile, or the absence of one.
//
// Absence is the ordinary case, not a failure: most users have never built a
// profile, and a draft written without one is the product working. OK is what
// separates "this rep has a voice" from "this rep does not", and every caller
// branches on it rather than on a nil check somewhere further down.
type Context struct {
	Profile ai.VoiceProfile
	Version ai.VoiceProfileVersion
	OK      bool
}

// Load resolves the actor's active voice. A lookup failure degrades to no voice
// with the reason logged: a broken voice read must never take drafting down,
// because the draft is the thing the rep asked for and the voice is how it is
// phrased. reader may be nil, which is a deployment that wired no voice lane.
func Load(ctx context.Context, reader Reader, log *slog.Logger) Context {
	if reader == nil {
		return Context{}
	}
	profile, version, ok, err := reader.ActiveVoiceForActor(ctx)
	if err != nil {
		if log == nil {
			log = slog.Default()
		}
		log.WarnContext(ctx, "voice profile lookup failed; drafting without voice", "err", err)
		return Context{}
	}
	return Context{Profile: profile, Version: version, OK: ok}
}

// Block renders the profile for the CALLING call's user turn.
//
// It takes the caller's fence rather than making its own: the block is
// prepended to that call's user turn, so it must be bounded by the marker that
// call's system prompt declares. A block fenced with its own nonce is text the
// model was never told to treat as data.
//
// Everything rendered descends from corpus text — the artifact carries verbatim
// quotes and the exemplars ARE corpus excerpts — so each piece sits inside the
// boundary unedited. The exemplars are shown to be imitated verbatim, which is
// exactly why they may not be rewritten on the way in.
func (c Context) Block(fence promptfence.Fence) string {
	if !c.OK {
		return ""
	}
	var block strings.Builder
	block.WriteString("Voice profile:\n")
	if personality := strings.TrimSpace(c.Profile.PersonalityMD); personality != "" {
		block.WriteString("Human-authored identity (highest priority):\n" + fence.Wrap(personality) + "\n\n")
	}
	block.WriteString(fence.Wrap(strings.TrimSpace(c.Version.VoiceProfileMD)))
	for _, exemplar := range ai.VersionExemplars(c.Version) {
		fmt.Fprintf(&block, "\n\nVerbatim example (%s %s):\n%s",
			exemplar.Register, exemplar.Kind, fence.Wrap(exemplar.Text))
	}
	stats := ai.DecodeVersionStats(c.Version)
	fmt.Fprintf(&block, "\n\nStylometric guardrails — limits, NOT targets: mean sentence length ≈ %.0f words (do not write a wall of short sentences to hit it), em dashes per 100 words ≈ %.2f (at 0, treat them as forbidden).",
		stats.MeanSentenceWords, stats.EmDashPer100Words)
	block.WriteString("\n\n" + VocabularyRule)
	return block.String()
}

// VocabularyRule is the one instruction a stylometric profile cannot carry: the
// words themselves. A profile measures sentence length and punctuation, so
// without this a model writes the author's rhythm in its own vocabulary — and
// the tell a reader notices first is a translated metaphor no native speaker
// says.
const VocabularyRule = "Vocabulary: use the words this author uses, including the terms they borrow from other languages. " +
	"Keep a borrowed term in the form they write it — never translate a metaphor into a compound word native speakers do not say. " +
	"When unsure a word is real in their language, use the plainer wording they would."

// SystemRule is what a voiced call's system turn says about the block arriving
// in its user turn. Without it the model is handed a profile with no statement
// of what outranks what, and the failure is the expensive direction: a draft
// that bends a fact to fit the author's habitual phrasing.
const SystemRule = `VOICE PROFILE
The user turn carries the sender's own voice profile. It controls expression — rhythm, vocabulary, directness, sentence length, structure — and never facts.
Obey its avoid rules. Treat its style metrics as limits, not targets.
Where the profile and a grounding rule disagree, the grounding rule wins: a fact bent to fit a phrasing is the one error the sender cannot see before it goes out.`

// Violations runs the deterministic floor over the two texts a draft is made
// of, independently. Concatenation would hide a canned opener inside the body,
// because the opener rule anchors at the start of the text it is given.
func Violations(subject, body string) []ai.VoiceViolation {
	return append(ai.DetectAIPatterns(subject), ai.DetectAIPatterns(body)...)
}

// Feedback is the critic turn: what the last attempt broke, so the retry fixes
// the sentence rather than the punctuation.
func Feedback(violations []ai.VoiceViolation) string {
	var b strings.Builder
	b.WriteString("\n\nThe previous attempt violated these hard rules; rewrite without them:\n")
	for _, violation := range violations {
		b.WriteString("- " + violation.Detail + "\n")
	}
	return b.String()
}

// Sanitize removes what the floor can mechanically remove, after the retry has
// had its chance at the sentence.
func Sanitize(subject, body string) (string, string) {
	return ai.SanitizeAIPatterns(subject), ai.SanitizeAIPatterns(body)
}
