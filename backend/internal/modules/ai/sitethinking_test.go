// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bytes"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const siteThinkingFlashLite = "gemini-3.1-flash-lite"

// siteThinkingRouter routes every rung to one real gemini adapter bound at
// bindingLevel, and returns the bodies it was sent.
func siteThinkingRouter(t *testing.T, bindingLevel string) (*Router, *[][]byte) {
	t.Helper()
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodies = append(bodies, readBody(t, r.Body))
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{}"}]},"finishReason":"STOP"}]}`))
	}))
	t.Cleanup(srv.Close)
	binding := ProviderConfig{Provider: providerGemini, Model: siteThinkingFlashLite, BaseURL: srv.URL, ThinkingLevel: bindingLevel}
	client, err := selectLocalBrain(binding, cloudKeyFor(providerGemini, testGeminiKey))
	if err != nil {
		t.Fatal(err)
	}
	lane := routeMeta{provider: providerGemini, model: siteThinkingFlashLite}
	r := assembleRouter(map[Tier]model.Client{TierCheapCloud: client, TierPremium: client},
		NewFakeClient(), ProfileCloudFrontier, &memMeter{}, DefaultMonthlyTokens, nil,
		map[Tier]routeMeta{TierCheapCloud: lane, TierPremium: lane}, false, nil)
	r.cacheOff = true
	return r, &bodies
}

// The contract's per-site floor reaches the wire of the site that declares it
// and of no other, and ranks below both the request's own level and the
// binding's.
func TestASiteThinkingLevelReachesOnlyItsOwnSitesRequests(t *testing.T) {
	medium := map[string]json.RawMessage{providerGemini: json.RawMessage(`{"thinking_level":"medium"}`)}
	for name, tc := range map[string]struct {
		task         Task
		site         string
		bindingLevel string
		opts         map[string]json.RawMessage
		want         string
	}{
		"a declaring site is sent its level":        {TaskColdStart, "sitereadmessage", "", nil, `"thinkingLevel":"low"`},
		"the other company conversation too":        {TaskColdStart, "company_message", "", nil, `"thinkingLevel":"low"`},
		"a sibling site keeps the binding's":        {TaskColdStart, "acts", "", nil, ""},
		"another task's site is sent none":          {TaskDraftReply, "reply", "", nil, ""},
		"a request naming no site is sent none":     {TaskColdStart, "", "", nil, ""},
		"the binding outranks the site":             {TaskColdStart, "sitereadmessage", "minimal", nil, `"thinkingLevel":"minimal"`},
		"a site declaring none keeps the binding's": {TaskDraftReply, "reply", "high", nil, `"thinkingLevel":"high"`},
		"the request outranks the site":             {TaskColdStart, "sitereadmessage", "", medium, `"thinkingLevel":"medium"`},
	} {
		t.Run(name, func(t *testing.T) {
			r, bodies := siteThinkingRouter(t, tc.bindingLevel)
			req := structuredAsk(tc.opts)
			req.Site = tc.site
			if _, _, err := r.Complete(wsContext(t), tc.task, req); err != nil {
				t.Fatal(err)
			}
			body := (*bodies)[0]
			if tc.want == "" {
				if bytes.Contains(body, []byte(`"thinkingConfig"`)) {
					t.Fatalf("a request whose site declares no level was sent one: %s", body)
				}
				return
			}
			if !bytes.Contains(body, []byte(tc.want)) {
				t.Fatalf("wire lacks %s: %s", tc.want, body)
			}
		})
	}
}

// A site the task never declares is refused before any provider is asked: the
// request was built for a prompt the contract does not know.
func TestARequestNamingAnUndeclaredSiteIsRefused(t *testing.T) {
	r, bodies := siteThinkingRouter(t, "")
	req := structuredAsk(nil)
	req.Site = "sitereadmesage"
	if _, _, err := r.Complete(wsContext(t), TaskColdStart, req); err == nil {
		t.Fatal("a request naming an undeclared site was served")
	}
	if len(*bodies) != 0 {
		t.Fatalf("a request naming an undeclared site reached the provider %d time(s)", len(*bodies))
	}
}

