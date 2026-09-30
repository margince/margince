// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The carriers are exported and take a transaction they do not own, so each one
// asks for the authority the merge door behind it asks for. And the production
// wiring reaches them: a merge through the HTTP handlers or the MCP provider
// carries, rather than refusing for want of a seam.

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// contactOwnedBy creates a contact owned by one seat, so row scope has
// something to refuse.
func (c *carryEnv) contactOwnedBy(t *testing.T, owner ids.UUID, name, address string) ids.ContactID {
	t.Helper()
	seat := ids.From[ids.UserKind](owner)
	created, err := c.e.Contacts.CreateContact(c.admin, contacts.CreateContactInput{
		FullName: name, Source: "manual", OwnerID: &seat,
		Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true, Position: 1}},
	})
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	return ids.From[ids.ContactKind](ids.UUID(created.Id))
}

func (c *carryEnv) carryConsentAs(ctx context.Context, from, to commsauthz.StopSubject) error {
	return c.e.DB().Tx(ctx, func(tx pgx.Tx) error { return c.consent.CarrySatellitesTx(ctx, tx, from, to) })
}

// A caller who may create contacts but not change them cannot move one
// contact's links onto another through the consent carrier.
func TestTheConsentCarrierRefusesACallerWhoCannotMergeContacts(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Grant Retired", "grant@carry.test")
	survivor := c.contactAt(t, "Grant Survivor", "grant-survivor@carry.test")
	c.withdrawalLink(t, consent.WithdrawalMintInput{Address: "grant@carry.test", ContactID: retired})
	creator := c.e.As(c.e.AdminUser, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"contact": {Create: true, Read: true}},
		RowScope: principal.RowScopeAll,
	})

	err := c.carryConsentAs(creator,
		commsauthz.ContactStopSubject(retired), commsauthz.ContactStopSubject(survivor))
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a contact:create caller carried a contact's links (err=%v), want it refused", err)
	}
	if n := c.count(t, `SELECT count(*) FROM withdrawal_credential WHERE contact_id = $1`, retired); n != 1 {
		t.Errorf("the refused carry moved the link anyway")
	}
}

// A rep who may update contacts, but only their own team's, cannot carry
// between two contacts another team owns — through either carrier. Everyone
// reads every contact here, so the refusal is write authority, not a 404.
func TestTheCarriersRefuseContactsOutsideTheCallersRowScope(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactOwnedBy(t, c.e.Rep1, "Scope Retired", "scope@carry.test")
	survivor := c.contactOwnedBy(t, c.e.Rep1, "Scope Survivor", "scope-survivor@carry.test")
	outsider := c.e.As(c.e.Rep3, nil, integration.RepPerms)

	err := c.carryConsentAs(outsider,
		commsauthz.ContactStopSubject(retired), commsauthz.ContactStopSubject(survivor))
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the consent carrier answered %v for contacts the caller may not change, want permission denied", err)
	}
	err = c.e.DB().Tx(outsider, func(tx pgx.Tx) error { return c.intros.CarryIntrosTx(outsider, tx, retired, survivor) })
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the intro carrier answered %v for contacts the caller may not change, want permission denied", err)
	}
}

// A promoting rep holds lead:update and contact:create and never contact:update.
// The carry must still run for them, or the gate would roll back every
// promotion of a lead holding a link.
func TestARepWhoMayPromoteButNotEditContactsStillCarriesTheLeadsLinks(t *testing.T) {
	c := setupCarry(t)
	owner := ids.From[ids.UserKind](c.e.Rep1)
	name, company, address := "Pia Promote", "Scope GmbH", "pia@leadcarry.test"
	lead, _, err := c.contacts.CreateLead(c.admin, contacts.CreateLeadInput{
		FullName: &name, CompanyName: &company, Email: &address, OwnerID: &owner, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	leadID := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	c.withdrawalLink(t, consent.WithdrawalMintInput{Address: address, LeadID: leadID})
	perms := maps.Clone(integration.RepPerms.Objects)
	perms["lead"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	perms["contact"] = principal.ObjectGrant{Create: true, Read: true}
	promoter := c.e.As(c.e.Rep1, []ids.UUID{c.e.Team1}, principal.Permissions{
		RoleKeys: integration.RepPerms.RoleKeys, Objects: perms, RowScope: principal.RowScopeOwn,
	})

	contact, _, err := c.contacts.PromoteLead(promoter, leadID, contacts.PromoteLeadInput{
		Trigger: string(contacts.TriggerHumanQualify),
	})
	if err != nil {
		t.Fatalf("the promotion was refused: %v", err)
	}
	if n := c.count(t, `SELECT count(*) FROM withdrawal_credential WHERE contact_id = $1`, ids.UUID(contact.Id)); n != 1 {
		t.Errorf("the promoted contact holds %d link(s), want the lead's 1", n)
	}
}

// holdsLinkAndAsk gives a contact one row for each carrier to move.
func (c *carryEnv) holdsLinkAndAsk(t *testing.T, contact ids.ContactID, address string) {
	t.Helper()
	c.withdrawalLink(t, consent.WithdrawalMintInput{Address: address, ContactID: contact})
	c.ask(t, contact, c.e.Rep1, nil)
}

func (c *carryEnv) assertCarried(t *testing.T, retired, survivor ids.ContactID) {
	t.Helper()
	if n := c.count(t, `SELECT count(*) FROM withdrawal_credential WHERE contact_id = $1`, survivor); n != 1 {
		t.Errorf("the survivor holds %d link(s), want the retired contact's 1", n)
	}
	if n := c.count(t, `SELECT count(*) FROM intro_request WHERE contact_id = $1`, survivor); n != 1 {
		t.Errorf("the survivor holds %d ask(s), want the retired contact's 1", n)
	}
	if n := c.count(t, `SELECT count(*) FROM contact WHERE id = $1 AND merged_into_id = $2`, retired, survivor); n != 1 {
		t.Error("the retired contact was not merged")
	}
}

// The HTTP handlers the server mounts carry both kinds of row.
func TestTheServersContactHandlersAreWiredToCarry(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "HTTP Retired", "http@carry.test")
	survivor := c.contactAt(t, "HTTP Survivor", "http-survivor@carry.test")
	c.holdsLinkAndAsk(t, retired, "http@carry.test")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/contacts/"+retired.String()+"/merge",
		strings.NewReader(`{"target_id":"`+survivor.String()+`"}`)).WithContext(c.admin)
	req.Header.Set("Content-Type", "application/json")
	newContactsHandlers(c.e.Pool).MergeContact(rec, req, openapi_types.UUID(retired.UUID), crmcontracts.MergeContactParams{})
	if rec.Code != http.StatusOK {
		t.Fatalf("the merge answered %d: %s", rec.Code, rec.Body.String())
	}
	c.assertCarried(t, retired, survivor)
}

// The MCP provider merges records too, and carries the same way.
func TestTheProviderIsWiredToCarry(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "MCP Retired", "mcp@carry.test")
	survivor := c.contactAt(t, "MCP Survivor", "mcp-survivor@carry.test")
	c.holdsLinkAndAsk(t, retired, "mcp@carry.test")

	if _, err := NewProviderFor(c.e.DB()).Merge(c.admin, datasource.MergeInput{
		Type: datasource.EntityContact, SourceID: retired.UUID, TargetID: survivor.UUID,
	}); err != nil {
		t.Fatalf("the provider's merge failed: %v", err)
	}
	c.assertCarried(t, retired, survivor)
}
