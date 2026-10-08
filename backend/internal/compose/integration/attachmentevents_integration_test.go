// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Every attachment mutation states itself on the bus, and states no more than it
// should.
//
// Read back OUT of event_outbox rather than asserted against the call: the row
// is what a subscriber receives, and a payload built correctly and written
// wrongly looks identical from inside the store.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// attachmentEvents reads every attachment.* envelope staged for one parent,
// oldest first.
func attachmentEvents(t *testing.T, e *Env, parent string) []map[string]any {
	t.Helper()
	rows, err := e.Pool.Query(context.Background(), `
		SELECT envelope FROM event_outbox
		 WHERE envelope->>'type' LIKE 'attachment.%'
		   AND envelope->'payload'->>'parent_id' = $1
		 ORDER BY seq`, parent)
	if err != nil {
		t.Fatalf("reading the staged attachment events: %v", err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("scanning an envelope: %v", err)
		}
		var env map[string]any
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("decoding an envelope: %v", err)
		}
		out = append(out, env)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the staged attachment events: %v", err)
	}
	return out
}

func TestEveryAttachmentMutationStatesItselfOnTheBus(t *testing.T) {
	e := Setup(t)
	h := activities.NewHandlers(e.DB()).WithUploadLimit(uploadCeiling).WithBlobstore(blobstore.NewMemory())
	ctx := e.Admin()
	contact := e.SeedContact(t, "Bus Attach", &e.Rep1)

	body, ctype := multipartAttachment(t, "contact", contact.String(), "redundancy letter Weber.pdf", []byte("PDF-BYTES"))
	req := httptest.NewRequest(http.MethodPost, "/v1/attachments", body).WithContext(ctx)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	h.UploadAttachment(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: status %d, body %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	events := attachmentEvents(t, e, contact.String())
	if len(events) != 1 {
		t.Fatalf("an upload staged %d attachment event(s), want 1", len(events))
	}
	if got := events[0]["type"]; got != "attachment.created" {
		t.Errorf("the upload staged %v, want attachment.created", got)
	}

	// The envelope answers who and which file, so the payload does not repeat
	// them: entity is the attachment, actor is the uploader.
	entity, _ := events[0]["entity"].(map[string]any)
	if entity["type"] != "attachment" || entity["id"] != created.ID {
		t.Errorf("the envelope names entity %v, want the attachment %s", entity, created.ID)
	}
	if actor, _ := events[0]["actor"].(map[string]any); actor["id"] == "" || actor["id"] == nil {
		t.Errorf("the envelope names no actor: %v", events[0]["actor"])
	}

	payload, _ := events[0]["payload"].(map[string]any)
	if payload["parent_type"] != "contact" || payload["parent_id"] != contact.String() {
		t.Errorf("the payload names parent %v/%v, want contact/%s",
			payload["parent_type"], payload["parent_id"], contact)
	}
	if payload["byte_size"] != float64(len("PDF-BYTES")) {
		t.Errorf("the payload states byte_size %v, want %d", payload["byte_size"], len("PDF-BYTES"))
	}

	// the point of the payload's shape. A filename is text somebody typed and
	// can be the sensitive part on its own, so it does not travel to whoever
	// subscribed, while the audit row inside the workspace keeps it.
	if _, leaked := payload["filename"]; leaked {
		t.Errorf("the event carries a filename: %v", payload)
	}
	for key, value := range payload {
		if text, isText := value.(string); isText && text == "redundancy letter Weber.pdf" {
			t.Errorf("the event carries the typed filename under %q", key)
		}
	}
	if _, leaked := payload["content"]; leaked {
		t.Errorf("the event carries file content: %v", payload)
	}

	// A metadata edit states itself too, and says which file changed rather
	// than what it now says: a title is typed text as a filename is.
	id := ids.MustParse(created.ID)
	title := "Signed MSA"
	if _, err := e.Activities.UpdateAttachmentMetadata(ctx, id, activities.DocumentMetadata{
		Title: &title,
	}); err != nil {
		t.Fatalf("editing the metadata: %v", err)
	}
	events = attachmentEvents(t, e, contact.String())
	if len(events) != 2 || events[1]["type"] != "attachment.updated" {
		t.Fatalf("after a metadata edit the bus holds %d event(s), ending %v — want 2 ending attachment.updated",
			len(events), lastEventType(events))
	}
	updated, _ := events[1]["payload"].(map[string]any)
	for key, value := range updated {
		if text, isText := value.(string); isText && text == title {
			t.Errorf("the update event carries the new title under %q — a title is as sensitive as a filename", key)
		}
	}
	if updated["parent_id"] != contact.String() {
		t.Errorf("the update event names parent %v, want %s", updated["parent_id"], contact)
	}

	// And archiving, which is this subsystem's delete.
	if err := e.Activities.ArchiveAttachment(ctx, id); err != nil {
		t.Fatalf("archiving: %v", err)
	}
	events = attachmentEvents(t, e, contact.String())
	if len(events) != 3 || events[2]["type"] != "attachment.archived" {
		t.Fatalf("after an archive the bus holds %d event(s), ending %v — want 3 ending attachment.archived",
			len(events), lastEventType(events))
	}

	// Archiving an archived file is refused before any write. The double-emit
	// the row lock in ArchiveAttachment guards against is reachable only when
	// two archives overlap, which this sequential path cannot observe.
	if err := e.Activities.ArchiveAttachment(ctx, id); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("archiving an archived file answered %v, want ErrNotFound", err)
	}
	archived, _ := events[2]["payload"].(map[string]any)
	if archived["parent_id"] != contact.String() || archived["parent_type"] != "contact" {
		t.Errorf("the archive event names parent %v/%v, want contact/%s",
			archived["parent_type"], archived["parent_id"], contact)
	}
}

