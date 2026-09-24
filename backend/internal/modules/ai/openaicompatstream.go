// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// openAICompatStream reads the OpenAI-compatible SSE stream: `data: {...}`
// lines, terminated by `data: [DONE]`. The reply's terminal is the choice's
// finish_reason, read through the same openAICompatChoice.terminal Complete
// uses; [DONE] alone ends a reply whose host never states one.
type openAICompatStream struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	end     streamEnd
}

type openAICompatStreamEvent struct {
	Choices []openAICompatStreamChoice `json:"choices"`
	// Error is a broker's report, beside the choices or instead of them, that
	// the upstream failed after the 200 went out.
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// openAICompatStreamChoice is one streamed choice: Complete's choice, with the
// increment in delta where a whole reply has message.
type openAICompatStreamChoice struct {
	openAICompatChoice
	Delta struct {
		Content string `json:"content"`
		Refusal string `json:"refusal"`
	} `json:"delta"`
}

func (s *openAICompatStream) Next(ctx context.Context) (string, bool, error) {
	for !s.end.read && s.scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		line := strings.TrimSpace(s.scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			s.end.finish("")
			continue
		}
		var ev openAICompatStreamEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return "", false, fmt.Errorf("ai: openai-compat: stream event: %w", err)
		}
		chunk, err := s.read(ctx, ev)
		if err != nil || chunk != "" {
			return chunk, err == nil, err
		}
	}
	return s.end.outcome(s.scanner.Err())
}

// read takes one event's text, and records its terminal when it carries one.
func (s *openAICompatStream) read(ctx context.Context, ev openAICompatStreamEvent) (string, error) {
	if len(ev.Choices) == 0 {
		// A usage-only chunk carries no choice; a broker's failure may not either.
		ev.Choices = []openAICompatStreamChoice{{}}
	}
	choice := ev.Choices[0]
	if choice.Error == nil {
		choice.Error = ev.Error
	}
	choice.Message.Refusal = choice.Delta.Refusal
	if choice.FinishReason != "" || choice.Error != nil || choice.Message.Refusal != "" {
		finish, err := choice.terminal(ctx)
		if err != nil {
			return "", err
		}
		s.end.finish(finish)
	}
	return choice.Delta.Content, nil
}

func (s *openAICompatStream) Close() error { return s.body.Close() }
