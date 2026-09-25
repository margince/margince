// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// recordingClient counts what reached the provider. The assertion this file
// exists for is not "the call failed" but "the prompt never left", and only a
// client that reports being called can tell those apart.
type recordingClient struct {
	calls int
	resp  model.Response
}

func (c *recordingClient) Complete(context.Context, model.Request) (model.Response, error) {
	c.calls++
	return c.resp, nil
}

func (c *recordingClient) Stream(context.Context, model.Request) (model.TokenStream, error) {
	return nil, errors.New("unused")
}

func (c *recordingClient) Embed(context.Context, model.EmbedRequest) (model.Embeddings, error) {
	return model.Embeddings{}, errors.New("unused")
}

func (c *recordingClient) Caps() model.Capabilities { return model.Capabilities{} }

func localOnlyRouter(t *testing.T, provider string, client model.Client) *Router {
	t.Helper()
	return assembleRouter(
		map[Tier]model.Client{TierLocalSmall: client},
		nil, ProfileEUHosted, stubMeter{}, unlimitedBudget{}, &fakeCallStore{},
		map[Tier]routeMeta{TierLocalSmall: {provider: provider, model: "m"}},
		false, nil,
	)
}

// TestALocalOnlyTaskNeverReachesAHostedProvider is the guarantee itself.
//
// capture_counterparty_verdict decides whether an unjudged sender is personal,
// and its prompt carries that sender's subject and body. The ladder is
// [local_small] — but local_small is a size class, and an eu_hosted profile
// legitimately binds it to a cloud vendor, which is what four of the five
// shipped presets do. The tier name cannot carry this; the task's own
// local_only declaration does.
func TestALocalOnlyTaskNeverReachesAHostedProvider(t *testing.T) {
	client := &recordingClient{resp: model.Response{Text: "contact"}}
	r := localOnlyRouter(t, "gemini", client)

	_, _, err := r.Complete(wsCtx(), TaskCaptureCounterpartyVerdict, model.Request{})

	if err == nil {
		t.Fatal("a local_only task served by a hosted rung must be refused, not answered")
	}
	if client.calls != 0 {
		t.Fatalf("the prompt reached the hosted provider %d time(s) — refusing after the call is not refusing", client.calls)
	}
	if !strings.Contains(err.Error(), string(TaskCaptureCounterpartyVerdict)) {
		t.Errorf("the refusal must name the task it refused: %v", err)
	}
	if !strings.Contains(err.Error(), "gemini") {
		t.Errorf("the refusal must name what the rung is bound to, or the operator cannot find it: %v", err)
	}
}

// TestALocalOnlyTaskServesOnALocalRung: the narrowing refuses a hosted binding,
// not the task. An operator who binds local_small to ollama gets the feature.
func TestALocalOnlyTaskServesOnALocalRung(t *testing.T) {
	client := &recordingClient{resp: model.Response{Text: "contact", OutputTokens: 1}}
	r := localOnlyRouter(t, providerOllama, client)

	resp, info, err := r.Complete(wsCtx(), TaskCaptureCounterpartyVerdict, model.Request{})
	if err != nil {
		t.Fatalf("a local rung must serve a local_only task: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("want exactly one provider call, got %d", client.calls)
	}
	if info.Tier != TierLocalSmall || resp.Text != "contact" {
		t.Fatalf("want the local_small answer, got tier %s text %q", info.Tier, resp.Text)
	}
}

// TestAnOrdinaryTaskStillServesOnAHostedRung guards the other direction. The
// narrowing must not reach tasks that never claimed the guarantee — most tasks
// ride local_small legitimately, and refusing them would take the shipped
// presets offline in the name of two tasks.
func TestAnOrdinaryTaskStillServesOnAHostedRung(t *testing.T) {
	if LocalOnly(TaskColdStart) {
		t.Fatalf("this test's premise is gone: %s now declares local_only", TaskColdStart)
	}
	client := &recordingClient{resp: model.Response{Text: "ok", OutputTokens: 1}}
	r := localOnlyRouter(t, "gemini", client)

	if _, _, err := r.serveCompletion(wsCtx(), TaskColdStart, []Tier{TierLocalSmall}, model.Request{}); err != nil {
		t.Fatalf("a task without local_only must still serve on a hosted rung: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("want exactly one provider call, got %d", client.calls)
	}
}

// TestALocalOnlyTaskKeepsOnlyItsLocalRungs: a mixed ladder drops the hosted
// rungs and keeps the local ones, rather than refusing wholesale. A ladder
// degrades, so this is the case where the guarantee is most easily lost — the
// first rung is local, nobody notices, and the fallback is a vendor.
func TestALocalOnlyTaskKeepsOnlyItsLocalRungs(t *testing.T) {
	meta := map[Tier]routeMeta{
		TierLocalSmall: {provider: providerOllama, model: "m"},
		TierCheapCloud: {provider: "gemini", model: "m"},
		TierPremium:    {provider: "anthropic", model: "m"},
	}
	ladder := []Tier{TierLocalSmall, TierCheapCloud, TierPremium}

	got := localOnlyLadder(TaskCaptureCounterpartyVerdict, meta, ladder)

	if len(got) != 1 || got[0] != TierLocalSmall {
		t.Fatalf("want only the local rung, got %v", got)
	}
	if kept := localOnlyLadder(TaskColdStart, meta, ladder); len(kept) != 3 {
		t.Fatalf("an ordinary task keeps its whole ladder, got %v", kept)
	}
}

// TestAnUnboundRungIsNotLocal: routeMeta has no entry for an unbound tier, so
// its provider reads empty — and empty must not pass the local test. A rung
// nothing is bound to cannot serve the call, and treating the zero value as
// local would make a config that binds NOTHING look like the safest one.
func TestAnUnboundRungIsNotLocal(t *testing.T) {
	got := localOnlyLadder(TaskCaptureCounterpartyVerdict, map[Tier]routeMeta{}, []Tier{TierLocalSmall})
	if len(got) != 0 {
		t.Fatalf("an unbound rung must not survive the narrowing, got %v", got)
	}
}

// TestEveryLocalOnlyTaskDeclaresANoPayloadPosture: local_only and no_payload
// answer two halves of one question — where the prompt goes, and what it may
// carry. A task that must not leave the machine but may log its payload
// upstream has the guarantee only on one side of the wire.
func TestEveryLocalOnlyTaskDeclaresANoPayloadPosture(t *testing.T) {
	local := LocalOnlyTasks()
	if len(local) == 0 {
		t.Fatal("no task declares local_only — this gate is watching nothing")
	}
	for _, task := range local {
		if !NoPayload(task) {
			t.Errorf("task %s is local_only but not no_payload: the prompt stays on the machine while its content may still be captured", task)
		}
	}
}
