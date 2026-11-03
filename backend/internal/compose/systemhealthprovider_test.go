// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type healthProviderStore struct {
	statuses map[string]model.ProviderHealthStatus
}

func (s healthProviderStore) Load(context.Context) (map[string]model.ProviderHealthStatus, error) {
	return s.statuses, nil
}

func (s healthProviderStore) Publish(context.Context, string, model.ProviderHealthStatus) error {
	return nil
}

func (s healthProviderStore) Clear(context.Context, string) error { return nil }

func TestProviderSummaryRequiresBlockedCallsBeforeAskingAnAdministratorToAct(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(func() {
		cancel()
		ai.ShareProviderHealth(ctx, nil)
	})
	blockedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &healthProviderStore{statuses: map[string]model.ProviderHealthStatus{
		"openai": {Health: model.HealthDegraded, Since: blockedAt.Add(-time.Hour)},
	}}
	ai.ShareProviderHealth(ctx, store)
	handlers := ai.NewHandlers(nil, nil).WithBoundModels(func() map[ai.Tier]ai.ModelRef {
		return map[ai.Tier]ai.ModelRef{"fast": {Provider: "openai"}, "reasoning": {Provider: "anthropic"}}
	})
	server := Server{voiceHandlers: handlers}
	reader := healthTestContext(map[string]principal.ObjectGrant{"ai_diagnostics": {Read: true}})
	source, findings, err := server.providersSummary(reader)
	if err != nil || len(findings) != 0 || source.Code != "providers_unobserved" {
		t.Fatalf("degraded calls must not raise a blocked-provider alert: source=%+v findings=%+v err=%v", source, findings, err)
	}
	ai.ShareProviderHealth(ctx, healthProviderStore{statuses: map[string]model.ProviderHealthStatus{
		"openai":    store.statuses["openai"],
		"anthropic": {Health: model.HealthUnauthorized, Since: blockedAt},
	}})
	_, findings, err = server.providersSummary(reader)
	if err != nil || len(findings) != 1 {
		t.Fatalf("blocked provider alert missing: %+v err=%v", findings, err)
	}
	if !findings[0].RequiresAction || findings[0].Responsible != "administrator" || findings[0].FirstObservedAt == nil || !findings[0].FirstObservedAt.Equal(blockedAt) {
		t.Fatalf("blocked-provider evidence includes an unrelated degradation: %+v", findings[0])
	}
}
