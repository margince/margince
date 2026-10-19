// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestStaleVoiceCompletionKeepsTheBuildLanguage(t *testing.T) {
	for _, tc := range []struct{ language, phrase string }{
		{"en", "the corpus changed"}, {"de", "Die Textsammlung hat sich"}, {"vi", "Tập văn bản đã thay đổi"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			quote := "A concrete sample from the original corpus."
			env, build := seedVoiceBuild(t, quote, 6)
			integration.SetBaseLanguage(env.e.Admin(), t, env.e.Pool, tc.language)
			changed := false
			scripted := &scriptedBuildBrain{judgeScore: .9, quote: quote}
			brain := brainFunc(func(ctx context.Context, req model.Request) (model.Response, error) {
				if !changed {
					changed = true
					_, _, _, err := env.store.IngestSource(env.owner, env.profile.ID, ai.IngestSourceInput{Kind: "email", Register: "email", SourceLabel: "Added during build", Content: strings.Repeat("A new concrete account of finished work. ", 60)})
					if err != nil {
						return model.Response{}, err
					}
					integration.SetBaseLanguage(env.e.Admin(), t, env.e.Pool, "en")
				}
				return scripted.Complete(ctx, req)
			})
			worker := newVoiceBuildWorker(env.e.Pool, brain, slog.New(slog.DiscardHandler))
			if err := worker.Work(t.Context(), voiceBuildJob(env, build)); err != nil {
				t.Fatal(err)
			}
			versions, err := env.store.ListVersions(env.owner, env.profile.ID, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(versions.Items) != 1 {
				t.Fatalf("versions: %+v", versions)
			}
			version := versions.Items[0]
			found := false
			for _, reason := range version.ReviewReasons {
				if strings.Contains(reason, tc.phrase) {
					found = true
				}
			}
			if !changed || !found || version.Status != "candidate" {
				t.Fatalf("stale outcome: %+v", version)
			}
		})
	}
}
