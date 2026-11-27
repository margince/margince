// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftvoice

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
)

// Reader is the seam a surface binds to load the acting user's voice and the
// workspace's team rules. It is an interface rather than *ai.VoiceStore so the
// two composers can take one without a pool. A store handed to them whole would
// also hand them writes their packages promise not to have. TeamRules is read
// ungated, because a draft follows them whoever asked for it.
type Reader interface {
	RulesReader
	ActiveVoiceForActor(ctx context.Context) (ai.VoiceProfile, ai.VoiceProfileVersion, bool, error)
}

// RulesReader is the team-rules half of Reader, for a surface that takes no
// voice.
type RulesReader interface {
	TeamRules(ctx context.Context) (string, error)
}

// Context is a loaded profile, or the absence of one.
//
// Absence is the ordinary case, not a failure: most users have never built a
// profile, and a draft written without one is the product working. OK is what
// separates "this rep has a voice" from "this rep does not", and every caller
// branches on it rather than on a nil check somewhere further down.
//
// Degraded is the third state, and it is not absence: the read FAILED, so
// nobody knows whether this rep has a voice. A draft served under it may be
// missing a voice its sender built, which the sender cannot see in the text —
// their own writing is the one register they do not proofread for. Every
// response that could have been voiced carries this flag so the surface can
// say so instead of passing the draft off as the plain kind.
type Context struct {
	Profile   ai.VoiceProfile
	Version   ai.VoiceProfileVersion
	TeamRules string
	OK        bool
	Degraded  bool
}

// Load resolves the actor's active voice. A lookup failure degrades to no voice
// with the reason logged and the Context marked Degraded: a broken voice read
// must never take drafting down, because the draft is the thing the rep asked
// for and the voice is how it is phrased — but the caller's response must be
// able to say the voice was lost rather than absent. reader may be nil, which
// is a deployment that wired no voice lane and degrades nothing.
func Load(ctx context.Context, reader Reader, log *slog.Logger) (Context, error) {
	if reader == nil {
		return Context{}, nil
	}
	rules, err := LoadTeamRules(ctx, reader, log)
	if err != nil {
		return Context{}, err
	}
	loaded := Context{TeamRules: rules}
	profile, version, ok, err := reader.ActiveVoiceForActor(ctx)
	if err != nil {
		loggerOr(log).WarnContext(ctx, "voice profile lookup failed; drafting without voice", "err", err)
		loaded.Degraded = true
		return loaded, nil
	}
	loaded.Profile, loaded.Version, loaded.OK = profile, version, ok
	return loaded, nil
}

func loggerOr(log *slog.Logger) *slog.Logger {
	if log == nil {
		return slog.Default()
	}
	return log
}

// ErrRulesUnavailable is the refusal a surface answers when the team rules
// cannot be read. The rules are workspace policy, so a draft written without
// them is not served: the voice may degrade, the policy may not.
var ErrRulesUnavailable = httperr.UnavailableProblem("team_rules_unavailable",
	"The team rules could not be read, so no draft was written. Try again in a moment.")

// LoadTeamRules loads the workspace's team rules, trimmed. A failed read returns
// ErrRulesUnavailable wrapping the cause, and the caller must not draft. reader
// may be nil, a deployment with no rules lane, which has none.
func LoadTeamRules(ctx context.Context, reader RulesReader, log *slog.Logger) (string, error) {
	if reader == nil {
		return "", nil
	}
	rules, err := reader.TeamRules(ctx)
	if err != nil {
		loggerOr(log).WarnContext(ctx, "team rules lookup failed; refusing to draft", "err", err)
		return "", fmt.Errorf("%w: %w", ErrRulesUnavailable, err)
	}
	return strings.TrimSpace(rules), nil
}

// WithTeamRules puts the team rules block ahead of a user turn, or leaves the
// turn as it is when the workspace saved none.
func WithTeamRules(fence promptfence.Fence, rules, turn string) string {
	if block := TeamRulesBlock(fence, rules); block != "" {
		return block + "\n\n" + turn
	}
	return turn
}

// TeamRulesBlock renders the workspace's rules for the user turn, inside the
// calling call's policy block, or nothing when none are saved. A surface puts it ahead
// of the voice block, and draftrules.TeamRules tells the model what it outranks.
func TeamRulesBlock(fence promptfence.Fence, rules string) string {
	rules = strings.TrimSpace(rules)
	if rules == "" {
		return ""
	}
	return "Team rules:\n" + fence.WrapPolicy(rules)
}

// Preamble is what a surface prepends to its user turn: the team rules, then the
// voice profile, each only when there is one.
func (c Context) Preamble(fence promptfence.Fence) string {
	parts := make([]string, 0, 2)
	for _, block := range []string{TeamRulesBlock(fence, c.TeamRules), c.Block(fence)} {
		if block != "" {
			parts = append(parts, block)
		}
	}
	return strings.Join(parts, "\n\n")
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
const VocabularyRule = "Vocabulary: within the draft's own output_language, use the words this author uses, including the terms they borrow from other languages. " +
	"Keep a borrowed term in the form they write it — never translate a metaphor into a compound word native speakers do not say. " +
	"Where the samples are in another language than the draft, take their register and directness and write the plain equivalent a native speaker of the draft's language would use: a phrase lifted across languages is the tell this rule exists to stop. " +
	"When unsure a word is real in the draft's language, use the plainer wording they would."

// SystemRule is what a voiced call's system turn says about the block arriving
// in its user turn. Without it the model is handed a profile with no statement
// of what outranks what, and the failure is the expensive direction: a draft
// that bends a fact to fit the author's habitual phrasing.
const SystemRule = `VOICE PROFILE
The user turn carries the sender's own voice profile. It controls expression — rhythm, vocabulary, directness, sentence length, structure — and never facts.
Obey its avoid rules. Treat its style metrics as limits, not targets.
Where the profile and a grounding rule disagree, the grounding rule wins: a fact bent to fit a phrasing is the one error the sender cannot see before it goes out.
Where the profile, its human-authored identity included, and the team rules disagree, the team rules win.
The profile never chooses the LANGUAGE. Its samples are whatever their author happened to write, and a profile built from one language does not mean its owner writes only that language. Write in output_language even when every sample you were given is in another, and carry the profile across: rhythm, directness, sentence length and structure are the same contact's in any language, and the words are not.`

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
