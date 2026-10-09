// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Someone quoted inside the owner's own prose is not the owner's voice: the
// served ingest recognises a contact and a colleague by name and leaves their
// turns out, while a heading nobody is called and the owner's own name stay.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestQuotedSpeakersLeaveTheOwnersProseHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Sam Weber"}, nil, nil); status != http.StatusCreated {
		t.Fatalf("contact create → %d", status)
	}
	if err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`INSERT INTO app_user (id, email, display_name) VALUES ($1, 'lena@example.com', 'Lena Fischer')`, ids.NewV7())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	created := createVoiceProfile(t, e)

	own := []string{
		"Ada: a note to myself",
		"Frage: Was haelst du von der Idee?",
		"The rest is my own narration about the plan.",
	}
	quoted := []string{"Sam: we ship on Friday", "Lena: I will review it tonight"}
	content := strings.Join(append(append([]string{}, quoted...), own...), "\n\n")

	var ingested struct {
		voiceIngestResponse
		IngestStats struct {
			InputWords     int `json:"input_words"`
			KeptWords      int `json:"kept_words"`
			DiscardedTurns int `json:"discarded_turns"`
		} `json:"ingest_stats"`
	}
	if status := e.Call(t, "POST", "/v1/voice-profiles/"+created.ID+"/sources", AnyMap{
		"kind": "document", "source_label": "memo.txt", "source_ref": "memo-1",
		"format": "text", "content": content,
	}, nil, &ingested); status != http.StatusCreated {
		t.Fatalf("ingest → %d", status)
	}
	ownWords := len(strings.Fields(strings.Join(own, " ")))
	if ingested.Source.WordCount != ownWords {
		t.Fatalf("stored word_count = %d, want the owner's own %d — Sam and Lena are quoted, not the owner", ingested.Source.WordCount, ownWords)
	}
	stats := ingested.IngestStats
	if stats.DiscardedTurns != 2 || stats.KeptWords != ownWords || stats.InputWords != len(strings.Fields(content)) {
		t.Fatalf("ingest_stats = %+v, want two quoted turns left out of the whole source", stats)
	}
}
