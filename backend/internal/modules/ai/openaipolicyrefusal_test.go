// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// OpenAI refuses a prompt its usage policy flags with a 400 before generating
// anything. That is the policy deciding about the content, the same decision a
// failed response with the same code reports, so it is an answer withheld.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestAPromptOpenAIsPolicyFlagsIsAnAnswerWithheld(t *testing.T) {
	for name, body := range map[string]string{
		"usage policy":        `{"error":{"message":"Invalid prompt: your prompt was flagged as potentially violating our usage policy","type":"invalid_request_error","param":null,"code":"invalid_prompt"}}`,
		"repetitive patterns": `{"error":{"message":"Sorry! We've encountered an issue with repetitive patterns in your prompt. Please try again with a different prompt.","type":"invalid_request_error","param":"prompt","code":"invalid_prompt"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := refusingAdapter(t, refusalFixture{provider: providerOpenAI, status: http.StatusBadRequest, body: body}).Complete(
				context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "q"}}},
			)
			if !errors.Is(err, model.ErrOutputWithheld) {
				t.Errorf("a policy refusal of the prompt read as %v, want the answer withheld", err)
			}
			if errors.Is(err, model.ErrRequestRejected) {
				t.Errorf("a policy refusal read as a malformed request: %v", err)
			}
		})
	}
}
