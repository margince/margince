// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for document_extract/chat.
//
// It certifies the shipped path: chatCaptureRequest, then the reading's own
// validator and grounding. The expectation names the other party when the
// header prints them, and words each message must carry. The line layout is
// the model's and is not compared. An empty expectation is the abstention.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const laneChat = "chat"

type chatCaptureFixture struct {
	MIME     string `json:"mime"`
	Bytes    []byte `json:"bytes"`
	Filename string `json:"filename"`
}

// chatCaptureExpectation is what a screenshot shows.
type chatCaptureExpectation struct {
	CounterpartyName  string   `json:"counterparty_name"`
	CounterpartyPhone string   `json:"counterparty_phone"`
	Messages          []string `json:"messages"`
}

type chatCaptureCases struct{}

func (chatCaptureCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskDocumentExtract, Variant: laneChat, Kind: ai.SiteKindOneShot}
}

// CertifiedScope is the site's own kind: a screenshot is read in one call.
func (chatCaptureCases) CertifiedScope() string { return aitasks.ScopeFullInvocation }

//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (chatCaptureCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f chatCaptureFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("document_extract/chat: the fixture is not the shape this site takes: %w", err)
	}
	if len(f.Bytes) == 0 || len(f.Bytes) > imagenorm.MaxReadImageBytes {
		return nil, fmt.Errorf("document_extract/chat: the fixture must be a screenshot of 1 to %d bytes", imagenorm.MaxReadImageBytes)
	}
	if !ai.CarriesImage(f.MIME) {
		return nil, fmt.Errorf("document_extract/chat: %q is not an image type a reading accepts", f.MIME)
	}
	var want chatCaptureExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("document_extract/chat: the expected answer names the other party and words of each message: %w", err)
	}
	src := documentSource{Part: model.Attachment{MIME: f.MIME, Bytes: f.Bytes, Name: f.Filename}, Filename: f.Filename}
	return &chatCaptureCase{src: src, want: want}, nil
}

type chatCaptureCase struct {
	src  documentSource
	want chatCaptureExpectation
}

// Run issues the one request this site sends, bare, as the other one-call
// sites do.
func (c *chatCaptureCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := chatCaptureRequest(c.src)
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("document_extract/chat: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate applies the reading's own validator and grounding, then compares.
func (c *chatCaptureCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	q := chatCaptureQuestion()
	if err := questionShapeValid(q, c.src)(trace.Output); err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	fields, err := readQuestionFields(q, trace.Output)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	grounded := map[string]string{}
	for _, f := range fields {
		if !f.Omitted {
			grounded[f.Field] = f.Value
		}
	}
	if missed := c.want.disagreements(grounded); len(missed) > 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: strings.Join(missed, "; ")}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

func (want chatCaptureExpectation) disagreements(grounded map[string]string) []string {
	var out []string
	text, offered := grounded[chatFieldText]
	if len(want.Messages) == 0 && offered {
		out = append(out, "the reading offers chat text for a picture that shows no chat")
	}
	for _, words := range want.Messages {
		if !strings.Contains(strings.ToLower(text), strings.ToLower(words)) {
			out = append(out, fmt.Sprintf("the chat text does not carry %q", words))
		}
	}
	header := []struct{ field, expected string }{
		{chatFieldName, want.CounterpartyName}, {chatFieldPhone, want.CounterpartyPhone},
	}
	for _, h := range header {
		field, expected := h.field, h.expected
		got, has := grounded[field]
		switch {
		case expected == "" && has:
			out = append(out, fmt.Sprintf("the reading offers %s = %q, which the header does not print", field, got))
		case expected != "" && !strings.EqualFold(strings.TrimSpace(got), expected):
			out = append(out, fmt.Sprintf("the reading offers %s = %q where the header prints %q", field, got, expected))
		}
	}
	return out
}