// A captured file is covered by the mail's own event, so it adds none. Held
// here because the absence is a decision: a later reader who "fixed" the
// missing emit would make one delivery look like two to every subscriber
// counting them.
//
// Driven through the captured-file writer the mailbox pull itself uses, not a
// stand-in, a test that supplies its own version of that path would prove
// nothing about it.
func TestACapturedFileEmitsNoAttachmentEventOfItsOwn(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	contact := e.SeedContact(t, "Captured Attach", &e.Rep1)

	subject := "Re: MSA"
	logged, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: StrPtr("inbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("logging the mail: %v", err)
	}
	activityID := ids.From[ids.ActivityKind](ids.UUID(logged.Id))

	staged, err := e.Activities.StageCapturedFiles(ctx, []activities.CapturedFile{{
		PartID: "1", Filename: "MSA-redline.docx", Body: []byte("redline"),
		ContentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	}})
	if err != nil {
		t.Fatalf("staging the captured part: %v", err)
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return e.Activities.RecordCapturedFiles(ctx, tx, activityID,
			activities.CapturedFileSource{
				System: "imap", MessageID: "msa-" + activityID.String(),
				CapturedBy: "connector:imap", Category: "email_attachment",
			}, staged)
	}); err != nil {
		t.Fatalf("recording the captured file: %v", err)
	}

	if events := attachmentEvents(t, e, activityID.String()); len(events) != 0 {
		t.Errorf("a captured file staged %d attachment event(s) of its own; the mail's own event covers it: %v",
			len(events), events)
	}
}

// lastEventType reports the final envelope's type for a failure message, and
// answers nothing when there are none. Fatalf evaluates its arguments before it
// runs, so indexing the slice inline would panic on the regression this message
// exists to describe.
func lastEventType(events []map[string]any) string {
	if len(events) == 0 {
		return "no events at all"
	}
	kind, named := events[len(events)-1]["type"].(string)
	if !named {
		return "an envelope carrying no type"
	}
	return kind
}

// The refusals on the way in, which are the other half of each mutation: a file
// that is not there is not archived, not re-described, and announces nothing.
// Asserted because an event emitted on a refused mutation would be worse than
// one missing, a subscriber would act on a change that never happened.
func TestAMutationOnAFileThatIsNotThereAnnouncesNothing(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	contact := e.SeedContact(t, "Absent File", &e.Rep1)
	missing := ids.NewV7()

	if err := e.Activities.ArchiveAttachment(ctx, missing); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("archiving a file that is not there answered %v, want ErrNotFound", err)
	}
	title := "Renamed"
	if _, err := e.Activities.UpdateAttachmentMetadata(ctx, missing, activities.DocumentMetadata{
		Title: &title,
	}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("re-describing a file that is not there answered %v, want ErrNotFound", err)
	}
	if events := attachmentEvents(t, e, contact.String()); len(events) != 0 {
		t.Errorf("a refused mutation staged %d event(s): %v", len(events), events)
	}
}
