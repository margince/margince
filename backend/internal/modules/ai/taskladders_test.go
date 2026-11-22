// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
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

// An order naming no task, or not exactly the task's own rungs, is refused.
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
