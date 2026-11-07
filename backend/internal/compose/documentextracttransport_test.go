// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The 501 a role without a job runner answers, which is the one thing this
// transport does that nothing else can: a queued reading nobody will ever pick
// up is indistinguishable, to the rep watching it, from one still being worked,
// so the door has to refuse to queue it at all.

import (
	"github.com/riverqueue/river"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReadingADocumentIs501WithoutAJobRunner(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/attachments/x/extraction", nil)

	documentReadHandlers{}.ReadAttachmentForFields(rec, req, openapi_types.UUID(ids.NewV7()))

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501 (body %s)", rec.Code, rec.Body.String())
	}
	// Named, so an operator reads which capability is absent rather than that
	// something unspecified is missing.
	if !strings.Contains(rec.Body.String(), "readAttachmentForFields") {
		t.Errorf("body = %s, want it to name the operation", rec.Body.String())
	}
}

func TestReadingInsertionDoesNotWaitForAnAbandonedRiverRow(t *testing.T) {
	opts := documentExtractInsertOpts()
	if opts.Queue != transcriptReadQueue {
		t.Errorf("queue = %q, want %q", opts.Queue, transcriptReadQueue)
	}
	if !reflect.DeepEqual(opts.UniqueOpts, river.UniqueOpts{}) {
		t.Errorf("a dead River row can suppress replacement work: %+v", opts.UniqueOpts)
	}
}
