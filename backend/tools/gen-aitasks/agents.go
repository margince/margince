// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The tools an agent_loop site attaches, and the ways that declaration can be
// wrong. agent_loop is the engine; each of its sites is one scheduled agent,
// and the site's tools are the only tools that run is ever offered.

import (
	"fmt"
	"sort"
)

// validateSiteTools holds every site's tools to the things a runtime cannot
// recover from on its own: an agent_loop site with no allowlist, the same tool
// attached twice, and a tool list on a site that assembles no tool-fed window.
//
// An empty allowlist is refused rather than defaulted because the runner
// refuses a job carrying none, and a build defect discovered at boot is the
// shape ADR-0074 asks every field in this contract to avoid.
//
// The tool NAMES, and whether a list covers the whole catalog, are checked at
// composition time: this generator has no registry to check them against, and
// splitting the check would mean two half-answers to one question.
func validateSiteTools(task string, def taskDef) error {
	for _, site := range def.Sites {
		if site.Kind != kindAgentLoop {
			if len(site.Tools) > 0 {
				return fmt.Errorf(
					"task %q: site %q attaches tools but is %s — only an agent_loop site has a tool listing to attach "+
						"them to, so this allowlist would never be assembled", task, site.Name, site.Kind)
			}
			continue
		}
		if len(site.Tools) == 0 {
			return fmt.Errorf(
				"task %q: agent_loop site %q declares no tools — every run attaches the tools its goal needs, "+
					"and the runner refuses a job with none", task, site.Name)
		}
		seen := make(map[string]bool, len(site.Tools))
		for _, tool := range site.Tools {
			if seen[tool] {
				return fmt.Errorf("task %q: site %q attaches %q twice", task, site.Name, tool)
			}
			seen[tool] = true
		}
	}
	return nil
}

// agentLoopSites is a task's agent_loop sites in name order — the one place
// that order is decided, for the reason sortedTaskNames exists: a generated
// file that moves between runs cannot be drift-gated.
func agentLoopSites(def taskDef) []siteDef {
	var sites []siteDef
	for _, site := range def.Sites {
		if site.Kind == kindAgentLoop {
			sites = append(sites, site)
		}
	}
	sort.Slice(sites, func(i, j int) bool { return sites[i].Name < sites[j].Name })
	return sites
}
