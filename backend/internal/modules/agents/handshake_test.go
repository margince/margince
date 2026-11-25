// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"regexp"
	"strings"
	"testing"
)

// A client may list a tool as mcp__<server>__find_records while every text of
// this server says find_records. The instructions say so before they first
// name a tool, so the model reads the rule before the name it would copy.
func TestTheInstructionsSayAClientMayPrefixAToolBeforeTheyNameOne(t *testing.T) {
	d := NewDispatcher(fullRegistry(t), bindAuthenticated, "margince-crm", "test").WithLogger(discardLog())
	served := d.instructions()
	rule := strings.Index(served, toolNamesRule)
	if rule < 0 {
		t.Fatalf("the instructions do not say a client may list a tool under its own prefix:\n%s", served)
	}
	first, firstName := len(served), ""
	for _, spec := range d.registry.Specs() {
		at := regexp.MustCompile(`\b` + regexp.QuoteMeta(spec.Name) + `\b`).FindStringIndex(served)
		if at != nil && at[0] < first {
			first, firstName = at[0], spec.Name
		}
	}
	if firstName == "" {
		t.Fatalf("the instructions name no served tool, so the order checked here is empty:\n%s", served)
	}
	if first < rule {
		t.Errorf("the instructions name %s before they say a client may prefix it:\n%s", firstName, served)
	}
}

// The rule holds for a composition that serves no tool the instructions name,
// because tool descriptions and answers name tools bare all the same.
func TestTheToolNamesRuleIsServedWhereNoStartingToolIs(t *testing.T) {
	d := NewDispatcher(NewRegistry(nil, nil), bindAuthenticated, "margince-crm", "test").WithLogger(discardLog())
	if !strings.Contains(d.instructions(), toolNamesRule) {
		t.Errorf("an empty surface drops the tool-names rule:\n%s", d.instructions())
	}
}
