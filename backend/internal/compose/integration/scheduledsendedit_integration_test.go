// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// scheduledForEdit is the slice of a scheduled row an edit rebuilds the message from.
type scheduledForEdit struct {
	ID          string  `json:"id"`
	HTMLBody    *string `json:"html_body"`
	DraftRef    *string `json:"draft_ref"`
	Attachments []struct {
		ID       string `json:"id"`
		Filename string `json:"filename"`
		ByteSize int64  `json:"byte_size"`
	} `json:"attachments"`
	AttachmentsUnavailable *int `json:"attachments_unavailable"`
}

// readForEdit reads one row through both doors an edit opens it from, and fails
// when the detail and the list disagree about it.
func (p *preflightEnv) readForEdit(t *testing.T, id string) scheduledForEdit {
	t.Helper()
	var detail scheduledForEdit
	if status := p.Call(t, "GET", "/v1/scheduled-sends/"+id, nil, nil, &detail); status != http.StatusOK {
		t.Fatalf("reading the scheduled send → %d, want 200", status)
	}
	var list []scheduledForEdit
	if status := p.Call(t, "GET", "/v1/scheduled-sends", nil, nil, &list); status != http.StatusOK {
		t.Fatalf("listing scheduled sends → %d, want 200", status)
	}
	for _, row := range list {
		if row.ID == id {
			if len(row.Attachments) != len(detail.Attachments) || (row.AttachmentsUnavailable == nil) != (detail.AttachmentsUnavailable == nil) {
				t.Fatalf("the list and the detail disagree about the files: list %+v, detail %+v", row, detail)
			}
			return detail
		}
	}
	t.Fatalf("the list does not hold scheduled send %s", id)
	return detail
}

func TestAScheduledMessageReadsBackWhatAnEditRebuildsItFrom(t *testing.T) {
	p := setupPreflight(t, compose.WithBlobstore(blobstore.NewMemory()))
	p.connect(t, gmailReadonlyScope, gmailSendScope)

	status, raw := uploadAs(t, p.AppEnv, nil, "activity", p.activityID, []byte("%PDF the quote"))
	if status != http.StatusCreated {
		t.Fatalf("uploading the file → %d: %s", status, raw)
	}
	file := attachmentIDOf(t, raw)
	const markup = "<p>Written <b>the night before</b>.</p>"
	var scheduled struct {
		ID       string  `json:"id"`
		HTMLBody *string `json:"html_body"`
	}
	if status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", AnyMap{
		"subject": "Monday morning", "body": "Written the night before.", "html_body": markup,
		"attachment_ids": []string{file}, "draft_ref": "draft-ref-1",
		"to": []string{"buyer@preflight.test"}, "consent_purpose": "transactional",
		"scheduled_at": clocktest.Now(t).Add(2 * time.Hour).UTC().Format(time.RFC3339),
		"scheduled_tz": "Europe/Berlin",
	}, nil, &scheduled); status != http.StatusCreated {
		t.Fatalf("scheduling a send with a file → %d, want 201", status)
	}
	if scheduled.HTMLBody == nil || *scheduled.HTMLBody != markup {
		t.Errorf("the scheduling answer carries markup %v, want the markup sent", scheduled.HTMLBody)
	}

	row := p.readForEdit(t, scheduled.ID)
	if row.HTMLBody == nil || *row.HTMLBody != markup {
		t.Errorf("html_body = %v, want the markup sent", row.HTMLBody)
	}
	if row.DraftRef == nil || *row.DraftRef != "draft-ref-1" {
		t.Errorf("draft_ref = %v, want draft-ref-1", row.DraftRef)
	}
	if len(row.Attachments) != 1 || row.Attachments[0].ID != file || row.Attachments[0].Filename != "scan.pdf" ||
		row.Attachments[0].ByteSize != int64(len("%PDF the quote")) {
		t.Errorf("attachments = %+v, want the one file attached", row.Attachments)
	}
	if row.AttachmentsUnavailable != nil {
		t.Errorf("attachments_unavailable = %d with every file attachable, want absent", *row.AttachmentsUnavailable)
	}

	if status := p.Call(t, "DELETE", "/v1/attachments/"+file, nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("archiving the file → %d, want 204", status)
	}

	row = p.readForEdit(t, scheduled.ID)
	if len(row.Attachments) != 0 {
		t.Errorf("an archived file is still offered to the edit: %+v", row.Attachments)
	}
	if row.AttachmentsUnavailable == nil || *row.AttachmentsUnavailable != 1 {
		t.Errorf("attachments_unavailable = %v after archiving the file, want 1", row.AttachmentsUnavailable)
	}
	if row.HTMLBody == nil || *row.HTMLBody != markup {
		t.Errorf("html_body = %v after the file went, want the markup sent", row.HTMLBody)
	}
}
