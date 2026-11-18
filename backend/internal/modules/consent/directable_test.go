// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestARepMayDirectASendPastEveryPolicyRefusal(t *testing.T) {
	seat := func(role string, grant bool) context.Context {
		return principal.WithActor(context.Background(), principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:reviewer",
			Permissions: principal.Permissions{
				RoleKeys: []string{role}, RowScope: principal.RowScopeAll,
				Objects: map[string]principal.ObjectGrant{entityCommunicationException: {Create: grant}},
			},
		})
	}
	for _, role := range []string{"rep", "admin"} {
		t.Run(role, func(t *testing.T) {
			ctx := seat(role, true)
			for _, reason := range commsauthz.RefusalCodes() {
				want := reason != commsauthz.ReasonHardBounce && reason != commsauthz.ReasonNoSubject
				if got := directableBy(ctx, []RefusedRecipient{{ReasonCode: reason}}); got != want {
					t.Errorf("directable past %s = %v, want %v", reason, got, want)
				}
			}
			stopBeside := []RefusedRecipient{
				{ReasonCode: commsauthz.ReasonMarketingConsentRequired}, {ReasonCode: commsauthz.ReasonObjection},
			}
			if !directableBy(ctx, stopBeside) {
				t.Error("combined policy warnings must remain directable")
			}
			if directableBy(ctx, nil) {
				t.Error("a missing check offers a send instruction")
			}
		})
	}
	if directableBy(seat("rep", false), []RefusedRecipient{{ReasonCode: commsauthz.ReasonNoEvidence}}) {
		t.Error("a seat without the exception grant may direct a send")
	}
}

// A refused send's decision is the user's own, so only a human at the keyboard
// records it or finds its held message. An agent on their passport may read the
// review, and may not decide it, attended or not.
func TestOnlyAHumanDecidesARefusedSend(t *testing.T) {
	human := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:rep", UserID: ids.NewV7()})
	if err := requireAHumanAtTheKeyboard(human); err != nil {
		t.Fatalf("a human was refused: %v", err)
	}
	agent := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:t", OnBehalfOf: ids.NewV7(), PassportID: ids.NewV7(),
	})
	if err := requireAUserOrTheirAgent(agent); err != nil {
		t.Errorf("an agent may not read the review: %v", err)
	}
	for name, ctx := range map[string]context.Context{"attended": agent, "unattended": principal.WithUnattended(agent)} {
		if err := requireAHumanAtTheKeyboard(ctx); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("an %s agent deciding = %v, want ErrPermissionDenied", name, err)
		}
	}
}

// A decision answers the refusals its review showed. A new reason for the same
// recipient, or a recipient the review never named, needs a new decision.
func TestADecisionAnswersOnlyTheRefusalsItWasShown(t *testing.T) {
	denied := func(email, reason string) commsauthz.Decision {
		return commsauthz.Decision{Recipient: connector.Recipient{Email: email}, Verdict: commsauthz.VerdictDeny, ReasonCode: reason}
	}
	shown := []RefusedRecipient{{Address: "ana@example.test", ReasonCode: commsauthz.ReasonNoMarketingConsent}}
	for name, tc := range map[string]struct {
		now  []commsauthz.Decision
		want bool
	}{
		"same refusal":    {[]commsauthz.Decision{denied("Ana@Example.test", commsauthz.ReasonNoMarketingConsent)}, true},
		"new reason":      {[]commsauthz.Decision{denied("ana@example.test", commsauthz.ReasonObjection)}, false},
		"new recipient":   {[]commsauthz.Decision{denied("ben@example.test", commsauthz.ReasonNoMarketingConsent)}, false},
		"nothing refused": {nil, true},
	} {
		if got := everyRefusalWasShown(shown, commsauthz.DecisionSet{Decisions: tc.now}); got != tc.want {
			t.Errorf("%s: shown = %v, want %v", name, got, tc.want)
		}
	}
}
