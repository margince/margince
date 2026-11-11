// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// chatFixture is a screenshot uploaded to a contact and read for a chat by the
// real reader over a scripted model.
type chatFixture struct {
	env        *integration.Env
	store      *activities.Store
	blob       blobstore.Store
	contact    ids.UUID
	attachment ids.UUID
	reading    ids.UUID
}

func readChatScreenshot(t *testing.T, modelReply string) chatFixture {
	t.Helper()
	env := integration.Setup(t)
	ctx := env.Admin()
	blob := blobstore.NewMemory()
	store := activities.NewStore(env.DB()).WithBlobstore(blob)
	contact := env.SeedContact(t, "Lena Hoffmann", &env.AdminUser)
	att, err := store.UploadAttachment(ctx, activities.AttachmentInput{
		EntityType: "contact", EntityID: contact, Filename: "chat.png", ContentType: "image/png",
		Content: bytes.NewReader(cardPNG(t)),
	})
	if err != nil {
		t.Fatal(err)
	}
	attachment := ids.UUID(att.Id)
	reading, _, err := store.StartExtractionReadFor(ctx, attachment,
		activities.ExtractionQuestionChatCapture, "human:"+env.AdminUser.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	brain := scriptedBrain{FakeClient: ai.NewFakeClient().Script(modelReply), carriage: carriesImagesOnly}
	if err := NewDocumentExtractor(env.Pool, brain, discardLogger()).Read(ctx, store, reading.ID, attachment); err != nil {
		t.Fatal(err)
	}
	return chatFixture{env: env, store: store, blob: blob, contact: contact, attachment: attachment, reading: reading.ID}
}

func (f chatFixture) file(t *testing.T, body *string) (ids.UUID, error) {
	t.Helper()
	out, err := NewChatCaptureFile(f.env.Pool).File(f.env.Admin(), f.attachment, crmcontracts.FileChatCaptureRequest{
		ExtractionId: openapi_types.UUID(f.reading), ChannelProvider: "whatsapp", Body: body,
	})
	return ids.UUID(out.ActivityId), err
}

func (f chatFixture) chatActivities(t *testing.T) int {
	t.Helper()
	return f.env.WsCount(t, `SELECT count(*) FROM activity WHERE source_system = $1`, activities.ChatCaptureSourceSystem)
}

func TestAConfirmedChatScreenshotLandsAsOneMessageWithTheImageAttached(t *testing.T) {
	f := readChatScreenshot(t, chatReply())
	if n := f.chatActivities(t); n != 0 {
		t.Fatalf("the reading filed %d activities before the user confirmed", n)
	}
	body := chatScreenshotText
	filed, err := f.file(t, &body)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if n := f.env.WsCount(t, `SELECT count(*) FROM activity a
		  JOIN activity_link l ON l.activity_id = a.id AND l.contact_id = $2
		 WHERE a.id = $1 AND a.kind = 'message' AND a.channel_provider = 'whatsapp' AND a.body = $3`,
		filed, f.contact, chatScreenshotText); n != 1 {
		t.Fatal("the chat is not one whatsapp message on the contact carrying the read text")
	}
	if n := f.env.WsCount(t, `SELECT count(*) FROM attachment WHERE id = $1 AND entity_type = 'activity' AND entity_id = $2`,
		f.attachment, filed); n != 1 {
		t.Fatal("the screenshot did not move onto the activity it was filed as")
	}
	if n := f.env.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND captured_by LIKE '%attachment-extractor%'`, filed); n != 1 {
		t.Fatal("an unedited chat text is not marked as read by the model")
	}
	if n := f.env.WsCount(t, `SELECT count(*) FROM event_outbox o
		  JOIN audit_log a ON a.id = (o.envelope->'trace'->>'audit_log_id')::uuid
		 WHERE a.entity_id = $1 AND a.entity_type = 'activity' AND a.action = 'create'`, filed); n != 1 {
		t.Fatal("the activity write has no audit row with its outbox event")
	}
	again, err := f.file(t, &body)
	if err != nil || again != filed || f.chatActivities(t) != 1 {
		t.Fatalf("a repeated confirm wrote a second activity: %v %v", again, err)
	}
}

func TestAnEditedOrEmptyChatTextIsTheUsersOwn(t *testing.T) {
	for label, body := range map[string]*string{"edited": new("Lena: see you Thursday"), "empty": nil} {
		t.Run(label, func(t *testing.T) {
			f := readChatScreenshot(t, chatReply())
			filed, err := f.file(t, body)
			if err != nil {
				t.Fatalf("File: %v", err)
			}
			if n := f.env.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND captured_by = $2`,
				filed, "human:"+f.env.AdminUser.String()); n != 1 {
				t.Fatal("a chat text the user typed or left empty is not stamped as theirs")
			}
		})
	}
}

func TestAChatScreenshotIsStillFiledWhenTheModelCouldNotReadIt(t *testing.T) {
	f := readChatScreenshot(t, reply(notStated(chatFieldText), notStated(chatFieldName), notStated(chatFieldPhone)))
	if _, err := f.file(t, nil); err != nil {
		t.Fatalf("a screenshot with no readable chat could not be filed: %v", err)
	}
	if n := f.env.WsCount(t, `SELECT count(*) FROM attachment WHERE id = $1 AND entity_type = 'activity'`, f.attachment); n != 1 {
		t.Fatal("the image was not filed with the empty message")
	}
}

