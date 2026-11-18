// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// A role that boots before any binding exists must pick up the one an admin
// saves later. Its router refuses calls first and serves them after Rebind.
func TestAnUnboundRouterServesOnceItIsRebound(t *testing.T) {
	r := NewUnboundRouter(nil, DefaultMonthlyTokens, nil, false, nil)
	r.meter = stubMeter{}
	if got := r.RoutingVersion(); got != "" {
		t.Fatalf("RoutingVersion of an unbound router = %q, want empty", got)
	}
	if got := r.ConfiguredProfile().State; got != "unconfigured" {
		t.Fatalf("profile state before a binding = %q, want unconfigured", got)
	}
	if _, ok := r.CurrentModelForTier(TierPremium); ok {
		t.Fatal("an unbound router reports a model for the premium tier")
	}
	req := model.Request{Messages: []model.Message{{Role: "user", Content: "hello"}}}
	_, _, err := r.Complete(wsContext(t), TaskRateExtract, req)
	if fault, ok := httperr.Classify(err); !ok || fault.Status != http.StatusNotImplemented || fault.Code != "not_implemented" {
		t.Fatalf("a call on an unbound router answered %v (%+v), want the 501 an unwired surface gives", err, fault)
	}
	if _, err := r.Embed(wsContext(t), model.EmbedRequest{}); err == nil {
		t.Fatal("an embed on an unbound router succeeded, want a refusal")
	}

	if err := r.Rebind(parsed(t, rebindFrom)); err != nil {
		t.Fatalf("Rebind: %v", err)
	}
	if got := r.RoutingVersion(); got == "" {
		t.Error("RoutingVersion is still empty after the rebind")
	}
	if got := r.ConfiguredProfile().State; got != "configured" {
		t.Errorf("profile state after the rebind = %q, want configured", got)
	}
	if _, _, err := r.Complete(wsContext(t), TaskRateExtract, req); err != nil {
		t.Errorf("a call after the rebind failed: %v", err)
	}
}
