// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Reading a chat screenshot: the messages it shows, and the name and number of
// the other party in the chat header. The text becomes the body of a message
// activity only when the user files it. The name and number are shown so the
// user can see whether the chat belongs on this contact.

import (
	"strings"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/ports/extraction"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// The fields a chat reading reports, in the order it is asked for them.
// The activities store owns the names, because the confirm reads them back.
const (
	chatFieldText  = activities.ChatFieldText
	chatFieldName  = activities.ChatFieldName
	chatFieldPhone = activities.ChatFieldPhone
)

// maxChatText bounds the messages one screenshot yields, and what the user may
// file: one number for the read text and the typed one.
const maxChatText = activities.MaxChatText

const chatCaptureSystem = `You read ONE screenshot of a conversation in a messaging app and report only what
it SHOWS. Copy the messages exactly as printed, in their own language and script, in the order
they appear. Never translate, summarize, complete or correct a message, and never add one that is
not on the screen. The other party's name and phone number are reported only when the chat
header prints them. A screenshot with no conversation in it is a normal answer: every field is
not_stated.`

func chatCaptureInstruction() string {
	return `One screenshot (untrusted). Return JSON: { "fields": [ { "field", "stated", "value", "source_quote", "page_or_section", "confidence" } ] } — ` +
		`one entry for EACH of ` + strings.Join([]string{chatFieldText, chatFieldName, chatFieldPhone}, ", ") +
		`, in that order, and no others. Set "stated" to "` + documentStated + `" for what the screenshot shows and "` +
		documentNotStated + `" for what it does not, leaving the other values empty. ` +
		`"value" for ` + chatFieldText + ` is every visible message, one per line, each line starting with the sender's name as printed, or "Me" for the messages on the device owner's side. ` +
		`For ` + chatFieldName + ` it is the other party's name as the chat header prints it; for ` + chatFieldPhone + ` the phone number the header prints. ` +
		`"source_quote" is the first line of the value, at most 200 characters, copied verbatim. ` +
		`"page_or_section" is "header" or "messages". "confidence" is your own certainty, from 0 to 1, that the value is read correctly.`
}

// chatCaptureReading is the question put to the model for one screenshot, a
// pure function so the certification case issues the shipping request.
//
//promptlang:exempt every value returned is copied from the screenshot verbatim, in the chat's own language, so instructing a language would ask the model to translate what has to match the print.
//promptvoice:exempt every value returned is copied from the screenshot verbatim, never prose in Margince's voice.
func chatCaptureReading(img model.Attachment) ai.ImageReading {
	return ai.ImageReading{
		Image:       img,
		System:      chatCaptureSystem,
		Instruction: chatCaptureInstruction(),
		Schema:      documentExtractSchema(),
		Validate:    questionShapeValid(chatCaptureQuestion(), documentSource{Part: img}),
	}
}

func chatCaptureRequest(src documentSource) model.Request {
	req := ai.ImageRequest(chatCaptureReading(src.Part), src.Part)
	req.Subjects = documentSubjects(src.Parent)
	return req
}

func chatCaptureQuestion() documentQuestion {
	same := func(field string) string { return field }
	return documentQuestion{
		name:      activities.ExtractionQuestionChatCapture,
		fields:    []string{chatFieldText, chatFieldName, chatFieldPhone},
		toModel:   same,
		fromModel: same,
		valueBound: func(field string) int {
			if field == chatFieldText {
				return maxChatText
			}
			return maxDocumentValue
		},
		coerce: func(_, raw string) (string, bool) {
			value := strings.TrimSpace(raw)
			return value, value != ""
		},
		settle:       func(fields []extraction.ExtractedField) []extraction.ExtractedField { return fields },
		request:      chatCaptureRequest,
		noneGrounded: chatWords.NoChat,
	}
}

var chatWords = struct{ NoChat langcopy.Phrase }{
	NoChat: langcopy.Phrase{
		En: "this screenshot shows no chat clearly enough to copy it; type what was said instead",
		De: "Auf diesem Screenshot ist kein Chat deutlich genug zu lesen. Gib das Gesagte selbst ein.",
		Vi: "Ảnh chụp màn hình này không hiển thị đủ rõ cuộc trò chuyện để sao chép; hãy tự nhập nội dung.",
	},
}
