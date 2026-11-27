// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"context"
	"encoding/json"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A staged agent mail is asked about the mailbox of the principal its release
// runs as, not of the human who reviews it.
func TestAnAgentMailIsAskedAboutTheMailboxItSendsFrom(t *testing.T) {
	t.Parallel()
	e := setupStaging(t)
	reviewer := e.asHumanWith(grantsFor(map[string]principal.ObjectGrant{
		objectActivity: {Create: true, Read: true},
		tableContact:   {Read: true},
	}))
	passport := e.seedPassport(t)
	id, err := e.svc.Stage(e.asPassport(passport), StageInput{
		Kind: "send_email",
		ProposedChange: json.RawMessage(`{"operation":"sendEmail","path":"/v1/activities/a1/send-email",` +
			`"body":{"to":["anna@example.test"],"subject":"Hi","body":"Text"}}`),
		DiffHash: "hash-agent-mail",
		Summary:  "Reply to Anna?",
	})
	if err != nil {
		t.Fatalf("staging the agent mail: %v", err)
	}

	cases := []struct {
		name          string
		agentHasInbox bool
		blocked       bool
	}{
		{"only the reviewing human has a mailbox", false, true},
		{"only the sending agent has a mailbox", true, false},
	}
	for _, tc := range cases {
		e.svc.WithSendProbe(func(ctx context.Context) (bool, error) {
			actor, _ := principal.Actor(ctx)
			return (actor.Type == principal.PrincipalAgent) == tc.agentHasInbox, nil
		})
		got, err := e.svc.GetWire(reviewer, id)
		if err != nil {
			t.Fatalf("%s: reading the approval: %v", tc.name, err)
		}
		blocked := got.MailboxSendCapable != nil && !*got.MailboxSendCapable
		if blocked != tc.blocked {
			t.Errorf("%s: mailbox_send_capable = %v, want blocked=%v", tc.name, got.MailboxSendCapable, tc.blocked)
		}
	}
	var _ crmcontracts.Approval
}
