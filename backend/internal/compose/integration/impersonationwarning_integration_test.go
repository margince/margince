// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The impersonation warning capture records on a new sender. The contact read
// serves it with its reason, and a user who may edit the contact clears it once.

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const punycodeSenderDomain = "xn--80ak6aa92e.com"

// firstMailAt dates the mail each suspect sender is captured from.
var firstMailAt = time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)

// captureSuspectSender runs capture's ensure on a first mail from a sender, as
// the sink does, and answers the contact it made.
func captureSuspectSender(t *testing.T, e *Env, displayName, address, domain string) ids.ContactID {
	t.Helper()
	mail := seedMailFrom(t, address, firstMailAt)
	res, err := e.Contacts.EnsureCounterparty(e.Admin(), contacts.EnsureCounterpartyInput{
		Email: address, Domain: domain, DisplayName: displayName, Replied: true,
		OwnerID: e.Rep1, ActivityID: ids.From[ids.ActivityKind](mail),
		Source: "gmail:" + mail.String(), CapturedBy: "connector:gmail",
	})
	if err != nil {
		t.Fatalf("capturing %s: %v", address, err)
	}
	if !res.ContactCreated {
		t.Fatalf("capture of %s made no contact: %+v", address, res)
	}
	return res.ContactID
}

// warningOn reads the warning the contact read serves to the reader.
func warningOn(reader context.Context, t *testing.T, e *Env, contact ids.ContactID) *crmcontracts.ContactImpersonationWarning {
	t.Helper()
	read, err := e.Contacts.GetContact(reader, contact, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the contact: %v", err)
	}
	return read.ImpersonationWarning
}

// clearedAudits counts the audit rows of a cleared warning whose event names
// them, which proves both came from one transaction.
func clearedAudits(t *testing.T, e *Env, contact ids.ContactID) (audits, events int) {
	t.Helper()
	const cleared = ` FROM audit_log a WHERE a.entity_type = 'contact' AND a.entity_id = $1
		AND a.action = 'update' AND a.after ? 'impersonation_warning_cleared'`
	audits = e.WsCount(t, `SELECT count(*)`+cleared, contact.UUID)
	events = e.WsCount(t, `SELECT count(*) FROM event_outbox o
		WHERE o.envelope->>'type' = 'contact.updated'
		  AND o.envelope->'trace'->>'audit_log_id' IN (SELECT a.id::text`+cleared+`)`, contact.UUID)
	return audits, events
}

func TestACapturedSenderShowingASignServesTheWarningWithItsReason(t *testing.T) {
	cases := []struct {
		name, displayName, address, domain string
		want                               crmcontracts.ContactImpersonationWarningReason
	}{
		{
			"a punycode domain", "Acme Billing", "billing@" + punycodeSenderDomain, punycodeSenderDomain,
			crmcontracts.ContactImpersonationWarningReasonPunycodeDomain,
		},
		{
			"a display name on another domain", "ceo@real-corp.example", "boss@spoof.test", "spoof.test",
			crmcontracts.ContactImpersonationWarningReasonDisplayNameOtherDomain,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := Setup(t)
			contact := captureSuspectSender(t, e, tc.displayName, tc.address, tc.domain)

			got := warningOn(e.Admin(), t, e, contact)
			if got == nil || got.Reason == nil || *got.Reason != tc.want {
				t.Fatalf("the contact read serves warning %+v, want reason %s", got, tc.want)
			}
			if got.DetectedAt.IsZero() {
				t.Error("the warning carries no detection time")
			}
			page := readContact360(t, e, contact.UUID).Contact.ImpersonationWarning
			if page == nil || page.Reason == nil || *page.Reason != tc.want {
				t.Errorf("the contact page read serves warning %+v, want reason %s", page, tc.want)
			}
		})
	}
}

func TestAnOrdinarySenderServesNoWarning(t *testing.T) {
	e := Setup(t)
	contact := captureSuspectSender(t, e, "Carol Example", "carol@acme.test", "acme.test")
	if got := warningOn(e.Admin(), t, e, contact); got != nil {
		t.Fatalf("an ordinary sender serves warning %+v, want none", got)
	}
}

