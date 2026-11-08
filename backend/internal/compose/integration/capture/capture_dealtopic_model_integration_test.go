// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// topicModel stands in for the capture_classify lane the deal topic check
// asks. By default one deal asked about is "about" it with confidence 0.95,
// and several are "unsure", which is how capture filed before the check. A
// test sets answer to judge otherwise, unconfigured to have no model, or
// forbid to fail when it is asked at all.
type topicModel struct {
	mu           sync.Mutex
	answer       func(deal string) (verdict string, confidence float64)
	unconfigured bool
	forbid       *testing.T
	calls        int
	// prompts holds the user turns the check sent, in order.
	prompts []string
	// during runs once, while the model is answering its first question.
	during func()
}

func (m *topicModel) Complete(_ context.Context, req model.Request) (model.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	for _, msg := range req.Messages {
		m.prompts = append(m.prompts, msg.Content)
	}
	if during := m.during; during != nil {
		m.during = nil
		during()
	}
	if m.forbid != nil {
		m.forbid.Errorf("the deal topic check asked the model for a mail filed on explicit evidence")
	}
	if m.unconfigured {
		return model.Response{}, ai.ErrUnconfiguredModel
	}
	deals, err := askedDeals(req)
	if err != nil {
		return model.Response{}, err
	}
	results := make([]string, len(deals))
	for i, deal := range deals {
		verdict, confidence := "unsure", 0.5
		if len(deals) == 1 {
			verdict, confidence = "about", 0.95
		}
		if m.answer != nil {
			verdict, confidence = m.answer(deal)
		}
		results[i] = fmt.Sprintf(`{"deal_id":%q,"verdict":%q,"confidence":%v}`, deal, verdict, confidence)
	}
	return model.Response{Text: `{"results":[` + strings.Join(results, ",") + `]}`}, nil
}

// refiler is the refile pass's repair: the data task's, with the topic check
// asking the env's model.
func refiler(env captureEnv, w dealWorld) repairer {
	r := newRepairer(env.e, w)
	r.repair.Check = compose.NewDealTopicCheck(env.e.Pool, env.topic)
	return r
}

// sent returns the user turns the check sent.
func (m *topicModel) sent() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.prompts...)
}

// asked is how many times the check called the model.
func (m *topicModel) asked() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// askedDeals reads the deal ids a request asks about from its own schema.
func askedDeals(req model.Request) ([]string, error) {
	var shape struct {
		Properties struct {
			Results struct {
				Items struct {
					Properties struct {
						DealID struct {
							Enum []string `json:"enum"`
						} `json:"deal_id"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"results"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(req.ResponseSchema, &shape); err != nil {
		return nil, fmt.Errorf("the topic request carries no schema to read deal ids from: %w", err)
	}
	return shape.Properties.Results.Items.Properties.DealID.Enum, nil
}
