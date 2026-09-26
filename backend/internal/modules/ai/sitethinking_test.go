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

// The contract's per-site level reaches the wire of the site that declares it
// and of no other, and its place in the precedence is between the request's
// own level and the binding's.
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
		"the site outranks the binding":             {TaskColdStart, "sitereadmessage", "high", nil, `"thinkingLevel":"low"`},
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

// Only a Gemini 3 rung is sent the level; the other gemini options a request
// carries survive the merge, and the caller's map is left as it was.
func TestASiteThinkingLevelIsSentOnlyWhereItCanBeCarried(t *testing.T) {
	signatures := map[string]json.RawMessage{providerGemini: json.RawMessage(`{"thought_signatures":["s"]}`)}
	req := model.Request{Site: "sitereadmessage", ProviderOptions: signatures}
	for name, lane := range map[string]routeMeta{
		"an OpenAI-compatible rung": {provider: providerOpenAICompatible, model: "m"},
		"an Ollama rung":            {provider: providerOllama, model: "gemma4"},
		"a Gemini 2.5 rung":         {provider: providerGemini, model: "gemini-2.5-flash"},
	} {
		got, err := withSiteThinking(req, TaskColdStart, lane)
		if err != nil || string(got.ProviderOptions[providerGemini]) != `{"thought_signatures":["s"]}` {
			t.Errorf("%s: options = %s (err %v), want them untouched", name, got.ProviderOptions[providerGemini], err)
		}
	}
	got, err := withSiteThinking(req, TaskColdStart, routeMeta{provider: providerGemini, model: siteThinkingFlashLite})
	if err != nil {
		t.Fatal(err)
	}
	opts, err := geminiReadOptions(got.ProviderOptions)
	if err != nil || opts.ThinkingLevel != "low" || !slices.Equal(opts.ThoughtSignatures, []string{"s"}) {
		t.Errorf("merged options = %+v (err %v), want low beside the request's own signatures", opts, err)
	}
	if string(signatures[providerGemini]) != `{"thought_signatures":["s"]}` {
		t.Errorf("the caller's options map was written to: %s", signatures[providerGemini])
	}
}

// The certification record and the preset view read the same precedence: the
// sites a binding serves off its own level, and none where it is not sent.
func TestSiteThinkingLevelsNamesTheSitesServedOffTheBindingsLevel(t *testing.T) {
	flashLite := ProviderConfig{Provider: providerGemini, Model: siteThinkingFlashLite}
	want := map[string]string{"company_message": "low", "sitereadmessage": "low"}
	if got := SiteThinkingLevels(flashLite, TaskColdStart); !maps.Equal(got, want) {
		t.Errorf("at the default: %v, want %v", got, want)
	}
	flashLite.ThinkingLevel = "low"
	if got := SiteThinkingLevels(flashLite, TaskColdStart); got != nil {
		t.Errorf("a binding already at low serves every site at its own level, got %v", got)
	}
	if got := SiteThinkingLevels(ProviderConfig{Provider: providerOpenAICompatible, Model: "m"}, TaskColdStart); got != nil {
		t.Errorf("a rung never sent the level reports %v", got)
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
