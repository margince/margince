// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The Art. 17 cascade holds a Handelsbrief under the statutory floor this
// binary compiles in (GoBDFloorPack). The floor keeps the record, never a
// voice sample copied from it, so the sample is emptied with the redacted mail.
// The sample is filed through the ingest door under the sent-mail key, which
// is the key the copy purge finds it by.

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
)

func TestErasingTheRecipientEmptiesTheVoiceSampleOfAHeldMail(t *testing.T) {
	e := Setup(t)
	f := seedRestrictionFixture(t, e)
	owner := e.As(e.Rep1, nil, voiceRepPerms)
	voice := ai.NewVoiceStore(e.DB())
	profile, err := voice.CreateProfile(owner, ai.CreateVoiceProfileInput{})
	if err != nil {
		t.Fatal(err)
	}
	ref := "sent-mail:" + f.email.String()
	if _, _, _, err := voice.IngestSource(owner, profile.ID, ai.IngestSourceInput{
		Kind: "email", SourceLabel: "Angebot 2026-0042", SourceRef: ref, Content: "Our offer, as discussed with you.",
	}); err != nil {
		t.Fatalf("filing the sample: %v", err)
	}

	if err := compose.ComposedEraser(e.Pool, nil, nil).EraseContact(e.Admin(), f.contact, "test"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND restricted_at IS NOT NULL AND body IS NOT NULL`, f.email); n != 1 {
		t.Fatal("the erasure did not hold the Handelsbrief, so this does not test the held set")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM voice_corpus_source WHERE source_ref = $1
		AND content IS NULL AND content_erased_at IS NOT NULL AND source_label NOT LIKE '%Angebot%'`, ref); n != 1 {
		t.Error("the held mail's voice sample keeps its text or subject after the erasure")
	}
}
