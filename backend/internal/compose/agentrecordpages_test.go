// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// A record PAGE is agent-readable; the acts a human takes ON that page are not.
//
// The person and organization halves of the composite read (PO-EXT-3) carry the
// same kind of material — profile, timeline, graph — and for a while they
// answered an agent differently: the person half was annotated human-only, the
// organization half was annotated nothing at all. Neither posture was argued;
// one was written and the other was forgotten, and the pair drifted apart.
//
// They are one class now, and the class is READABLE. What keeps a passport from
// out-seeing its human on these pages is not the annotation but the audience
// gate inside the assembly: auth.ActivityAudienceArm withholds a row's content
// per reader and auth.ActivityContentClause drops a row a reader may not count,
// both keyed on the acting user, and an agent carries the id of the human who
// minted it. Correspondence is the product's one read boundary and it is held
// there, in the SQL, for every principal alike.
//
// The writes keep their refusal for a reason the reads never had: each consumes
// something that belongs to one human's attention. Acknowledging a view moves
// the unread baseline "what changed since I last looked" counts from, and
// dismissing a moment spends it outright — an agent prefetching a record would
// destroy the answer its human opened the page to read. person360's Acknowledge
// says so at its auth.RequireHuman, and sectionstimeline.go's actingUser names
// that gate as the only thing standing between an agent and the write.
//
// Both halves are pinned because both are one line someone can add or delete
// with every other gate, arch and drift test still green.
func TestTheRecordPagesAnswerAnAgentAndTheirWritesDoNot(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/crm.yaml")
	if err != nil {
		t.Fatalf("loading the contract: %v", err)
	}

	// Read as a set rather than a pair: the org half is what the person half
	// was reconciled TO, so a test naming only the person half would pass
	// while the two drifted apart again in the other direction.
	readable := map[string]string{
		"getPerson360":             "/people/{id}/360",
		"getPersonGraph":           "/people/{id}/graph",
		"getOrganization360":       "/organizations/{id}/360",
		"getOrganizationGraph":     "/organizations/{id}/graph",
		"listOrganizationContacts": "/organizations/{id}/contacts",
	}
	humanOnly := map[string]string{
		"acknowledgePersonView":       "/people/{id}/view-ack",
		"dismissPersonMoment":         "/people/{id}/moment/dismiss",
		"acknowledgeOrganizationView": "/organizations/{id}/view-ack",
	}

	byOp := map[string]*openapi3.Operation{}
	routeOf := map[string]string{}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			if op.OperationID == "" {
				continue
			}
			byOp[op.OperationID] = op
			routeOf[op.OperationID] = method + " /v1" + path
		}
	}

	for opID, path := range readable {
		op, ok := byOp[opID]
		if !ok {
			t.Errorf("%s (%s) is not in the contract — the pin covers nothing", opID, path)
			continue
		}
		if access, annotated := op.Extensions["x-agent-access"].(string); annotated {
			t.Errorf("%s is annotated x-agent-access: %s — the record pages are one readable class, "+
				"and the audience gate is what bounds a passport on them, not this line", opID, access)
		}
		// The annotation and the transport override are two locks on one
		// door: removing the first while leaving the second refuses the
		// agent just as completely, one layer further down, where no
		// x-agent-access test would see it.
		if op.Security != nil {
			t.Errorf("%s overrides security to %d scheme(s) — a cookie-only override keeps "+
				"refusing the passport at the transport whatever the annotation says", opID, len(*op.Security))
		}
		if _, gated := agentPolicies[routeOf[opID]]; gated {
			t.Errorf("%s has a policy row — an unannotated read must reach the handler on the "+
				"granting human's RBAC, not be refused by the gate", opID)
		}
	}

	for opID, path := range humanOnly {
		op, ok := byOp[opID]
		if !ok {
			t.Errorf("%s (%s) is not in the contract — the pin covers nothing", opID, path)
			continue
		}
		access, annotated := op.Extensions["x-agent-access"].(string)
		if !annotated || access != "human-only" {
			t.Errorf("%s is annotated %q — it spends one human's unread baseline, "+
				"which an agent reading on their behalf must never consume", opID, access)
			continue
		}
		pol, known := agentPolicies[routeOf[opID]]
		if !known {
			t.Errorf("%s has left the policy table — the refusal is unreachable rather than human-only", opID)
			continue
		}
		if pol.Access != accessHumanOnly {
			t.Errorf("%s: contract says human-only, generated table says %q", opID, pol.Access)
		}
	}
}
