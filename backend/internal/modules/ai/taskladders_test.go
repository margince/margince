// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// reorderedRouting binds every tier to the fake and reorders agent_loop's
// ladder as the Gemini presets do.
const reorderedRouting = `
tiers:
  local_small: {provider: fake}
  cheap_cloud: {provider: fake}
  premium: {provider: fake}
  frontier: {provider: fake}
task_ladders:
  agent_loop: [premium, cheap_cloud]
embeddings: {provider: fake}
profile: eu_hosted
`

// A config's reordering moves the one task it names, and every other task
// keeps its contract order.
func TestATaskLadderOrderMovesOnlyTheTaskItNames(t *testing.T) {
	cfg, err := ParseRouting([]byte(reorderedRouting))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.Ladder(TaskAgentLoop), []Tier{TierPremium, TierCheapCloud}; !slices.Equal(got, want) {
		t.Errorf("agent_loop walks %v, want %v", got, want)
	}
	for _, task := range AllTasks() {
		if task != TaskAgentLoop && !slices.Equal(cfg.Ladder(task), TaskLadder(task)) {
			t.Errorf("%s walks %v, want its contract order %v", task, cfg.Ladder(task), TaskLadder(task))
		}
	}
}

// The router answers the reordered task on the rung the config puts first.
func TestTheRouterWalksTheConfigsOrder(t *testing.T) {
	cfg, err := ParseRouting([]byte(reorderedRouting))
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewLocalRouter(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, info, err := router.Complete(wsContext(t), TaskAgentLoop, model.Request{
		Subjects: model.AboutNoContact("a scheduled run"), Messages: []model.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if info.Tier != TierPremium {
		t.Errorf("agent_loop answered on %s, want premium", info.Tier)
	}
}

// An order naming no task, or other than the task's own rungs, is refused.
func TestATaskLadderOrderMustReorderARealTasksOwnRungs(t *testing.T) {
	for name, order := range map[string]string{
		"an unknown task":    "  no_such_task: [premium, cheap_cloud]",
		"a rung it lacks":    "  agent_loop: [frontier, cheap_cloud]",
		"a rung left out":    "  agent_loop: [premium]",
		"a rung named twice": "  agent_loop: [premium, premium, cheap_cloud]",
	} {
		raw := strings.Replace(reorderedRouting, "  agent_loop: [premium, cheap_cloud]", order, 1)
		if _, err := ParseRouting([]byte(raw)); err == nil || !strings.Contains(err.Error(), "task_ladders") {
			t.Errorf("%s: err = %v, want a task_ladders refusal", name, err)
		}
	}
}

// An installation bound tier for tier to a committed preset takes its task
// order. One bound otherwise, or already ordering its tasks, keeps its own.
func TestAStockBindingAdoptsItsPresetsTaskOrderAndACustomOneDoesNot(t *testing.T) {
	stock, err := ParsePreset([]byte(stockPresets["gemini_cloud.yaml"]))
	if err != nil {
		t.Fatal(err)
	}
	installed := stock
	installed.TaskLadders = nil
	adopted, ok := installed.WithStockTaskLadders()
	premiumFirst := []Tier{TierPremium, TierCheapCloud}
	for _, task := range []Task{TaskAgentLoop, TaskDraftReply} {
		if !ok || !slices.Equal(adopted.Ladder(task), premiumFirst) {
			t.Errorf("a stock Gemini install walks %s %v after adopting (%v), want %v", task, adopted.Ladder(task), ok, premiumFirst)
		}
	}

	vertex, err := ParsePreset([]byte(stockPresets["gemini_vertex_eu.yaml"]))
	if err != nil {
		t.Fatal(err)
	}
	vertex.TaskLadders = nil
	for tier, binding := range vertex.Tiers {
		binding.Location = "europe-west4"
		vertex.Tiers[tier] = binding
	}
	if _, ok := vertex.WithStockTaskLadders(); !ok {
		t.Error("a stock Vertex install at a location of its own did not adopt its preset's order")
	}

	customised := stock
	customised.TaskLadders = nil
	customised.Tiers = maps.Clone(stock.Tiers)
	premium := customised.Tiers[TierPremium]
	premium.Model = "gemini-3.1-flash-lite"
	customised.Tiers[TierPremium] = premium
	if got, ok := customised.WithStockTaskLadders(); ok || len(got.TaskLadders) > 0 {
		t.Errorf("a customised install adopted %v", got.TaskLadders)
	}

	ordered := stock
	ordered.TaskLadders = map[Task][]Tier{TaskAgentLoop: TaskLadder(TaskAgentLoop)}
	got, ok := ordered.WithStockTaskLadders()
	if ok || !slices.Equal(got.Ladder(TaskAgentLoop), TaskLadder(TaskAgentLoop)) || !slices.Equal(got.Ladder(TaskDraftReply), TaskLadder(TaskDraftReply)) {
		t.Errorf("an install ordering one task itself was given %v; it adopts no part of the stock order", got.TaskLadders)
	}
}

// Settings' lead tier and the call flow read the order the router walks, so a
// preset that reorders a task shows the rung its calls are first sent to.
func TestTheLeadTierAndTheCallFlowReadTheRoutersOrder(t *testing.T) {
	stock, err := ParsePreset([]byte(stockPresets["gemini_cloud.yaml"]))
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range FeatureRoutes(stock, stock, BandNormal) {
		if row.Task == string(TaskAgentLoop) && row.LeadingTier != string(TierPremium) {
			t.Errorf("agent_loop leads on %s in Settings, but the router sends it to premium first", row.LeadingTier)
		}
	}
	order := stock.Ladder(TaskAgentLoop)
	steps := []FlowStep{{Tier: TierCheapCloud}, {Tier: TierPremium}, {Decision: true}}
	slices.SortStableFunc(steps, func(a, b FlowStep) int { return routeOrder(order, a) - routeOrder(order, b) })
	if !steps[0].Decision || steps[1].Tier != TierPremium || steps[2].Tier != TierCheapCloud {
		t.Errorf("the call flow reads %+v, want the decision, then premium, then cheap_cloud", steps)
	}
	router := assembleRouter(map[Tier]model.Client{}, NewFakeClient(), ProfileCloudFrontier, &memMeter{}, StaticBudget(100),
		&fakeCallStore{}, map[Tier]routeMeta{}, false, nil)
	router.install(router.binding().withConfig(stock, nil))
	if got := router.TaskOrder(TaskAgentLoop); !slices.Equal(got, order) {
		t.Errorf("the router walks agent_loop %v, want %v", got, order)
	}
}

// A binding two committed presets share, ordering its tasks two ways, adopts
// neither; nor does one whose matching preset orders nothing.
func TestAnAmbiguousOrEmptyStockOrderIsNotAdopted(t *testing.T) {
	stock, err := ParsePreset([]byte(stockPresets["gemini_cloud.yaml"]))
	if err != nil {
		t.Fatal(err)
	}
	installed := stock
	installed.TaskLadders = nil
	original := stockPresets
	t.Cleanup(func() { stockPresets = original })

	twin := strings.Replace(original["gemini_cloud.yaml"], "draft_reply: [premium, cheap_cloud]", "", 1)
	stockPresets = map[string]string{"a.yaml": original["gemini_cloud.yaml"], "b.yaml": twin}
	if got, ok := installed.WithStockTaskLadders(); ok {
		t.Errorf("two presets ordering this binding differently adopted %v", got.TaskLadders)
	}

	unordered := strings.Replace(twin, "agent_loop: [premium, cheap_cloud]", "", 1)
	stockPresets = map[string]string{"a.yaml": unordered}
	if got, ok := installed.WithStockTaskLadders(); ok {
		t.Errorf("a preset ordering nothing was adopted as a change: %v", got.TaskLadders)
	}
}
