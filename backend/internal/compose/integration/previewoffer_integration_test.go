// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"maps"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The preview asks the question the send asks. An offer attached by file name
// alone previews as a refusal, and the send then refuses it too.
func TestAnOfferInAFileNamePreviewsAsTheSendDecides(t *testing.T) {
	c := setupConsent(t)
	c.useReviewableMarketingProfile(t)
	anchor := c.inbound(t)
	file := c.seedFile(t, anchor, "Angebot_2026.pdf")
	message := AnyMap{
		"subject": "Re: Termin", "body": "Siehe Anhang.", "to": []string{"subject@consent.test"},
		"consent_purpose": "business_correspondence",
	}
	preview := func(files []string) bool {
		body := AnyMap{"attachment_ids": files}
		maps.Copy(body, message)
		var answer struct {
			Allowed bool `json:"allowed"`
		}
		if status := c.Call(t, "POST", "/v1/activities/"+anchor+"/send-email:preview", body, nil, &answer); status != http.StatusOK {
			t.Fatalf("previewing → %d, want 200", status)
		}
		return answer.Allowed
	}
	if !preview(nil) {
		t.Fatal("the reply without the file previews as refused: the fixture proves nothing")
	}
	if preview([]string{file}) {
		t.Fatal("the reply with an offer attached previews as allowed")
	}
	message["attachment_ids"] = []string{file}
	if status := c.Call(t, "POST", "/v1/activities/"+anchor+"/send-email", message, nil, nil); status != http.StatusConflict {
		t.Fatalf("sending the reply with the offer attached → %d, want 409 like its preview", status)
	}
}

// seedFile files an attachment on an activity, as an upload leaves one.
func (c *consentEnv) seedFile(t *testing.T, activity, filename string) string {
	t.Helper()
	id := ids.NewV7()
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO attachment (id, entity_type, entity_id, filename, content_type, byte_size,
			                        storage_key, checksum, source, captured_by)
			VALUES ($1, 'activity', $2, $3, 'application/pdf', 2048, $4, 'sum', 'upload', 'human:test')`,
			id, activity, filename, "preview-files/"+id.String())
		return err
	}); err != nil {
		t.Fatalf("seeding the file: %v", err)
	}
	return id.String()
}