// The router hands the site's floor to every rung on the request itself, and
// leaves the caller's provider options as they were.
func TestTheSiteFloorRidesTheRequestAndLeavesItsOptionsAlone(t *testing.T) {
	signatures := map[string]json.RawMessage{providerGemini: json.RawMessage(`{"thought_signatures":["s"]}`)}
	got, err := withSiteThinking(model.Request{Site: "sitereadmessage", ProviderOptions: signatures}, TaskColdStart)
	if err != nil || got.ThinkingFloor != "low" {
		t.Fatalf("floor = %q (err %v), want low", got.ThinkingFloor, err)
	}
	if string(got.ProviderOptions[providerGemini]) != `{"thought_signatures":["s"]}` {
		t.Errorf("options = %s, want them untouched", got.ProviderOptions)
	}
	kept, err := withSiteThinking(model.Request{Site: "acts", ThinkingFloor: "high"}, TaskColdStart)
	if err != nil || kept.ThinkingFloor != "high" {
		t.Errorf("a site declaring none replaced the caller's floor: %q (err %v)", kept.ThinkingFloor, err)
	}
}

// On Gemini the floor raises only a default shallower than itself: Flash-Lite
// (minimal) is raised to low, a deeper model is sent nothing, and a model
// before Gemini 3, which takes no thinkingLevel, is sent nothing either.
func TestAGeminiFloorRaisesOnlyAShallowerDefault(t *testing.T) {
	for name, tc := range map[string]struct {
		model, level, floor, want string
	}{
		"flash-lite's minimal is raised":        {siteThinkingFlashLite, "", "low", "low"},
		"pro at its own default is left alone":  {"gemini-3.1-pro-preview", "", "low", ""},
		"the structured default already meets":  {"gemini-3.5-flash", "low", "low", "low"},
		"the structured default is raised":      {"gemini-3.5-flash", "low", "high", "high"},
		"a Gemini 2.5 is never named a level":   {"gemini-2.5-flash", "", "low", ""},
		"no floor leaves the adapter's default": {siteThinkingFlashLite, "", "", ""},
	} {
		if got := geminiRaisedToFloor(tc.model, tc.level, tc.floor); got != tc.want {
			t.Errorf("%s: level = %q, want %q", name, got, tc.want)
		}
	}
}

// The certification record and the preset view read the same precedence: the
// floor each site asks of a binding that takes one, and none where the binding
// names its own level or its adapter maps no floor.
func TestSiteThinkingLevelsNamesTheSitesServedOffTheBindingsLevel(t *testing.T) {
	flashLite := ProviderConfig{Provider: providerGemini, Model: siteThinkingFlashLite}
	want := map[string]string{"company_message": "low", "sitereadmessage": "low"}
	if got := SiteThinkingLevels(flashLite, TaskColdStart); !maps.Equal(got, want) {
		t.Errorf("at the default: %v, want %v", got, want)
	}
	flashLite.ThinkingLevel = "minimal"
	if got := SiteThinkingLevels(flashLite, TaskColdStart); got != nil {
		t.Errorf("a binding naming its own level outranks every site's floor, got %v", got)
	}
	broker := ProviderConfig{Provider: providerOpenAICompatible, Model: "openai/gpt-oss-120b", BaseURL: "https://openrouter.ai/api"}
	if got := SiteThinkingLevels(broker, TaskColdStart); !maps.Equal(got, want) {
		t.Errorf("a broker rung asks the floor, got %v, want %v", got, want)
	}
	broker.Routing = &OpenRouterRouting{ReasoningEffort: "none"}
	if got := SiteThinkingLevels(broker, TaskColdStart); got != nil {
		t.Errorf("a broker rung whose binding sets its own effort reports %v", got)
	}
	for _, never := range []ProviderConfig{
		{Provider: providerOpenAICompatible, Model: "m", BaseURL: "https://api.mistral.ai"},
		{Provider: providerVLLM, Model: "mlx-community/Qwen3-14B-4bit"},
		{Provider: providerOpenAI, Model: "gpt-4.1"},
	} {
		if got := SiteThinkingLevels(never, TaskColdStart); got != nil {
			t.Errorf("%s/%s is never sent a floor and reports %v", never.Provider, never.Model, got)
		}
	}
}

// Every declared level is one the Gemini adapter accepts: the contract has no
// vocabulary of its own, so this is what keeps it from growing one.
func TestEverySiteThinkingLevelIsAGeminiThinkingLevel(t *testing.T) {
	declared := 0
	for _, task := range AllTasks() {
		for _, site := range SitesFor(task) {
			if site.Thinking == "" {
				continue
			}
			declared++
			if !slices.Contains(geminiThinkingLevels, site.Thinking) {
				t.Errorf("%s/%s declares thinking %q, not one of %v", task, site.Name, site.Thinking, geminiThinkingLevels)
			}
		}
	}
	if declared == 0 {
		t.Fatal("no site declares a thinking level, so this check read nothing")
	}
}
