// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for voice_build/demo_draft.
//
// The demonstration draft is the line of writing the profile card SHOWS: a
// built voice nobody can read a sentence of is a voice nobody can judge, which
// is what the step exists for. It is its own prompt and its own reader, sent on
// every build beside the two evaluation passes — and it had no case, so what
// came back was graded by nothing. The prompt census found it.
//
// It certifies the shipped path: the request is voiceDemoDraftRequest and the
// reply is read by readVoiceEvalDraft, the same reader production trusts.
//
// What the expectation MEANS here is narrower than eval_draft's floor beside
// it. That site measures how CLOSE a draft sits to the corpus, which is a
// number. This one asks whether the card can show anything at all: a reply the
// production reader refuses leaves the card blank, and a draft that writes in
// nobody's particular voice would read the same for every member.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const voiceDemoDraftSite = "voice_build/demo_draft"

// voiceDemoDraftFixture is one built voice as the demonstration reads it. It
// carries no held-out sample and no repeat index, unlike eval_draft's: this
// call answers a fixed task in the built voice rather than replying to one
// withheld message, so those fields would be inputs the site never reads.
type voiceDemoDraftFixture struct {
	Personality    string             `json:"personality"`
	VoiceProfileMD string             `json:"voice_profile_md"`
	Exemplars      []ai.VoiceExemplar `json:"exemplars"`
	Stats          ai.VoiceStats      `json:"stats"`
}

// voiceDemoDraftExpectation is what a readable demonstration must contain: a
// phrase this corpus produced and a generic draft would not.
type voiceDemoDraftExpectation struct {
	NamesToken string `json:"names_token"`
}

// voiceDemoDraftCases serves the draft the profile card shows.
type voiceDemoDraftCases struct{}

func (voiceDemoDraftCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskVoiceBuild,
		Variant: "demo_draft",
		Kind:    ai.SiteKindOneShot,
	}
}

// Prepare turns one built voice and the phrase a draft in it must carry into a
// runnable case.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (voiceDemoDraftCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f voiceDemoDraftFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("%s: the fixture is not the shape this site takes: %w", voiceDemoDraftSite, err)
	}
	var want voiceDemoDraftExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("%s: the expected answer is not this site's shape: %w", voiceDemoDraftSite, err)
	}
	if err := refuseUndemonstrableVoice(f, want); err != nil {
		return nil, err
	}
	return &voiceDemoDraftCase{
		personality: f.Personality,
		artifact: ai.VoiceArtifact{
			Markdown:  f.VoiceProfileMD,
			Stats:     f.Stats,
			Exemplars: f.Exemplars,
		},
		mustName: want.NamesToken,
	}, nil
}

// refuseUndemonstrableVoice names a scenario that would measure nothing, at
// parse time rather than after a paid run.
func refuseUndemonstrableVoice(f voiceDemoDraftFixture, want voiceDemoDraftExpectation) error {
	switch {
	case strings.TrimSpace(f.VoiceProfileMD) == "":
		return fmt.Errorf("%s: the fixture carries no built profile, so there is no voice to demonstrate",
			voiceDemoDraftSite)
	case len(f.Exemplars) == 0:
		return fmt.Errorf("%s: the fixture supplies no verbatim example, so the draft has no voice to copy",
			voiceDemoDraftSite)
	case f.Stats.WordCount < ai.StarterVoiceWords:
		return fmt.Errorf("%s: the fixture's corpus is %d own-authored words, and a build needs at least %d",
			voiceDemoDraftSite, f.Stats.WordCount, ai.StarterVoiceWords)
	case strings.TrimSpace(want.NamesToken) == "":
		return fmt.Errorf(
			"%s: the expectation names no phrase of this voice, so a draft in anybody's voice would satisfy it",
			voiceDemoDraftSite)
	}
	// And the phrase has to be one this VOICE produced. A token the profile and
	// its examples never contain could only be invented, so the scenario would
	// fail every correct draft.
	corpus := f.VoiceProfileMD
	for _, exemplar := range f.Exemplars {
		corpus += " " + exemplar.Text
	}
	if !strings.Contains(corpus, want.NamesToken) {
		return fmt.Errorf(
			"%s: the expectation's phrase %q appears in neither the profile nor its examples, so only an invented "+
				"draft could carry it", voiceDemoDraftSite, want.NamesToken)
	}
	return nil
}

// voiceDemoDraftCase is one built voice ready to write a line of its own.
type voiceDemoDraftCase struct {
	personality string
	artifact    ai.VoiceArtifact
	mustName    string
}

// Run issues the one request this site sends, bare — and so does the build: the
// demonstration drafts over the plain completer seam, so one draft is one call
// there and here alike.
func (c *voiceDemoDraftCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := voiceDemoDraftRequest(c.personality, c.artifact)
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("%s: %w", voiceDemoDraftSite, err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate runs the production reader and asks whether the card could show
// what came back, in a voice a reader would recognise as this member's.
func (c *voiceDemoDraftCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	reply, err := readVoiceEvalDraft(trace.Output)
	if err != nil {
		// Production shows a blank card for exactly this, which is the state
		// the whole step exists to avoid.
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	if !strings.Contains(reply.subject+" "+reply.body, c.mustName) {
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: fmt.Sprintf("never wrote %q, so the draft would read the same in anybody's voice", c.mustName),
		}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}
