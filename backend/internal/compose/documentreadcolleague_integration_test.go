// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A document reading runs for the colleague who asked for it. The run row is
// kept by system:document_read, so it always reaches an end. The document is
// read by agent:document-extractor under that colleague's live grants. A
// colleague who may no longer see it gets a failed reading.

import (
	"context"
	"strings"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// countingDocumentBrain refuses every call and counts it, so a test can tell
// whether the document reached a model at all.
type countingDocumentBrain struct{ calls *int }

func (countingDocumentBrain) AttachmentMIMEs() []string { return nil }

func (b countingDocumentBrain) Complete(context.Context, model.Request) (model.Response, error) {
	*b.calls++
	return model.Response{}, ai.ErrAllTiersFailed
}

// readDocumentFor queues one reading of a text attachment on a deal Rep1 owns,
// asked for by Rep1, and works its last try.
func readDocumentFor(t *testing.T, env *integration.Env) (readID ids.UUID, calls int, after activities.ExtractionRead, workErr error) {
	t.Helper()
	pipeline, stage, _ := integration.DealFixture(t, env)
	deal := env.SeedDeal(t, "Document for a colleague", pipeline, stage, &env.Rep1)
	blob := blobstore.NewMemory()
	store := activities.NewStore(env.DB()).WithBlobstore(blob)
	attachment, err := store.UploadAttachment(env.Admin(), activities.AttachmentInput{
		EntityType: "deal", EntityID: deal, Filename: "price.txt", ContentType: "text/plain",
		Content: strings.NewReader("Price: 10 EUR."),
	})
	if err != nil {
		t.Fatal(err)
	}
	requester := "human:" + env.Rep1.String()
	reading, _, err := store.StartExtractionReadQueued(env.Admin(), ids.UUID(attachment.Id), requester, nil)
	if err != nil {
		t.Fatal(err)
	}
	worker := newDocumentExtractWorker(env.Pool, countingDocumentBrain{calls: &calls}, blob, discardLogger())
	job := &river.Job[DocumentExtractArgs]{
		JobRow: &rivertype.JobRow{Attempt: oneOffJobMaxAttempts, MaxAttempts: oneOffJobMaxAttempts},
		Args: DocumentExtractArgs{
			AttachmentID: ids.UUID(attachment.Id), ExtractionReadID: reading.ID, RequestedBy: requester,
		},
	}
	workErr = worker.Work(boundBackground(t), job)
	after, err = store.GetExtractionRead(env.Admin(), ids.UUID(attachment.Id), reading.ID)
	if err != nil {
		t.Fatal(err)
	}
	return reading.ID, calls, after, workErr
}

func TestADocumentIsReadByTheRequestersAgentAndClosedByTheSystem(t *testing.T) {
	env := integration.Setup(t)
	env.GrantRole(t, env.Rep1, "rep")

	readID, calls, after, workErr := readDocumentFor(t, env)
	if calls == 0 || workErr == nil {
		t.Fatalf("model calls = %d, job error = %v: the requester's agent did not open the document", calls, workErr)
	}
	if after.Status != activities.ExtractionReadFailed {
		t.Fatalf("reading = %q on its last refused try, want failed", after.Status)
	}
	if n := env.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'attachment_extraction' AND entity_id = $1
		AND actor_type = 'system' AND actor_id = 'system:document_read' AND on_behalf_of = $2`, readID, env.Rep1); n == 0 {
		t.Error("no change to the reading was kept by system:document_read for Rep1")
	}
}

func TestADocumentReadingForARequesterWithoutTheGrantFailsWithoutAModel(t *testing.T) {
	env := integration.Setup(t)

	readID, calls, after, workErr := readDocumentFor(t, env)
	if workErr != nil {
		t.Errorf("the refused reading faulted its job (%v); it is closed, so the job is done", workErr)
	}
	if calls != 0 {
		t.Errorf("the model was called %d times for a requester who may not read the document", calls)
	}
	if after.Status != activities.ExtractionReadFailed {
		t.Fatalf("reading = %q, want failed and never left running", after.Status)
	}
	if n := env.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'attachment_extraction' AND entity_id = $1
		AND actor_id = 'system:document_read'`, readID); n == 0 {
		t.Error("the failed reading was not closed by system:document_read")
	}
}
