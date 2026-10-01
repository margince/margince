// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The §5.2 retry policy as behavior: valid output passes through, an
// invalid first answer retries once WITH the validator's reason, a
// second failure escalates one tier, and total failure is an honest
// error — never a partial fabrication handed to the caller.

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func jsonObjectValidator(text string) error {
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return errors.New("not a JSON object")
	}
	return nil
}

func structuredReq() model.Request {
	return model.Request{Messages: []model.Message{{Role: "user", Content: "extract"}}}
}

func TestStructuredValidFirstTry(t *testing.T) {
	cheap := NewFakeClient().Script(`{"ok":true}`)
	r := testRouter(map[Tier]model.Client{TierCheapCloud: cheap}, &memMeter{}, DefaultMonthlyTokens, ProfileEUHosted)

	resp, _, err := r.CompleteStructured(wsContext(t), TaskColdStart, structuredReq(), jsonObjectValidator)
	if err != nil || resp.Text != `{"ok":true}` {
		t.Fatalf("valid first answer: %v %q", err, resp.Text)
	}
	if calls := cheap.Calls(); len(calls) != 1 {
		t.Fatalf("valid answer cost %d calls, want 1", len(calls))
	}
}

func TestStructuredRetryCarriesValidatorError(t *testing.T) {
	cheap := NewFakeClient().Script("garbage", `{"fixed":true}`)
	r := testRouter(map[Tier]model.Client{TierCheapCloud: cheap}, &memMeter{}, DefaultMonthlyTokens, ProfileEUHosted)

	resp, _, err := r.CompleteStructured(wsContext(t), TaskColdStart, structuredReq(), jsonObjectValidator)
	if err != nil || resp.Text != `{"fixed":true}` {
		t.Fatalf("retry: %v %q", err, resp.Text)
	}
	calls := cheap.Calls()
	if len(calls) != 2 {
		t.Fatalf("retry cost %d calls, want 2", len(calls))
	}
	// The correction turn carries WHY the first answer failed.
	if !strings.Contains(string(calls[1].Payload), "not a JSON object") ||
		!strings.Contains(string(calls[1].Payload), "garbage") {
		t.Fatalf("retry payload lacks the validator feedback: %s", calls[1].Payload)
	}
}

func TestStructuredEscalatesOneTierOnSecondFailure(t *testing.T) {
	cheap := NewFakeClient().Script("bad one", "bad two")
	premium := NewFakeClient().Script(`{"rescued":true}`)
	meter := &memMeter{}
	r := testRouter(map[Tier]model.Client{TierCheapCloud: cheap, TierPremium: premium},
		meter, DefaultMonthlyTokens, ProfileEUHosted)

	resp, info, err := r.CompleteStructured(wsContext(t), TaskColdStart, structuredReq(), jsonObjectValidator)
	if err != nil || resp.Text != `{"rescued":true}` {
		t.Fatalf("escalation: %v %q", err, resp.Text)
	}
	if info.Tier != TierPremium {
		t.Fatalf("third attempt served from %s, want the escalated tier", info.Tier)
	}
	// All three attempts hit the meter — retries are budgeted spend.
	if len(meter.records) != 3 {
		t.Fatalf("metered %d calls, want 3", len(meter.records))
	}
}

func TestStructuredExhaustionIsAnHonestError(t *testing.T) {
	cheap := NewFakeClient().Script("bad", "bad", "bad")
	r := testRouter(map[Tier]model.Client{TierCheapCloud: cheap}, &memMeter{}, DefaultMonthlyTokens, ProfileEUHosted)

	_, _, err := r.CompleteStructured(wsContext(t), TaskColdStart, structuredReq(), jsonObjectValidator)
	// The SENTINEL, not the wording: a caller with a watermark or a queue has
	// to tell "this input will never validate" from "the provider had a bad
	// minute", and only one of those is worth retrying. Asserting the message
	// text let the classification change without the test noticing.
	if !errors.Is(err, ErrOutputRejected) {
		t.Fatalf("exhaustion → %v, want it to wrap ErrOutputRejected", err)
	}
	// Still legible to a human reading a log: the task and the validator's own
	// complaint survive the wrap.
	if !strings.Contains(err.Error(), "after retry and escalation") ||
		!strings.Contains(err.Error(), "not a JSON object") {
		t.Errorf("exhaustion error %q drops the task or the validator's reason", err)
	}
}

// An installation with no vendor bound runs on the offline stand-in, whose
// answer is a hash string no structured task can accept. The rejection is
// terminal like any other, but its CAUSE is a setting rather than the model or
// the input — so the router marks it, and a caller can say which of the two it
// is instead of telling an operator to retry a thing that cannot succeed.
//
// Built through the real FakeRoutingConfig rather than a hand-supplied route
// map, because what carries the provider name into RouteInfo is the config the
// --ai-fake path actually installs.
func TestStructuredMarksRejectionFromTheUnconfiguredStandIn(t *testing.T) {
	r, err := NewLocalRouter(FakeRoutingConfig(), WithoutResultCache())
	if err != nil {
		t.Fatalf("building the offline router: %v", err)
	}

	_, info, err := r.CompleteStructured(wsContext(t), TaskColdStart, structuredReq(), jsonObjectValidator)
	if err == nil {
		t.Fatal("the stand-in cannot satisfy a structured validator, so this must fail")
	}
	if info.Provider != ProviderFake {
		t.Fatalf("the route says what served it; got %q", info.Provider)
	}
	// Both sentinels: every caller that classifies a terminal rejection keeps
	// classifying this one, and the one that wants the cause can now ask.
	if !errors.Is(err, ErrOutputRejected) {
		t.Fatalf("still a terminal rejection: %v", err)
	}
	if !errors.Is(err, ErrUnconfiguredModel) {
		t.Fatalf("a rejection served by the stand-in is marked as one: %v", err)
	}
}

// The counter-case, without which the marker above could be unconditional: a
// REAL bound model that answers badly is not an unconfigured installation, and
// sending its operator to a settings screen wastes the one message they get.
func TestStructuredLeavesARealModelsBadAnswerUnmarked(t *testing.T) {
	cheap := NewFakeClient().Script("garbage", "still garbage", "garbage again")
	r := assembleRouter(
		map[Tier]model.Client{TierCheapCloud: cheap, TierPremium: cheap},
		NewFakeClient(), ProfileEUHosted, &memMeter{}, DefaultMonthlyTokens, nil,
		map[Tier]routeMeta{
			TierCheapCloud: {provider: "anthropic", model: "claude-x"},
			TierPremium:    {provider: "anthropic", model: "claude-y"},
		}, false, nil,
	)

	_, _, err := r.CompleteStructured(wsContext(t), TaskColdStart, structuredReq(), jsonObjectValidator)
	if !errors.Is(err, ErrOutputRejected) {
		t.Fatalf("three bad answers are a terminal rejection: %v", err)
	}
	if errors.Is(err, ErrUnconfiguredModel) {
		t.Fatalf("a bound vendor that answered badly is not an unconfigured installation: %v", err)
	}
}
