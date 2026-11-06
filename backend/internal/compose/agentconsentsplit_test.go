// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
)

// A contact's consent routes split by weight. An agent reads the state and its
// proof, because it must check before it records. The acts with legal weight ask
// the user first, and the mails spend send. Defining a purpose stays human-only.
func TestConsentRoutesSplitByWhoseDecisionTheyHold(t *testing.T) {
	const contactConsent = "/v1/contacts/{id}/consent"
	type weight struct {
		scope     agentScope
		asksFirst bool
	}
	writes := map[string]weight{
		"recordConsent": {scopeWrite, true}, "allowContact": {scopeWrite, true}, "revokeOverride": {scopeWrite, true},
		"liftSuppression": {scopeWrite, true}, "recordQualifyingEvent": {scopeWrite, true},
		"retractQualifyingEvent": {scopeWrite, true},
		"issueDoubleOptIn":       {scopeSend, true}, "sendPrivacyNotice": {scopeSend, true}, "requestDetailsConfirmation": {scopeSend, true},
		// A contact asking to stop has legal weight. The marker asks the model; the server still records it at once.
		"suppressContact": {scopeWrite, true},
	}
	verbs := map[string]agents.RouteVerb{}
	for _, v := range RouteVerbs() {
		verbs[v.Verb] = v
	}
	seen := 0
	for route, pol := range agentPolicies {
		method, pattern, _ := strings.Cut(route, " ")
		if !strings.HasPrefix(pattern, contactConsent) {
			continue
		}
		seen++
		if method == http.MethodGet {
			if pol.Access != accessTool || pol.ServedBy != servedByRoute || pol.Scope != scopeRead {
				t.Errorf("%s (%s) reads a contact's consent, so a read scope reaches it; got %+v", route, pol.Op, pol)
			}
			continue
		}
		if pol.Access != accessTool || pol.ServedBy != servedByRoute {
			t.Errorf("%s (%s) must be a route-backed action of the user's agent, got %+v", route, pol.Op, pol)
			continue
		}
		want, listed := writes[pol.Op]
		if !listed {
			t.Errorf("%s (%s) writes a contact's consent and this test does not say what it spends", route, pol.Op)
			continue
		}
		if verbs[pol.Tool].AskUserFirst != want.asksFirst || pol.Scope != want.scope {
			t.Errorf("%s (%s) must have ask=%v and spend %s, got ask=%v scope=%s",
				route, pol.Op, want.asksFirst, want.scope, verbs[pol.Tool].AskUserFirst, pol.Scope)
		}
		delete(writes, pol.Op)
	}
	for op := range writes {
		t.Errorf("%s left the per-contact consent routes: this pin no longer covers it", op)
	}
	if seen == 0 {
		t.Fatal("no per-contact consent route in the policy table: the pin no longer covers anything")
	}

	define, known := agentPolicies[http.MethodPost+" /v1/consent-purposes"]
	if !known || define.Access != accessHumanOnly {
		t.Errorf("defining a purpose is a human decision, got %+v (known %v)", define, known)
	}

	catalog, known := agentPolicies[http.MethodGet+" /v1/consent-purposes"]
	if !known {
		t.Fatal("the purposes catalog left the policy table")
	}
	if catalog.Access != accessTool || catalog.Tool != agents.ReadViewTool || catalog.Scope != scopeRead {
		t.Errorf("the purposes catalog is configuration an agent reads through %s with %s scope, got %+v",
			agents.ReadViewTool, scopeRead, catalog)
	}
}

// Directing a refused send is the user's decision, carried out by their agent:
// it spends send and asks first.
func TestDirectingARefusedSendAsksFirstAndSpendsSend(t *testing.T) {
	pol, known := agentPolicies[http.MethodPost+" /v1/communication-reviews/{id}/direct-send"]
	if !known || pol.Access != accessTool || pol.Scope != scopeSend {
		t.Fatalf("the directed send must be an agent's action on send, got %+v (known %v)", pol, known)
	}
	for _, v := range RouteVerbs() {
		if v.Verb == pol.Tool && v.AskUserFirst {
			return
		}
	}
	t.Errorf("%s does not ask the user first", pol.Tool)
}

// These reach something a user could not safely hand over: they destroy data, take
// it all out, or set what a whole company runs on.
func TestTheActsSecurityKeepsStayClosed(t *testing.T) {
	closed := map[string]bool{
		"purgeCaptureExclusion": false, "deleteCorpusDocument": false, "createFilteredExport": false,
		"updateCaptureSettings": false, "pauseReportingSchedules": false,
	}
	for route, pol := range agentPolicies {
		if _, named := closed[pol.Op]; !named {
			continue
		}
		closed[pol.Op] = true
		if pol.Access != accessHumanOnly || pol.HumanOnlyReason != humanOnlySecurity {
			t.Errorf("%s (%s) must stay human-only for security, got %+v", route, pol.Op, pol)
		}
	}
	for op, seen := range closed {
		if !seen {
			t.Errorf("%s left the policy table: this pin no longer covers it", op)
		}
	}
}