// A contact outside the caller's row scope reads as absent on the read and on
// the clear. Neither says the contact or its warning exists.
func TestASeatThatCannotReadTheContactGetsNotFoundOnTheReadAndTheClear(t *testing.T) {
	e := Setup(t)
	contact := captureSuspectSender(t, e, "Acme Billing", "billing@"+punycodeSenderDomain, punycodeSenderDomain)
	e.MakeCapturePrivate(t, "contact", contact.UUID, e.Rep1)
	outsider := e.As(e.Rep3, []ids.UUID{e.Team2}, RepPerms)

	if _, err := e.Contacts.GetContact(outsider, contact, storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("the read: err = %v, want not found", err)
	}
	if err := e.Contacts.ClearImpersonationWarning(outsider, contact); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("the clear: err = %v, want not found", err)
	}
	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	if got := warningOn(owner, t, e, contact); got == nil {
		t.Error("the outsider's clear removed the warning")
	}
}

func TestLooksFineClearsTheWarningOnceWithItsAuditRowAndEvent(t *testing.T) {
	e := Setup(t)
	contact := captureSuspectSender(t, e, "Acme Billing", "billing@"+punycodeSenderDomain, punycodeSenderDomain)
	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)

	if err := e.Contacts.ClearImpersonationWarning(owner, contact); err != nil {
		t.Fatalf("clearing the warning: %v", err)
	}
	if got := warningOn(e.Admin(), t, e, contact); got != nil {
		t.Errorf("the contact read still serves the cleared warning %+v", got)
	}
	if page := readContact360(t, e, contact.UUID).Contact.ImpersonationWarning; page != nil {
		t.Errorf("the contact page read still serves the cleared warning %+v", page)
	}
	if audits, events := clearedAudits(t, e, contact); audits != 1 || events != 1 {
		t.Fatalf("%d audit rows and %d events for the clear, want 1 and 1", audits, events)
	}
	kept := e.WsScalar(t, `SELECT quarantine_reason || ' cleared by ' || quarantine_cleared_by FROM contact
		WHERE id = $1 AND quarantined_at IS NOT NULL AND quarantine_cleared_at IS NOT NULL`, contact.UUID)
	if want := "punycode_domain cleared by human:" + e.Rep1.String(); kept != want {
		t.Errorf("the row keeps %q, want %q", kept, want)
	}

	if err := e.Contacts.ClearImpersonationWarning(owner, contact); err != nil {
		t.Fatalf("a second clear: %v", err)
	}
	if audits, events := clearedAudits(t, e, contact); audits != 1 || events != 1 {
		t.Errorf("after a second clear: %d audit rows and %d events, want still 1 and 1", audits, events)
	}
}

// Neither a seat with no edit grant nor a user's agent clears the warning. The
// agent holds its user's full grant, so the refusal is the human-only rule.
func TestOnlyAUserWhoMayEditTheContactClearsTheWarning(t *testing.T) {
	e := Setup(t)
	contact := captureSuspectSender(t, e, "Acme Billing", "billing@"+punycodeSenderDomain, punycodeSenderDomain)
	reader := e.As(e.Rep2, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{roleReadOnly},
		Objects:  map[string]principal.ObjectGrant{objContact: {Read: true}},
		RowScope: principal.RowScopeAll,
	})
	agent := e.AgentFor(t, e.Rep1, []ids.UUID{e.Team1}, RepPerms)

	for name, ctx := range map[string]context.Context{"a reader": reader, "an agent": agent} {
		if err := e.Contacts.ClearImpersonationWarning(ctx, contact); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s: err = %v, want permission denied", name, err)
		}
	}
	if got := warningOn(e.Admin(), t, e, contact); got == nil {
		t.Error("a refused clear removed the warning")
	}
	if audits, events := clearedAudits(t, e, contact); audits != 0 || events != 0 {
		t.Errorf("%d audit rows and %d events after refused clears, want none", audits, events)
	}
}

