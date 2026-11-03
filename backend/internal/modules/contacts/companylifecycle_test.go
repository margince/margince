// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func writerOfType(kind principal.PrincipalType) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{Type: kind, ID: string(kind) + ":writer"})
}

func TestOnlyASignedInUserSetsAnAccountLive(t *testing.T) {
	if err := checkLifecycleWriter(writerOfType(principal.PrincipalHuman), "live"); err != nil {
		t.Fatalf("a user setting live was refused: %v", err)
	}
	callers := map[string]context.Context{
		"agent":     writerOfType(principal.PrincipalAgent),
		"system":    writerOfType(principal.PrincipalSystem),
		"connector": writerOfType(principal.PrincipalConnector),
		"nobody":    context.Background(),
	}
	for name, ctx := range callers {
		t.Run(name, func(t *testing.T) {
			var refusal *httperr.DetailedError
			err := checkLifecycleWriter(ctx, "live")
			if !errors.As(err, &refusal) || refusal.Status != http.StatusUnprocessableEntity ||
				len(refusal.Fields) != 1 || refusal.Fields[0].Code != "lifecycle_live_needs_a_human" {
				t.Fatalf("want a 422 lifecycle_live_needs_a_human, got %v", err)
			}
		})
	}
}

func TestAnyWriterMaySetEveryStageButLive(t *testing.T) {
	agent := writerOfType(principal.PrincipalAgent)
	for value := range validLifecycles {
		if value == "live" {
			continue
		}
		if err := checkLifecycleWriter(agent, value); err != nil {
			t.Errorf("an agent setting %q was refused: %v", value, err)
		}
	}
	if err := checkLifecycleWriter(writerOfType(principal.PrincipalHuman), "Live"); err == nil {
		t.Error("a value outside the vocabulary was accepted")
	}
}
