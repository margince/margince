// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/extraction"
)

const chatLines = `Laura: Can you send the quote by Friday?\nMe: Yes, it goes out Thursday.`

// chatReply renders the four chat fields, each override replacing one in order.
func chatReply(overrides ...string) string {
	fields := []string{
		notStated(chatFieldText), notStated(chatFieldName),
		notStated(chatFieldPhone), notStated(chatFieldNetwork),
	}
	for i, override := range overrides {
		if override != "" {
			fields[i] = override
		}
	}
	return reply(fields...)
}

func groundedChatReply() string {
	return chatReply(
		statedField(chatFieldText, chatLines, "Laura: Can you send the quote by Friday?"),
		statedField(chatFieldName, "Laura Buyer", "Laura Buyer +49 151 2345678"),
		statedField(chatFieldPhone, "+49 151 2345678", "Laura Buyer +49 151 2345678"),
		"")
}

// imageSourceOnly is a chat screenshot on the bytes lane, where no quote can be
// checked against text.
func imageSourceOnly() documentSource { return documentSource{} }

func TestAChatReplyGroundsTheMessagesAndTheHeader(t *testing.T) {
	if err := documentShapeValid(chatCapture, imageSourceOnly())(groundedChatReply()); err != nil {
		t.Fatalf("a well-formed chat reply was refused: %v", err)
	}
	fields, err := readDocumentFields(chatCapture, groundedChatReply())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]extraction.ExtractedField{}
	for _, f := range fields {
		got[f.Field] = f
	}
	if got[chatFieldText].Value != "Laura: Can you send the quote by Friday?\nMe: Yes, it goes out Thursday." {
		t.Errorf("chat_text = %q, want the two messages on their own lines", got[chatFieldText].Value)
	}
	if got[chatFieldName].Value != "Laura Buyer" || got[chatFieldPhone].Value != "+49 151 2345678" {
		t.Errorf("header = %q / %q", got[chatFieldName].Value, got[chatFieldPhone].Value)
	}
	if !got[chatFieldNetwork].Omitted || got[chatFieldNetwork].OmittedReason != omitReasonNotStated {
		t.Errorf("an app the screenshot does not name must be omitted as not stated, got %+v", got[chatFieldNetwork])
	}
}

func TestAChatReplyOutsideTheClosedSetIsRefused(t *testing.T) {
	cases := map[string]string{
		"a field nobody asked for": reply(
			statedField(chatFieldText, chatLines, "Laura: Can you send the quote by Friday?"),
			notStated(chatFieldName), notStated(chatFieldPhone), notStated(chatFieldNetwork),
			notStated("email")),
		"the same field twice": reply(
			notStated(chatFieldText), notStated(chatFieldText),
			notStated(chatFieldName), notStated(chatFieldPhone), notStated(chatFieldNetwork)),
		"a field left out":     reply(notStated(chatFieldText), notStated(chatFieldName), notStated(chatFieldPhone)),
		"a deal field instead": allFour(),
		"a header name its quote does not hold": chatReply("",
			statedField(chatFieldName, "Laura Buyer", "Max Mustermann")),
		"a message quote the messages do not hold": chatReply(
			statedField(chatFieldText, chatLines, "See you on Monday")),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if err := documentShapeValid(chatCapture, imageSourceOnly())(body); err == nil {
				t.Error("the reply was accepted")
			}
		})
	}
}

func TestAnUnsureOrUndiallableHeaderIsNotOffered(t *testing.T) {
	unsure := strings.Replace(
		statedField(chatFieldName, "Laura Buyer", "Laura Buyer"), `"confidence":0.95`, `"confidence":0.4`, 1)
	fields, err := readDocumentFields(chatCapture, chatReply("", unsure,
		statedField(chatFieldPhone, "12:45", "12:45")))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fields {
		if (f.Field == chatFieldName || f.Field == chatFieldPhone) && !f.Omitted {
			t.Errorf("%s = %q was offered", f.Field, f.Value)
		}
	}
}

func TestAChatQuestionAsksForTheChatAndNotTheDeal(t *testing.T) {
	req := documentExtractRequest(chatCapture, documentSource{Part: imageSourceOnly().Part})
	turn := req.Messages[0].Content
	for _, want := range []string{"One chat screenshot (untrusted).", chatFieldText, chatFieldPhone} {
		if !strings.Contains(turn, want) {
			t.Errorf("the chat prompt does not say %q:\n%s", want, turn)
		}
	}
	if strings.Contains(turn, modelFieldAmount) || strings.Contains(req.System, "order form") {
		t.Errorf("the chat prompt asks about deal facts:\n%s\n%s", req.System, turn)
	}
}

func TestAChatScreenshotIsReadForTheChat(t *testing.T) {
	png := "image/png"
	store := &fakeReadStore{
		meta:     crmcontracts.Attachment{Filename: "chat.png", ContentType: &png},
		body:     string(cardPNG(t)),
		question: activities.ExtractionQuestionChatCapture,
	}
	brain := scriptedBrain{FakeClient: ai.NewFakeClient().Script(groundedChatReply()), carriage: []string{"image/png"}}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if store.outcome == nil || store.outcome.Status != activities.ExtractionReadDone {
		t.Fatalf("outcome = %+v, want done", store.outcome)
	}
	if !strings.Contains(string(brain.Calls()[0].Payload), "One chat screenshot") {
		t.Error("the reading did not ask the question its row names")
	}
	names := []string{}
	for _, f := range store.outcome.Fields {
		names = append(names, f.Field)
	}
	if strings.Join(names, ",") != strings.Join(chatFieldOrder(), ",") {
		t.Errorf("stored fields %v, want %v", names, chatFieldOrder())
	}
}

func TestAChatScreenshotShowingNoMessagesSaysSo(t *testing.T) {
	png := "image/png"
	store := &fakeReadStore{
		meta:     crmcontracts.Attachment{Filename: "chat.png", ContentType: &png},
		body:     string(cardPNG(t)),
		question: activities.ExtractionQuestionChatCapture,
	}
	brain := scriptedBrain{FakeClient: ai.NewFakeClient().Script(chatReply()), carriage: []string{"image/png"}}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if store.outcome == nil || store.outcome.Detail != documentWords.NoChat.En {
		t.Fatalf("outcome = %+v, want done with the empty-chat sentence", store.outcome)
	}
}

func TestEveryAttachmentParentIsCitedAsItsOwnKind(t *testing.T) {
	for _, kind := range []crmcontracts.AttachmentEntityType{
		crmcontracts.AttachmentEntityTypeActivity, crmcontracts.AttachmentEntityTypeCompany,
		crmcontracts.AttachmentEntityTypeContact, crmcontracts.AttachmentEntityTypeDeal,
		crmcontracts.AttachmentEntityTypeLead, crmcontracts.AttachmentEntityTypeProject,
	} {
		id := ids.NewV7()
		ref := attachmentParent(crmcontracts.Attachment{EntityType: kind, EntityId: openapi_types.UUID(id)})
		if ref.Type != string(kind) || ref.ID != id {
			t.Errorf("a %s parent is cited as %s %s", kind, ref.Type, ref.ID)
		}
	}
}
