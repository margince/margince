// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestSignatureTitlesFillTheCurrentRoleAndKeepAHumansRoleChoice(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		initial *string
		want    string
		machine bool
		agent   bool
		source  string
	}{
		{name: "empty", want: "VP Finance"},
		{name: "human role", initial: new("Founder"), want: "Founder"},
		{name: "captured human role", initial: new("Founder"), want: "Founder", source: "capture"},
		{name: "machine role", initial: new("Analyst"), want: "VP Finance", machine: true},
		{name: "agent role", initial: new("Analyst"), want: "Founder", machine: true, agent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			contact := ids.From[ids.ContactKind](seedEnrichContact(t, e, "lena@signature.example", "Lena | VP Finance"))
			var activity ids.UUID
			if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				return tx.QueryRow(context.Background(), `SELECT activity_id FROM activity_link WHERE contact_id=@contact`, pgx.NamedArgs{"contact": contact}).Scan(&activity)
			}); err != nil {
				t.Fatal(err)
			}
			company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Employer", nil))
			actor, source := e.Admin(), "manual"
			if tc.source != "" {
				source = tc.source
			}
			if tc.machine {
				actor, source = principal.SystemActing(actor, "system:employment-fixture"), "capture"
			}
			relationship, err := e.Contacts.CreateRelationship(actor, contacts.CreateRelationshipInput{Kind: "employment", ContactID: &contact, CompanyID: &company, Role: tc.initial, Source: source})
			if err != nil {
				t.Fatal(err)
			}
			if tc.agent {
				actor, ok := principal.Actor(e.Admin())
				if !ok {
					t.Fatal("admin principal missing")
				}
				actor.Type, actor.ID = principal.PrincipalAgent, "agent:role-choice"
				actor.OnBehalfOf = e.AdminUser
				actor.Scopes = principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite)
				if _, err := e.Contacts.UpdateRelationship(principal.WithActor(e.Admin(), actor), relationship.ID, contacts.UpdateRelationshipInput{Role: new(tc.want)}); err != nil {
					t.Fatal(err)
				}
			}
			applied, err := e.Contacts.ApplySignatureFields(e.Admin(), contact, activity, []contacts.SignatureField{{Name: "title", Value: "VP Finance", Evidence: "Lena | VP Finance", Confidence: 0.95}}, true)
			if err != nil {
				t.Fatal(err)
			}
			if applied.Applied != 1 || applied.SourceLimited {
				t.Fatalf("signature not applied: %+v", applied)
			}
			role := e.WsScalar(t, `SELECT coalesce(role,'') FROM relationship WHERE id=@id`, pgx.NamedArgs{"id": relationship.ID})
			if role != tc.want {
				t.Fatalf("role=%q, want %q; applied=%+v", role, tc.want, applied)
			}
		})
	}
}
