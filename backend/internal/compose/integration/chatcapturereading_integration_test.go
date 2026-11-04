// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// chatLane is a model lane that carries PNG images and answers one script.
type chatLane struct{ *ai.FakeClient }

func (chatLane) AttachmentMIMEs() []string { return []string{"image/png"} }

const chatModelReply = `{"fields":[
 {"field":"chat_text","stated":"stated","value":"Laura: Can you send the quote by Friday?\nMe: Yes, Thursday.","source_quote":"Laura: Can you send the quote by Friday?","page_or_section":"messages","confidence":0.95},
 {"field":"counterparty_name","stated":"stated","value":"Laura Buyer","source_quote":"Laura Buyer","page_or_section":"header","confidence":0.95},
 {"field":"counterparty_phone","stated":"stated","value":"+49 151 2345678","source_quote":"+49 151 2345678","page_or_section":"header","confidence":0.9},
 {"field":"network","stated":"not_stated","value":"","source_quote":"","page_or_section":"","confidence":0}]}`

func screenshotPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	img.Set(1, 1, color.White)
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func uploadTo(t *testing.T, e *Env, store *activities.Store, entityType string, entityID ids.UUID,
	filename, contentType string, body []byte,
) ids.UUID {
	t.Helper()
	att, err := store.UploadAttachment(e.Admin(), activities.AttachmentInput{
		EntityType: entityType, EntityID: entityID, Filename: filename, ContentType: contentType,
		Content: bytes.NewReader(body),
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(att.Id)
}

func TestAChatScreenshotOnAContactIsReadForTheChat(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	store := activities.NewStore(e.DB()).WithBlobstore(blobstore.NewMemory())
	contact := e.SeedContact(t, "Laura Buyer", &e.Rep1)
	attachment := uploadTo(t, e, store, "contact", contact, "chat.png", "image/png", screenshotPNG(t))

	reading, _, err := store.StartExtractionReadFor(ctx, attachment, activities.ExtractionQuestionChatCapture, "manual", nil)
	if err != nil {
		t.Fatal(err)
	}
	reader := compose.NewDocumentExtractor(e.Pool, chatLane{ai.NewFakeClient().Script(chatModelReply)}, slog.New(slog.DiscardHandler))
	if err := reader.Read(ctx, store, reading.ID, attachment); err != nil {
		t.Fatal(err)
	}

	finished, err := store.GetExtractionRead(ctx, attachment, reading.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != activities.ExtractionReadDone || finished.Question != activities.ExtractionQuestionChatCapture {
		t.Fatalf("reading = %+v, want a done chat reading", finished)
	}
	got := map[string]string{}
	for _, f := range finished.Fields {
		if !f.Omitted {
			got[f.Field] = f.Value
		}
	}
	if got["chat_text"] != "Laura: Can you send the quote by Friday?\nMe: Yes, Thursday." ||
		got["counterparty_name"] != "Laura Buyer" || got["counterparty_phone"] != "+49 151 2345678" {
		t.Errorf("grounded = %v", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'attachment_extraction' AND entity_id = $1
		  AND after->>'question' = 'chat_capture'`, reading.ID); n != 1 {
		t.Errorf("the start wrote %d audit rows naming the chat question, want 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity`); n != 0 {
		t.Errorf("reading a chat wrote %d activities; only a human's confirm may file one", n)
	}
}

func TestAChatReadingRefusesAFileThatIsNotAnImage(t *testing.T) {
	e := Setup(t)
	store := activities.NewStore(e.DB()).WithBlobstore(blobstore.NewMemory())
	contact := e.SeedContact(t, "Laura Buyer", &e.Rep1)
	attachment := uploadTo(t, e, store, "contact", contact, "chat.txt", "text/plain", []byte("Laura: hello"))

	_, _, err := store.StartExtractionReadFor(e.Admin(), attachment, activities.ExtractionQuestionChatCapture, "manual", nil)
	var notImage *activities.NotAnImageError
	if !errors.As(err, &notImage) {
		t.Fatalf("err = %v, want the not-an-image refusal", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM attachment_extraction WHERE attachment_id = $1`, attachment); n != 0 {
		t.Errorf("a refused chat reading left %d reading rows", n)
	}
}

func TestAReadingInFlightForTheOtherQuestionIsNotJoined(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	store := activities.NewStore(e.DB()).WithBlobstore(blobstore.NewMemory())
	contact := e.SeedContact(t, "Laura Buyer", &e.Rep1)
	attachment := uploadTo(t, e, store, "contact", contact, "chat.png", "image/png", screenshotPNG(t))

	if _, _, err := store.StartExtractionReadQueued(ctx, attachment, "manual", nil); err != nil {
		t.Fatal(err)
	}
	_, joined, err := store.StartExtractionReadFor(ctx, attachment, activities.ExtractionQuestionChatCapture, "manual", nil)
	if !errors.Is(err, apperrors.ErrConflict) || joined {
		t.Fatalf("joined = %v, err = %v; want a conflict, since the reading in flight answers another question", joined, err)
	}
}

func TestAChatReadingCannotBeAcceptedOntoADeal(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	store := activities.NewStore(e.DB()).WithBlobstore(blobstore.NewMemory())
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Accept Target", pipeline, open, &e.Rep1)
	attachment := uploadTo(t, e, store, "deal", deal, "chat.png", "image/png", screenshotPNG(t))
	reading, _, err := store.StartExtractionReadFor(ctx, attachment, activities.ExtractionQuestionChatCapture, "manual", nil)
	if err != nil {
		t.Fatal(err)
	}
	reader := compose.NewDocumentExtractor(e.Pool, chatLane{ai.NewFakeClient().Script(chatModelReply)}, slog.New(slog.DiscardHandler))
	if err := reader.Read(ctx, store, reading.ID, attachment); err != nil {
		t.Fatal(err)
	}

	_, err = compose.NewExtractionAccept(e.Pool).Accept(ctx, attachment, crmcontracts.AcceptExtractionRequest{
		ExtractionId: openapi_types.UUID(reading.ID), FieldKeys: []string{"name"},
	})
	var refused *compose.ExtractionAcceptError
	if !errors.As(err, &refused) || refused.Code != "not_deal_facts" {
		t.Fatalf("err = %v, want the not-deal-facts refusal", err)
	}
}
