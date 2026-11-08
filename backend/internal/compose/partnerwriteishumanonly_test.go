// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// `margin_tier` is the rate the commission ledger multiplies a won deal by, so it
// decides what a partner is paid on every future deal. `decideCommissionEntry`
// is human-only for the same reason. The rest of a partner's terms are open.

import (
	"reflect"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

const upsertPartnerRoute = "PUT /v1/companies/{id}/partner"

// An agent may set up a partner's program, on the route an admin uses. The margin
// tier stays a user's: the store refuses it, and an integration test proves that.
func TestSettingPartnerTermsIsOpenOnItsRoute(t *testing.T) {
	policy, declared := agentPolicies[upsertPartnerRoute]
	if !declared {
		t.Fatalf("%s has no agent policy — every mutating operation declares one, so this route "+
			"was renamed or removed and this test needs to follow it", upsertPartnerRoute)
	}
	if policy.Access != accessTool || policy.ServedBy != servedByRoute {
		t.Errorf("%s admits an agent as %q served by %q, want a route-backed tool", upsertPartnerRoute, policy.Access, policy.ServedBy)
	}
}

// The read side is open too, on its own tool.
func TestReadingAPartnerStaysOpenToAgents(t *testing.T) {
	policy, declared := agentPolicies["GET /v1/companies/{id}/partner"]
	if !declared {
		t.Fatal("GET /v1/companies/{id}/partner has no agent policy")
	}
	if policy.Access != accessTool {
		t.Errorf("reading a partner is %q, want %q",
			policy.Access, accessTool)
	}
	if policy.Tool != "read_record" {
		t.Errorf("reading a partner maps to tool %q, want read_record", policy.Tool)
	}
}

// The partner write an agent may stage for a user's yes starts a program and
// carries no tier. The tool's argument list and the request type generated from the
// contract both leave out everything commercial.
func TestTheAgentPartnerSetUpCannotCarryATier(t *testing.T) {
	spec, registered := NewRegistry(nil, SendPath{}).Spec("set_up_partner")
	if !registered {
		t.Fatal("set_up_partner is not registered")
	}
	for _, field := range []string{"margin_tier", "cert_status", "gate_metrics", "served_segments"} {
		if strings.Contains(string(spec.InputSchema), `"`+field+`"`) {
			t.Errorf("set_up_partner takes %q; a program set up by an agent starts with none of the commercial terms", field)
		}
		if reflectHasJSONField(reflect.TypeFor[crmcontracts.SetUpPartnerRequest](), field) {
			t.Errorf("SetUpPartnerRequest declares %q", field)
		}
	}
	policy := agentPolicies["POST /v1/companies/{id}/partner/setup"]
	if policy.Tier != tierConfirmationRequired || policy.Tool != "set_up_partner" {
		t.Errorf("setUpPartner policy = tool %q tier %q, want set_up_partner at %q", policy.Tool, policy.Tier, tierConfirmationRequired)
	}
}

func reflectHasJSONField(t reflect.Type, name string) bool {
	for field := range t.Fields() {
		if strings.Split(field.Tag.Get("json"), ",")[0] == name {
			return true
		}
	}
	return false
}
