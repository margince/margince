// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A reading every model withheld is the content's answer, not a provider's bad
// minute: the same words asked again meet the same filter. Each reading site
// that already treats a refused reply as terminal treats a withheld one the
// same way, and keeps retrying what is not a verdict on the content.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// outcomeLane answers every request with one provider outcome.
type outcomeLane struct{ outcome error }

func (l outcomeLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{}, fmt.Errorf("ai: gemini: answer withheld: SAFETY: %w", l.outcome)
}

func (outcomeLane) AttachmentMIMEs() []string { return nil }

func TestAReadingEveryModelWithheldIsRefusedNotRetried(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	thread := settledThread{Key: "t-1", CompanyID: ids.NewV7()}
	criteria := []stageEvidenceCriterion{{Key: "budget", Label: "Budget confirmed"}}
	spans := []stageEvidenceSpan{{SourceID: "s-1", Lines: []string{"we have budget"}}}

	for name, tc := range map[string]struct {
		outcome  error
		terminal bool
	}{
		"withheld": {outcome: model.ErrOutputWithheld, terminal: true},
		// Our own request refused is a defect to surface, not a verdict on the
		// content, so it keeps the path's ordinary failure.
		"rejected": {outcome: model.ErrRequestRejected, terminal: false},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			lane := outcomeLane{outcome: tc.outcome}

			_, signalErr := (&SignalExtractor{brain: lane, log: quiet}).ask(ctx, thread)
			_, transcriptErr := (&TranscriptProposer{brain: lane, log: quiet}).ask(ctx, []string{"hello"}, "2026-09-24")
			_, documentErr := (&DocumentExtractor{brain: lane, log: quiet}).ask(ctx, documentSource{Text: "terms", Parent: ids.New[ids.CompanyKind]().Ref()})
			claims, stageErr := (&StageEvidenceReader{brain: lane, log: quiet}).ask(ctx, criteria, spans)

			for site, got := range map[string]struct {
				err     error
				refused error
			}{
				"signal extract":   {signalErr, errRefusedReading},
				"transcript":       {transcriptErr, errRefusedTranscript},
				"document extract": {documentErr, errRefusedDocument},
			} {
				if isTerminal := errors.Is(got.err, got.refused); isTerminal != tc.terminal {
					t.Errorf("%s: refused = %v, want %v (err: %v)", site, isTerminal, tc.terminal, got.err)
				}
				if !tc.terminal && !errors.Is(got.err, tc.outcome) {
					t.Errorf("%s: the outcome must survive for the job log, got %v", site, got.err)
				}
			}
			// The stage reader answers a refused reading with no claims and no error.
			if gotTerminal := stageErr == nil && claims == nil; gotTerminal != tc.terminal {
				t.Errorf("stage evidence: terminal = %v, want %v (err: %v)", gotTerminal, tc.terminal, stageErr)
			}
		})
	}
}