func TestAChatConfirmThatCannotBeTrustedWritesNothing(t *testing.T) {
	f := readChatScreenshot(t, chatReply())
	body := chatScreenshotText
	_, err := NewChatCaptureFile(f.env.Pool).File(f.env.Admin(), f.attachment, crmcontracts.FileChatCaptureRequest{
		ExtractionId: openapi_types.UUID(f.reading), ChannelProvider: "carrier_pigeon", Body: &body,
	})
	var refused *activities.ChatCaptureError
	if !errors.As(err, &refused) || refused.Field != "channel_provider" {
		t.Fatalf("an unregistered channel was not refused by name: %v", err)
	}
	_, err = NewChatCaptureFile(f.env.Pool).File(f.env.Admin(), f.attachment, crmcontracts.FileChatCaptureRequest{
		ExtractionId: openapi_types.UUID(ids.NewV7()), ChannelProvider: "whatsapp", Body: &body,
	})
	if err == nil {
		t.Fatal("a reading of nothing on this file was accepted")
	}
	if n := f.chatActivities(t); n != 0 {
		t.Fatalf("a refused confirm wrote %d activities", n)
	}
}

func TestAChatIsReadOnlyFromAnImageTheReaderTakes(t *testing.T) {
	env := integration.Setup(t)
	ctx := env.Admin()
	store := activities.NewStore(env.DB()).WithBlobstore(blobstore.NewMemory())
	contact := env.SeedContact(t, "Not An Image", &env.AdminUser)
	for _, file := range []struct{ name, mime string }{
		{"chat.txt", "text/plain"}, {"chat.tiff", "image/tiff"}, {"chat.heic", "image/heic"},
	} {
		att, err := store.UploadAttachment(ctx, activities.AttachmentInput{
			EntityType: "contact", EntityID: contact, Filename: file.name, ContentType: file.mime,
			Content: strings.NewReader("not a png"),
		})
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = store.StartExtractionReadFor(ctx, ids.UUID(att.Id), activities.ExtractionQuestionChatCapture, "rep", nil)
		var refused *activities.ChatCaptureError
		if !errors.As(err, &refused) || refused.Code != "not_an_image" {
			t.Errorf("a %s file was read as a chat: %v", file.mime, err)
		}
	}
}

// Art. 17: erasing the contact removes the filed screenshot's bytes, its row,
// and the reading that holds the copied text.
func TestErasingTheContactErasesTheFiledScreenshot(t *testing.T) {
	f := readChatScreenshot(t, chatReply())
	body := chatScreenshotText
	filed, err := f.file(t, &body)
	if err != nil {
		t.Fatal(err)
	}
	ctx := f.env.Admin()
	key := blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](f.env.WS), "attachment", f.attachment.String())
	if _, _, err := f.blob.Get(ctx, key); err != nil {
		t.Fatalf("precondition: the screenshot's bytes are stored: %v", err)
	}
	if err := eraserWithGraph(f.env.DB()).WithBlobstore(f.blob).EraseContact(ctx, f.contact, "test-erasure"); err != nil {
		t.Fatalf("EraseContact: %v", err)
	}
	if _, _, err := f.blob.Get(ctx, key); !errors.Is(err, blobstore.ErrNotFound) {
		t.Fatalf("the screenshot's bytes survived the erasure: %v", err)
	}
	if n := f.env.WsCount(t, `SELECT count(*) FROM attachment WHERE id = $1`, f.attachment); n != 0 {
		t.Fatal("the screenshot's row survived the erasure")
	}
	if n := f.env.WsCount(t, `SELECT count(*) FROM attachment_extraction WHERE id = $1`, f.reading); n != 0 {
		t.Fatal("the reading holding the chat text survived the erasure")
	}
	if n := f.env.WsCount(t, `SELECT count(*) FROM activity
		 WHERE id = $1 AND body IS NULL AND archived_at IS NOT NULL`, filed); n != 1 {
		t.Fatal("the filed chat kept its text, or stayed live, after the erasure")
	}
}

// A chat reading is the model's transcript of an image no quote could be
// checked against. The deal-facts poll does not return it, and an agent
// cannot read it.
func TestAChatReadingIsNotDealFactsAndNotForAnAgent(t *testing.T) {
	f := readChatScreenshot(t, chatReply())
	h := activities.NewHandlers(f.env.DB()).WithBlobstore(f.blob)
	poll := func(ctx context.Context, question *crmcontracts.GetAttachmentExtractionParamsQuestion) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/attachments/"+f.attachment.String()+"/extraction", nil).WithContext(ctx)
		h.GetAttachmentExtraction(rec, req, openapi_types.UUID(f.attachment),
			crmcontracts.GetAttachmentExtractionParams{Question: question})
		return rec
	}
	if rec := poll(f.env.Admin(), nil); rec.Code != http.StatusNotFound {
		t.Errorf("the deal-facts poll answered %d for a file read only as a chat, want 404: %s", rec.Code, rec.Body)
	}
	chat := crmcontracts.GetAttachmentExtractionParamsQuestionChatCapture
	rec := poll(f.env.Admin(), &chat)
	var got crmcontracts.AttachmentExtraction
	if err := json.Unmarshal(rec.Body.Bytes(), &got); rec.Code != http.StatusOK || err != nil ||
		got.Question != crmcontracts.AttachmentExtractionQuestionChatCapture {
		t.Fatalf("the chat poll = %d %s", rec.Code, rec.Body)
	}
	agent := f.env.AgentFor(t, f.env.AdminUser, nil, integration.AdminPerms)
	if rec := poll(agent, &chat); rec.Code != http.StatusForbidden {
		t.Errorf("an agent's chat poll answered %d, want 403", rec.Code)
	}
}