// mergeAsAgent merges source into target as an agent acting for the admin,
// the principal that must not be able to answer a warning.
func mergeAsAgent(t *testing.T, e *Env, source, target ids.ContactID) {
	t.Helper()
	agent := e.AgentFor(t, e.AdminUser, nil, AdminPerms)
	if _, err := e.Contacts.MergeContact(agent, source, target, nil); err != nil {
		t.Fatalf("merging: %v", err)
	}
}

// carriedAudits counts the survivor's audit rows of a carried warning whose
// contact.updated event names them.
func carriedAudits(t *testing.T, e *Env, survivor ids.ContactID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM audit_log a
		  JOIN event_outbox o ON o.envelope->'trace'->>'audit_log_id' = a.id::text
		 WHERE a.entity_type = 'contact' AND a.entity_id = $1 AND a.action = 'update'
		   AND a.after ? 'impersonation_warning_carried' AND o.envelope->>'type' = 'contact.updated'`,
		survivor.UUID)
}

// A merge moves the suspect address onto the survivor, so the open warning
// goes with it. An agent may merge, and must not answer the warning that way.
func TestAMergeCarriesAnOpenWarningToTheSurvivor(t *testing.T) {
	const otherDomain = crmcontracts.ContactImpersonationWarningReasonDisplayNameOtherDomain
	e := Setup(t)
	suspect := captureSuspectSender(t, e, "ceo@real-corp.example", "boss@spoof.test", "spoof.test")
	trusted := captureSuspectSender(t, e, "Carol Example", "carol@acme.test", "acme.test")

	mergeAsAgent(t, e, suspect, trusted)

	got := warningOn(e.Admin(), t, e, trusted)
	if got == nil || got.Reason == nil || *got.Reason != otherDomain {
		t.Fatalf("the survivor serves warning %+v, want the merged-away contact's open one", got)
	}
	if n := carriedAudits(t, e, trusted); n != 1 {
		t.Errorf("%d audit rows with an event for the carried warning, want 1", n)
	}
}

// A survivor whose own warning a user cleared wears the merged-away contact's
// open one: the answer was about another sender.
func TestAMergeReopensASurvivorWhoseOwnWarningWasCleared(t *testing.T) {
	e := Setup(t)
	suspect := captureSuspectSender(t, e, "ceo@real-corp.example", "boss@spoof.test", "spoof.test")
	answered := captureSuspectSender(t, e, "Acme Billing", "billing@"+punycodeSenderDomain, punycodeSenderDomain)
	if err := e.Contacts.ClearImpersonationWarning(e.Admin(), answered); err != nil {
		t.Fatalf("clearing the survivor's own warning: %v", err)
	}

	mergeAsAgent(t, e, suspect, answered)

	got := warningOn(e.Admin(), t, e, answered)
	if got == nil || got.Reason == nil ||
		*got.Reason != crmcontracts.ContactImpersonationWarningReasonDisplayNameOtherDomain {
		t.Fatalf("the survivor serves warning %+v, want the merged-away contact's open one", got)
	}
	if audits, _ := clearedAudits(t, e, answered); audits != 1 {
		t.Errorf("%d audit rows of the earlier clear, want the 1 that names who answered", audits)
	}
}

// A survivor with an open warning keeps it, and a merge that carries nothing
// writes no audit row for a carry.
func TestAMergeKeepsTheSurvivorsOwnOpenWarning(t *testing.T) {
	e := Setup(t)
	suspect := captureSuspectSender(t, e, "ceo@real-corp.example", "boss@spoof.test", "spoof.test")
	warned := captureSuspectSender(t, e, "Acme Billing", "billing@"+punycodeSenderDomain, punycodeSenderDomain)

	mergeAsAgent(t, e, suspect, warned)

	got := warningOn(e.Admin(), t, e, warned)
	if got == nil || got.Reason == nil ||
		*got.Reason != crmcontracts.ContactImpersonationWarningReasonPunycodeDomain {
		t.Fatalf("the survivor serves warning %+v, want its own open one", got)
	}
	if n := carriedAudits(t, e, warned); n != 0 {
		t.Errorf("%d audit rows for a carry, want none", n)
	}
}
