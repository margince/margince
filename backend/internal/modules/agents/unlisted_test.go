// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"slices"
	"testing"
)

// advance_deal and relink_thread stay callable by name and are not offered.
// progress_deal takes every argument advance_deal takes and stages the same
// command, and relink_thread refuses every call over MCP in favour of
// relink_activities.
var superseded = []struct{ tool, by string }{
	{"advance_deal", "progress_deal"},
	{"relink_thread", "relink_activities"},
}

func TestASupersededToolIsNotOffered(t *testing.T) {
	var offered []string
	for _, spec := range fullRegistry(t).Offered(agentCtx()) {
		offered = append(offered, spec.Name)
	}
	for _, pair := range superseded {
		if slices.Contains(offered, pair.tool) || !slices.Contains(offered, pair.by) {
			t.Errorf("offered %v: want %s in place of %s", offered, pair.by, pair.tool)
		}
	}
}

// A tool nobody is offered is a capability no client can find. Every unlisted
// tool is therefore either fronted by a router or superseded by a named one.
func TestEveryUnlistedToolIsReachedSomeOtherWay(t *testing.T) {
	r := fullRegistry(t)
	fronted := map[string]bool{}
	for _, spec := range r.Specs() {
		targets, _ := r.RouteTargets(spec.Name)
		for _, target := range targets {
			fronted[target] = true
		}
	}
	unlisted := 0
	for _, spec := range r.Specs() {
		if !spec.Unlisted {
			continue
		}
		unlisted++
		replaced := slices.ContainsFunc(superseded, func(pair struct{ tool, by string }) bool { return pair.tool == spec.Name })
		if !fronted[spec.Name] && !replaced {
			t.Errorf("%s is unlisted, and no router fronts it and no listed tool supersedes it", spec.Name)
		}
	}
	if unlisted == 0 {
		t.Fatal("no unlisted tool was found, so this census checked nothing")
	}
}
