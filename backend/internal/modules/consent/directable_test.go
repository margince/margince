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

// A rep, like an admin, may direct one message past every policy refusal but
// the subject's own stop. A stop, a dead address, an unresolved recipient and a
// missing check are never directable, nor is anything for a seat without the
// grant.
func TestARepMayDirectASendPastEveryPolicyRefusalButAStop(t *testing.T) {
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
				want := reason != commsauthz.ReasonHardBounce && reason != commsauthz.ReasonNoSubject &&
					commsauthz.LevelForReason(reason) != commsauthz.LevelSubject
				if got := directableBy(ctx, []RefusedRecipient{{ReasonCode: reason}}); got != want {
					t.Errorf("directable past %s = %v, want %v", reason, got, want)
				}
			}
			stopBeside := []RefusedRecipient{
				{ReasonCode: commsauthz.ReasonMarketingConsentRequired}, {ReasonCode: commsauthz.ReasonObjection},
			}
			if directableBy(ctx, stopBeside) {
				t.Error("a stop beside a passable refusal is directable")
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

// An agent may record its user's decision in their conversation. A scheduled
// run has nobody present, so it may read a review but not decide it.
func TestOnlyAnAttendedAgentStatesItsUsersDecision(t *testing.T) {
	agent := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:t", OnBehalfOf: ids.NewV7(), PassportID: ids.NewV7(),
	})
	if err := requireSomebodyPresent(agent); err != nil {
		t.Fatalf("an attended agent was refused: %v", err)
	}
	for name, ctx := range map[string]context.Context{
		"unattended":    principal.WithUnattended(agent),
		"scheduled run": principal.WithAgentRunID(agent, ids.NewV7()),
	} {
		if err := requireAUserOrTheirAgent(ctx); err != nil {
			t.Errorf("%s agent may not read the review: %v", name, err)
		}
		if err := requireSomebodyPresent(ctx); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s agent deciding = %v, want ErrPermissionDenied", name, err)
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
