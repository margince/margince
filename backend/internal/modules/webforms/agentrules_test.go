// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func actorCtx(kind principal.PrincipalType) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{Type: kind})
}

func refusedCode(t *testing.T, err error) string {
	t.Helper()
	var detailed *httperr.DetailedError
	if !errors.As(err, &detailed) || detailed.Status != http.StatusForbidden {
		t.Fatalf("got %v, want a 403", err)
	}
	return detailed.Code
}

func TestAnAgentNeverSeesTheSecretAFormMinted(t *testing.T) {
	if got := secretForCaller(actorCtx(principal.PrincipalAgent), "mwf_minted"); got != "" {
		t.Errorf("an agent was answered %q", got)
	}
	if got := secretForCaller(actorCtx(principal.PrincipalHuman), "mwf_minted"); got != "mwf_minted" {
		t.Errorf("a human was answered %q, want the minted secret", got)
	}
}

func TestAnAgentSetsNoSecretOnANewForm(t *testing.T) {
	if code := refusedCode(t, refuseAgentSecret(actorCtx(principal.PrincipalAgent), true)); code != "agent_sets_no_form_secret" {
		t.Errorf("code = %q", code)
	}
	if err := refuseAgentSecret(actorCtx(principal.PrincipalAgent), false); err != nil {
		t.Errorf("an agent setting no secret was refused: %v", err)
	}
	if err := refuseAgentSecret(actorCtx(principal.PrincipalHuman), true); err != nil {
		t.Errorf("a human pasting a builder's secret was refused: %v", err)
	}
}

func TestAnAgentMakesNoFormThatPromisesAVisitorAnything(t *testing.T) {
	wording := "I agree to be contacted"
	for name, in := range map[string]CreateInput{
		"websites": {Sender: SenderBrowser, AllowedOrigins: []string{"https://www.example.com"}},
		"consent":  {Sender: SenderServer, ConsentRequired: true, ConsentWording: &wording},
		"notice":   {Sender: SenderServer, PrivacyNotice: &wording},
	} {
		err := refuseAgentPromiseChange(actorCtx(principal.PrincipalAgent), Form{}, in.promises())
		if code := refusedCode(t, err); code != "agent_sets_no_form_promise" {
			t.Errorf("%s: code = %q", name, code)
		}
		if err := refuseAgentPromiseChange(actorCtx(principal.PrincipalHuman), Form{}, in.promises()); err != nil {
			t.Errorf("%s: a human was refused: %v", name, err)
		}
	}
	plain := CreateInput{Sender: SenderServer, AllowedOrigins: []string{}}
	if err := refuseAgentPromiseChange(actorCtx(principal.PrincipalAgent), Form{}, plain.promises()); err != nil {
		t.Errorf("an agent's form that promises nothing was refused: %v", err)
	}
}
