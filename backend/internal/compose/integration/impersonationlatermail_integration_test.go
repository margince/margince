// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

// A later mail that shows a sign of impersonation labels a contact capture
// already holds, while the contact has no verdict. A warning a user cleared
// stays cleared, and one that stands is left alone.

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	laterMailSeatAddr  = "rep@authz.test"
	laterMailSender    = "pat@acme.test"
	laterMailCleanName = "Pat Example"
	// A display name holding an address on another domain than the sender's.
	laterMailSpoofedName = "ceo@real-corp.example"
)

// correspondWithLaterSender connects the seat's mailbox and records that the workspace
// wrote to the address, so capture's ladder files the sender's replies under a contact.
func correspondWithLaterSender(t *testing.T, e *integration.Env, address string) {
	t.Helper()
	e.GrantRole(t, e.Rep1, "rep")
	e.WsExec(t, `INSERT INTO capture_connection
		(user_id, provider, status, credential_ref, mail_posture, account_label)
		VALUES (@owner, 'gmail', 'connected', 'vault:test', 'shared', @address)
		ON CONFLICT (user_id, provider, lower(account_label)) WHERE user_id IS NOT NULL DO NOTHING`,
		pgx.NamedArgs{"owner": e.Rep1, "address": laterMailSeatAddr})
	e.WsExec(t, `INSERT INTO activity (kind, subject, body, direction, source_system, source_id,
		source, captured_by, thread_key, counterparty_email, counterparty_outbound_attested)
		VALUES ('email', 'Angebot', 'Anbei.', 'outbound', 'gmail', @source, 'gmail:'||@source,
		        'connector:gmail', @thread, @address, true)`,
		pgx.NamedArgs{"source": ids.NewV7().String(), "thread": laterMailThread(address), "address": address})
}

// laterMailThread is the one thread the workspace and the sender write on.
func laterMailThread(address string) string { return "thread-" + address }

// captureLaterMailFrom lands one reply from the sender, under the display name,
// through the sink production composes with the counterparty ensure wired.
func captureLaterMailFrom(t *testing.T, e *integration.Env, displayName, address string) {
	t.Helper()
	if err := captureLaterMail(e, displayName, address); err != nil {
		t.Fatalf("capturing the mail from %s: %v", address, err)
	}
}

// captureLaterMail is the capture itself, answering its error so a goroutine can report it.
func captureLaterMail(e *integration.Env, displayName, address string) error {
	sourceID := ids.NewV7().String()
	sink := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{})
	_, err := sink.Upsert(connectorCtx(e, "gmail", e.Rep1), connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: sourceID},
		Counterparty: connector.Counterparty{
			Email: address, DisplayName: displayName,
			Domain: address[strings.LastIndex(address, "@")+1:], Direction: connector.DirectionInbound,
		},
		Fields: capture.ActivityFields{
			Kind: "email", Subject: "Rechnung", Body: "Bitte zahlen.", Direction: connector.DirectionInbound,
		},
		ThreadKey: laterMailThread(address),
		Addresses: []string{address, laterMailSeatAddr},
		Source:    "gmail:" + sourceID, CapturedBy: "connector:gmail",
	})
	return err
}

// laterMailContact answers the live contact capture filed the address under.
func laterMailContact(t *testing.T, e *integration.Env, address string) ids.ContactID {
	t.Helper()
	id := scalar[ids.UUID](t, e, `SELECT c.id FROM contact c JOIN contact_email ce ON ce.contact_id = c.id
		WHERE ce.email = $1 AND c.merged_into_id IS NULL`, address)
	return ids.From[ids.ContactKind](id)
}

// laterMailOwner is the seat whose mailbox captured the mail, and who alone may
// read a contact capture has not yet judged the workspace's.
func laterMailOwner(e *integration.Env) context.Context {
	return e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
}

// laterWarningOn reads the warning the contact read serves the mailbox owner.
func laterWarningOn(t *testing.T, e *integration.Env, contact ids.ContactID) *crmcontracts.ContactImpersonationWarning {
	t.Helper()
	read, err := e.Contacts.GetContact(laterMailOwner(e), contact, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the contact: %v", err)
	}
	return read.ImpersonationWarning
}

// raisedAudits counts the audit rows of a raised warning whose contact.updated
// event names them, which proves both came from one transaction.
func raisedAudits(t *testing.T, e *integration.Env, contact ids.ContactID) int {
	t.Helper()
	return scalar[int](t, e, `SELECT count(*)::int FROM audit_log a
		  JOIN event_outbox o ON o.envelope->'trace'->>'audit_log_id' = a.id::text
		 WHERE a.entity_type = 'contact' AND a.entity_id = $1 AND a.action = 'update'
		   AND a.after ? 'impersonation_warning_raised' AND o.envelope->>'type' = 'contact.updated'`,
		contact.UUID)
}

// clearAsOwner presses Looks fine on the warning the mailbox owner is shown.
func clearAsOwner(t *testing.T, e *integration.Env, contact ids.ContactID) {
	t.Helper()
	shown := laterWarningOn(t, e, contact)
	if shown == nil {
		t.Fatal("the contact serves no warning to clear")
	}
	if err := e.Contacts.ClearImpersonationWarning(laterMailOwner(e), contact, shown.DetectedAt); err != nil {
		t.Fatalf("clearing the warning: %v", err)
	}
}

