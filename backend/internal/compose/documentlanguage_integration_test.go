// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestDocumentReadingStoresItsOriginalExplanationLanguage(t *testing.T) {
	for _, tc := range []struct{ language, text string }{{"en", "states none"}, {"de", "keines der Deal-Felder"}, {"vi", "không nêu rõ"}} {
		t.Run(tc.language, func(t *testing.T) {
			env := integration.Setup(t)
			ctx := env.Admin()
			integration.SetBaseLanguage(ctx, t, env.Pool, tc.language)
			pipeline, stage, _ := integration.DealFixture(t, env)
			deal := env.SeedDeal(t, "Example deal", pipeline, stage, &env.Rep1)
			store := activities.NewStore(env.DB()).WithBlobstore(blobstore.NewMemory())
			attachment, err := store.UploadAttachment(ctx, activities.AttachmentInput{EntityType: "deal", EntityID: deal, Filename: "note.txt", ContentType: "text/plain", Content: strings.NewReader("A note with no commercial terms.")})
			if err != nil {
				t.Fatal(err)
			}
			attachmentID := ids.UUID(attachment.Id)
			reading, _, err := store.StartExtractionReadQueued(ctx, attachmentID, "manual", nil)
			if err != nil {
				t.Fatal(err)
			}
			brain := scriptedBrain{FakeClient: ai.NewFakeClient().Script(allFour())}
			reader := NewDocumentExtractor(env.Pool, brain, discardLogger())
			if err := reader.Read(ctx, store, reading.ID, attachmentID); err != nil {
				t.Fatal(err)
			}
			integration.SetBaseLanguage(ctx, t, env.Pool, "en")
			finished, err := store.GetExtractionRead(ctx, attachmentID, reading.ID)
			if err != nil {
				t.Fatal(err)
			}
			if finished.Status != activities.ExtractionReadDone || finished.StatusDetail == nil || !strings.Contains(*finished.StatusDetail, tc.text) {
				t.Fatalf("reading lost its original language: %+v", finished)
			}
		})
	}
}
