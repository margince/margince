// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const chatScreenshotText = "Lena Hoffmann: Are we still on for Thursday? Me: Yes, 10am at the main gate."

func chatReply() string {
	return reply(
		statedField(chatFieldText, chatScreenshotText, "Lena Hoffmann: Are we still on for Thursday?"),
		statedField(chatFieldName, "Lena Hoffmann", "Lena Hoffmann"),
		notStated(chatFieldPhone))
}

func pngAttachment() crmcontracts.Attachment {
	mime := "image/png"
	return crmcontracts.Attachment{Filename: "chat.png", ContentType: &mime}
}

func chatSource(t *testing.T) documentSource {
	t.Helper()
	return documentSource{Part: model.Attachment{MIME: "image/png", Bytes: cardPNG(t)}}
}

func TestAChatScreenshotIsReadForItsMessagesAndTheOtherPerson(t *testing.T) {
	store := &fakeReadStore{meta: pngAttachment(), body: string(cardPNG(t)),
		question: activities.ExtractionQuestionChatCapture}
	brain := scriptedBrain{FakeClient: ai.NewFakeClient().Script(chatReply()), carriage: carriesImagesOnly}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if store.outcome == nil || store.outcome.Status != activities.ExtractionReadDone {
		t.Fatalf("the reading did not finish done: %+v", store.outcome)
	}
	got := map[string]string{}
	for _, f := range store.outcome.Fields {
		if !f.Omitted {
			got[f.Field] = f.Value
		}
	}
	if got[chatFieldText] != chatScreenshotText || got[chatFieldName] != "Lena Hoffmann" {
		t.Fatalf("the reading did not ground the chat as the screenshot shows it: %+v", got)
	}
	if _, offered := got[chatFieldPhone]; offered {
		t.Fatal("the reading offers a number the header does not print")
	}
	payload := string(brain.Calls()[0].Payload)
	if !strings.Contains(payload, "screenshot of a conversation") {
		t.Errorf("the chat reading did not put the chat question to the model:\n%s", payload)
	}
}

func TestAChatScreenshotShowingNoChatSaysWhyNothingIsOffered(t *testing.T) {
	store := &fakeReadStore{meta: pngAttachment(), body: string(cardPNG(t)),
		question: activities.ExtractionQuestionChatCapture}
	empty := reply(notStated(chatFieldText), notStated(chatFieldName), notStated(chatFieldPhone))
	brain := scriptedBrain{FakeClient: ai.NewFakeClient().Script(empty), carriage: carriesImagesOnly}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if store.outcome.Status != activities.ExtractionReadDone ||
		store.outcome.Detail != chatWords.NoChat.En {
		t.Fatalf("an empty chat reading must finish done with the chat sentence, got %+v", store.outcome)
	}
}

func TestTheChatValidatorHoldsTheClosedFieldSet(t *testing.T) {
	q := chatCaptureQuestion()
	text := statedField(chatFieldText, "Me: hi", "Me: hi")
	name := notStated(chatFieldName)
	phone := notStated(chatFieldPhone)
	for label, out := range map[string]string{
		"a deal field":    reply(text, name, phone, notStated("currency")),
		"a field twice":   reply(text, text, name, phone),
		"a missing field": reply(text, name),
	} {
		if err := questionShapeValid(q, chatSource(t))(out); err == nil {
			t.Errorf("the chat validator accepted a reply with %s", label)
		}
	}
	if err := questionShapeValid(q, chatSource(t))(reply(text, name, phone)); err != nil {
		t.Fatalf("the chat validator refused a complete reply: %v", err)
	}
}

func TestAChatTextLongerThanAScreenCarriesIsRefused(t *testing.T) {
	long := strings.Repeat("x", maxChatText+1)
	out := reply(statedField(chatFieldText, long, "x"), notStated(chatFieldName), notStated(chatFieldPhone))
	if err := questionShapeValid(chatCaptureQuestion(), chatSource(t))(out); err == nil {
		t.Fatal("the chat validator accepted more text than a screenshot shows")
	}
	if maxChatText <= maxDocumentValue {
		t.Fatal("a chat text must be allowed to run longer than a deal value")
	}
}

func TestAChatQuoteBelowTheFloorIsOmittedNotOffered(t *testing.T) {
	unsure := strings.Replace(statedField(chatFieldText, "Me: hi", "Me: hi"), `"confidence":0.95`, `"confidence":0.4`, 1)
	fields, err := readQuestionFields(chatCaptureQuestion(), reply(unsure, notStated(chatFieldName), notStated(chatFieldPhone)))
	if err != nil {
		t.Fatal(err)
	}
	if !fields[0].Omitted || fields[0].OmittedReason != omitReasonNotConfident {
		t.Fatalf("an unsure chat text was offered: %+v", fields[0])
	}
}
