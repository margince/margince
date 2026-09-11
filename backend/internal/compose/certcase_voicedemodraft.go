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
// The expectation is eval_draft's, deliberately: a stylometric floor, measured
// by the same stylometricProximity the evaluation itself spends. A content
// token would have been the wrong instrument — this call answers a FIXED
// hypothetical task about unspecified work, and the prompt forbids inventing
// particulars, so a phrase from the corpus is one the correct answer has no
// reason to reach for. Asking for it would reward parroting an exemplar and
// fail the drafts that did the job.

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
	// A correct draft differs from an incorrect one in how close it sits to the
	// corpus, so the expectation IS that number rather than a wrapper carrying
	// it — the same shape eval_draft takes.
	var floor float64
	if err := json.Unmarshal(expected, &floor); err != nil {
		return nil, fmt.Errorf(
			"%s: the expected answer is not a stylometric floor in [0,1]: %w", voiceDemoDraftSite, err)
	}
	if err := refuseUndemonstrableVoice(f, floor); err != nil {
		return nil, err
	}
	return &voiceDemoDraftCase{
		personality: f.Personality,
		artifact: ai.VoiceArtifact{
			Markdown:  f.VoiceProfileMD,
			Stats:     f.Stats,
			Exemplars: f.Exemplars,
		},
		floor: floor,
	}, nil
}

// refuseUndemonstrableVoice names a scenario that would measure nothing, at
// parse time rather than after a paid run.
func refuseUndemonstrableVoice(f voiceDemoDraftFixture, floor float64) error {
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
	case floor <= 0:
		return fmt.Errorf(
			"%s: the scenario expects a floor of %g, which every draft clears — including one with nothing in "+
				"common with the corpus — so it asserts nothing", voiceDemoDraftSite, floor)
	case floor > 1:
		return fmt.Errorf(
			"%s: the scenario expects a floor of %g, and stylometric proximity is at most 1",
			voiceDemoDraftSite, floor)
	}
	return nil
}

// voiceDemoDraftCase is one built voice ready to write a line of its own.
type voiceDemoDraftCase struct {
	personality string
	artifact    ai.VoiceArtifact
	floor       float64
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

// Evaluate runs the production reader, then measures the draft against the
// corpus fingerprint with the evaluation's own instrument. The order is the
// meaning: a draft the reader refuses has no fingerprint to disagree with, and
// production shows a blank card for it — the state the whole step exists to
// avoid.
func (c *voiceDemoDraftCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	reply, err := readVoiceEvalDraft(trace.Output)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	proximity := stylometricProximity(c.artifact.Stats, reply.body)
	result := aitasks.OutcomeAccepted
	detail := fmt.Sprintf("the draft sits at %.4f of the corpus fingerprint", proximity)
	if proximity < c.floor {
		result = aitasks.OutcomeWrongAnswer
		detail += fmt.Sprintf(", and the scenario expects at least %.4f", c.floor)
	}
	if len(reply.tells) > 0 {
		detail += "; " + voiceEvalTellNote(reply.tells)
	}
	return aitasks.Outcome{Result: result, Detail: detail}
}
