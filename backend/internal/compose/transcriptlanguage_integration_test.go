// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
)

func TestTranscriptReadingRetainsItsWrittenExplanationLanguage(t *testing.T) {
	for _, tc := range []struct{ language, text string }{{"en", "states no next steps"}, {"de", "keine nächsten Schritte"}, {"vi", "không nêu rõ"}} {
		t.Run(tc.language, func(t *testing.T) {
			env := setupTranscript(t)
			integration.SetBaseLanguage(env.Admin(), t, env.Pool, tc.language)
			read := env.read(t, cannedBrain{reply: `{"proposals":[]}`})
			integration.SetBaseLanguage(env.Admin(), t, env.Pool, "en")
			finished, err := env.Activities.GetTranscriptRead(env.ctx, env.activity, read.ID)
			if err != nil {
				t.Fatal(err)
			}
			if finished.Status != activities.TranscriptReadDone || len(finished.ProposalIDs) != 0 || finished.StatusDetail == nil || !strings.Contains(*finished.StatusDetail, tc.text) {
				t.Fatalf("reading lost its language or empty outcome: %+v", finished)
			}
		})
	}
}