func TestALaterMailShowingASignLabelsAContactWithNoVerdict(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	correspondWithLaterSender(t, e, laterMailSender)
	captureLaterMailFrom(t, e, laterMailCleanName, laterMailSender)
	contact := laterMailContact(t, e, laterMailSender)
	if got := laterWarningOn(t, e, contact); got != nil {
		t.Fatalf("a clean first mail left warning %+v, want none", got)
	}

	captureLaterMailFrom(t, e, laterMailSpoofedName, laterMailSender)

	got := laterWarningOn(t, e, contact)
	if got == nil || got.Reason == nil ||
		*got.Reason != crmcontracts.ContactImpersonationWarningReasonDisplayNameOtherDomain {
		t.Fatalf("after a mail with a sign the contact serves warning %+v, want the display-name sign", got)
	}
	if n := raisedAudits(t, e, contact); n != 1 {
		t.Errorf("%d audit rows with an event for the raised warning, want 1", n)
	}
}

func TestAContactAUserClearedStaysClearedAfterAnotherMailWithASign(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	correspondWithLaterSender(t, e, laterMailSender)
	captureLaterMailFrom(t, e, laterMailCleanName, laterMailSender)
	captureLaterMailFrom(t, e, laterMailSpoofedName, laterMailSender)
	contact := laterMailContact(t, e, laterMailSender)
	clearAsOwner(t, e, contact)

	captureLaterMailFrom(t, e, laterMailSpoofedName, laterMailSender)

	if got := laterWarningOn(t, e, contact); got != nil {
		t.Fatalf("the cleared contact serves warning %+v again, want none", got)
	}
	if n := raisedAudits(t, e, contact); n != 1 {
		t.Errorf("%d audit rows for a raised warning, want only the first", n)
	}
}

func TestAMailWithASignLeavesALabelledContactUntouched(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	correspondWithLaterSender(t, e, laterMailSender)
	captureLaterMailFrom(t, e, laterMailSpoofedName, laterMailSender)
	contact := laterMailContact(t, e, laterMailSender)
	first := laterWarningOn(t, e, contact)
	if first == nil {
		t.Fatal("a first mail with a sign left no warning")
	}

	captureLaterMailFrom(t, e, laterMailSpoofedName, laterMailSender)

	got := laterWarningOn(t, e, contact)
	if got == nil || !got.DetectedAt.Equal(first.DetectedAt) {
		t.Fatalf("the warning reads %+v after a second mail, want the first one detected at %s", got, first.DetectedAt)
	}
	if n := raisedAudits(t, e, contact); n != 0 {
		t.Errorf("%d audit rows for a raised warning on a labelled contact, want none", n)
	}
}

// Two mails with a sign captured side by side write one label between them.
func TestTwoMailsWithASignCapturedAtOnceRaiseOneWarning(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	correspondWithLaterSender(t, e, laterMailSender)
	captureLaterMailFrom(t, e, laterMailCleanName, laterMailSender)
	contact := laterMailContact(t, e, laterMailSender)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Go(func() { errs[i] = captureLaterMail(e, laterMailSpoofedName, laterMailSender) })
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("capturing a mail at once: %v", err)
		}
	}

	if laterWarningOn(t, e, contact) == nil {
		t.Fatal("two mails with a sign left no warning")
	}
	if n := raisedAudits(t, e, contact); n != 1 {
		t.Errorf("%d audit rows for the raised warning, want 1", n)
	}
}

// A survivor that took a merged-away contact's open warning has a verdict. A
// later mail with a sign from the suspect address writes nothing more.
func TestAMailWithASignLeavesASurvivorThatCarriedAWarningUntouched(t *testing.T) {
	t.Parallel()
	const suspect = "boss@spoof.test"
	e := integration.Setup(t)
	correspondWithLaterSender(t, e, suspect)
	correspondWithLaterSender(t, e, laterMailSender)
	captureLaterMailFrom(t, e, laterMailSpoofedName, suspect)
	captureLaterMailFrom(t, e, laterMailCleanName, laterMailSender)
	survivor := laterMailContact(t, e, laterMailSender)
	agent := e.AgentFor(t, e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
	if _, err := e.Contacts.MergeContact(agent, laterMailContact(t, e, suspect), survivor, nil); err != nil {
		t.Fatalf("merging: %v", err)
	}
	carried := laterWarningOn(t, e, survivor)
	if carried == nil {
		t.Fatal("the survivor carries no warning after the merge")
	}

	captureLaterMailFrom(t, e, laterMailSpoofedName, suspect)

	got := laterWarningOn(t, e, survivor)
	if got == nil || !got.DetectedAt.Equal(carried.DetectedAt) {
		t.Fatalf("the survivor serves %+v, want the carried warning detected at %s", got, carried.DetectedAt)
	}
	if n := raisedAudits(t, e, survivor); n != 0 {
		t.Errorf("%d audit rows for a raised warning on the survivor, want none", n)
	}
}
