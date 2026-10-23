// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// scheduledForEdit is the slice of a scheduled row an edit rebuilds the message from.
type scheduledForEdit struct {
	HTMLBody    *string `json:"html_body"`
	DraftRef    *string `json:"draft_ref"`
	Attachments []struct {
		ID string `json:"id"`
	} `json:"attachments"`
	AttachmentsUnavailable *int `json:"attachments_unavailable"`
}

func (row scheduledForEdit) attachmentIDs() []string {
	out := make([]string, 0, len(row.Attachments))
	for _, file := range row.Attachments {
		out = append(out, file.ID)
	}
	return out
}

// assertNoFileFields fails when a read the edit does not open from carries the
// resolved files, which a client would take for "this message has none".
func assertNoFileFields(t *testing.T, where string, raw json.RawMessage) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("%s is not an object: %v", where, err)
	}
	for _, name := range []string{"attachments", "attachments_unavailable"} {
		if _, present := fields[name]; present {
			t.Errorf("%s carries %s; only the detail read resolves files", where, name)
		}
	}
	if _, present := fields["html_body"]; !present {
		t.Errorf("%s dropped html_body, which every read carries", where)
	}
}

func (p *preflightEnv) uploadToAnchor(t *testing.T) string {
	t.Helper()
	status, raw := uploadAs(t, p.AppEnv, nil, "activity", p.activityID, []byte("%PDF the quote"))
	if status != http.StatusCreated {
		t.Fatalf("uploading a file → %d: %s", status, raw)
	}
	return attachmentIDOf(t, raw)
}

func (p *preflightEnv) withholdBytes(t *testing.T, file string) {
	t.Helper()
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE attachment SET bytes_withheld = true WHERE id = $1`, uuidOf(t, file))
		return err
	}); err != nil {
		t.Fatalf("withholding the file's bytes: %v", err)
	}
}

func TestAScheduledMessageReadsBackWhatAnEditRebuildsItFrom(t *testing.T) {
	p := setupPreflight(t, compose.WithBlobstore(blobstore.NewMemory()))
	p.connect(t, gmailReadonlyScope, gmailSendScope)

	files := []string{p.uploadToAnchor(t), p.uploadToAnchor(t), p.uploadToAnchor(t), p.uploadToAnchor(t)}
	const markup = "<p>Written <b>the night before</b>.</p>"
	var created json.RawMessage
	if status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", AnyMap{
		"subject": "Monday morning", "body": "Written the night before.", "html_body": markup,
		"attachment_ids": files, "draft_ref": "draft-ref-1",
		"to": []string{"buyer@preflight.test"}, "consent_purpose": "transactional",
		"scheduled_at": clocktest.Now(t).Add(2 * time.Hour).UTC().Format(time.RFC3339),
		"scheduled_tz": "Europe/Berlin",
	}, nil, &created); status != http.StatusCreated {
		t.Fatalf("scheduling a send with files → %d, want 201", status)
	}
	assertNoFileFields(t, "the scheduling answer", created)
	var scheduled struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created, &scheduled); err != nil {
		t.Fatalf("reading the scheduled id: %v", err)
	}

	id := uuidOf(t, scheduled.ID)
	p.withdrawConsent(t)
	p.makeDue(t, id)
	p.fire(t, id)
	if status := p.Call(t, "DELETE", "/v1/attachments/"+files[1], nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("archiving a file → %d, want 204", status)
	}
	p.withholdBytes(t, files[2])

	var row scheduledForEdit
	if status := p.Call(t, "GET", "/v1/scheduled-sends/"+scheduled.ID, nil, nil, &row); status != http.StatusOK {
		t.Fatalf("reading the held message → %d, want 200", status)
	}
	if got := p.scheduledStatusThroughAPI(t, id); got != activities.ScheduledStatusHeld {
		t.Fatalf("the message reads %q, want held", got)
	}
	if row.HTMLBody == nil || *row.HTMLBody != markup {
		t.Errorf("html_body = %v, want the markup sent", row.HTMLBody)
	}
	if row.DraftRef == nil || *row.DraftRef != "draft-ref-1" {
		t.Errorf("draft_ref = %v, want draft-ref-1", row.DraftRef)
	}
	if want := []string{files[0], files[3]}; !slices.Equal(row.attachmentIDs(), want) {
		t.Errorf("attachments = %v, want %v in the order they were attached", row.attachmentIDs(), want)
	}
	if row.AttachmentsUnavailable == nil || *row.AttachmentsUnavailable != 2 {
		t.Errorf("attachments_unavailable = %v, want 2 (one archived, one withheld)", row.AttachmentsUnavailable)
	}

	var list []json.RawMessage
	if status := p.Call(t, "GET", "/v1/scheduled-sends", nil, nil, &list); status != http.StatusOK || len(list) != 1 {
		t.Fatalf("listing scheduled sends → %d with %d rows, want 200 with the one", status, len(list))
	}
	assertNoFileFields(t, "the list", list[0])
}
