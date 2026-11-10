// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/decision"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// ctxMeter refuses a write on a canceled context, as a database write does.
type ctxMeter struct{ records []Usage }

func (m *ctxMeter) Record(ctx context.Context, u Usage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.records = append(m.records, u)
	return nil
}

func (m *ctxMeter) MonthTokens(context.Context) (int64, error) { return 0, nil }

// ctxSpender is the agent's share, refusing a canceled context the same way.
type ctxSpender struct{ tokens int }

func (s *ctxSpender) SpendAgentTokens(ctx context.Context, tokens int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.tokens += tokens
	return nil
}

// answerThenCancel answers and then loses its caller, the way a page closes
// while the provider's reply is on the wire.
type answerThenCancel struct {
	stubClient
	cancel context.CancelFunc
}

func (c answerThenCancel) Complete(context.Context, model.Request) (model.Response, error) {
	c.cancel()
	return model.Response{Text: "an answer", InputTokens: 30, OutputTokens: 5}, nil
}

func (c answerThenCancel) Embed(context.Context, model.EmbedRequest) (model.Embeddings, error) {
	c.cancel()
	return model.Embeddings{Vectors: [][]float32{{1, 0}}}, nil
}

// decideThenCancel answers the decision and then loses its caller.
type decideThenCancel struct{ cancel context.CancelFunc }

func (d decideThenCancel) Decide(context.Context, decision.Request) (decision.Response, error) {
	d.cancel()
	return answered("question", 0.95).resp, nil
}

// Tokens a provider spent are billed though the caller left before the write.
// They go to the workspace's usage and to the agent's share, on every kind of call.
func TestAServedCallIsBilledAfterItsCallerLeft(t *testing.T) {
	cases := map[string]func(context.Context, context.CancelFunc, *ctxMeter, *ctxSpender) int{
		"completion": func(ctx context.Context, cancel context.CancelFunc, meter *ctxMeter, spend *ctxSpender) int {
			r := assembleRouter(map[Tier]model.Client{TierCheapCloud: answerThenCancel{cancel: cancel}},
				nil, ProfileCloudFrontier, meter, unlimitedBudget{}, nil,
				map[Tier]routeMeta{TierCheapCloud: {provider: "openai", model: "gpt-x"}}, false, nil).WithAgentTokenSpend(spend)
			if _, _, err := r.serveCompletion(ctx, TaskColdStart, []Tier{TierCheapCloud}, model.Request{}); err != nil {
				t.Errorf("completion: %v", err)
			}
			return 35
		},
		"decision": func(ctx context.Context, cancel context.CancelFunc, meter *ctxMeter, spend *ctxSpender) int {
			f := newDecideFixture(t, decideThenCancel{cancel: cancel}, 0)
			f.router.meter = meter
			f.router.WithAgentTokenSpend(spend)
			// The answered decision is billed whether or not it stood; a caller
			// gone by then is told so and gets no ladder.
			if _, _, err := f.router.Decide(ctx, TaskSiteTriage, "triage", decision.Request{}, triageLLMRequest, acceptAnything, floorGate); err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("decision: %v", err)
			}
			return 400
		},
		"embedding": func(ctx context.Context, cancel context.CancelFunc, meter *ctxMeter, spend *ctxSpender) int {
			r := assembleRouter(map[Tier]model.Client{}, answerThenCancel{cancel: cancel}, ProfileEUHosted, meter,
				DefaultMonthlyTokens, nil, nil, false, nil).WithAgentTokenSpend(spend)
			if _, err := r.Embed(ctx, model.EmbedRequest{Inputs: []string{"a query"}}); err != nil {
				t.Errorf("embedding: %v", err)
			}
			return int(embedTokenEstimate([]string{"a query"}))
		},
	}
	for name, call := range cases {
		ctx, cancel := context.WithCancel(wsContext(t))
		meter, spend := &ctxMeter{}, &ctxSpender{}
		want := call(ctx, cancel, meter, spend)
		cancel()
		if len(meter.records) != 1 {
			t.Errorf("%s: %d usage rows written, want the served call's one", name, len(meter.records))
		}
		if spend.tokens != want {
			t.Errorf("%s: the agent was charged %d tokens, want %d", name, spend.tokens, want)
		}
	}
}
