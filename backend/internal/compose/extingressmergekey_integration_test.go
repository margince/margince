// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// setupVouchingIngress is setupIngress with a source that DECLARED the email
// merge key — the only difference that lets its records carry an address
// alongside the account they name their human by.
func setupVouchingIngress(t *testing.T) *ingressEnv {
	t.Helper()
	e := setupExtRuntime(t)
	composeCapturingUnit(t, ingressUnit,
		[]extension.Channel{{Provider: ingressProbeProvider, CredentialModel: extension.CredentialPerMember}},
		extension.IngressSource{
			System: ingressProbeSystem,
			Lands:  []extension.RecordKind{extension.KindActivity},
			Merges: []extension.MergeKey{extension.MergeKeyEmail},
		})
	bindCaptureForTest(t, e)
	grantCapture(t, e, e.Rep1)
	depositCredential(t, e, e.Rep1)
	return &ingressEnv{extRuntimeEnv: e, member: e.Rep1}
}

// corroboratedChannelMessage carries the address the provider knew, alongside
// the account that names the sender.
func corroboratedChannelMessage(key, senderEmail, account string) extension.Record {
	rec := aChannelMessage(key, senderEmail, account)
	rec.Counterparty.Email = senderEmail
	return rec
}

func (e *ingressEnv) ingest(t *testing.T, rec extension.Record) error {
	t.Helper()
	_, err := e.ingestingRuntime().Ingest(context.Background(), extension.UserID(e.member.String()), rec)
	return err
}

// End to end through the real ingress, capture and resolution path: a human
// already captured from mail sends a direct message and must not become a second
// contact.
//
// The incumbent is created by the REAL writer — a mail-shaped record from the
// same unit, resolved by the ladder's personal-domain tier — rather than
// inserted by the test, so what this proves is what production does.
//
// The count is the assertion that matters. One person, holding both keys: the
// address they were already known by, and the account they can be answered at.
// Without the address the ladder cannot see the incumbent and mints a twin,
// which nobody notices until a human opens the Duplicates surface.
func TestADirectMessageFindsTheHumanAlreadyCapturedFromMail(t *testing.T) {
	e := setupVouchingIngress(t)
	registerProbeTransport(t, e)
	const sender = "known.contact@globex.example"

	// The incumbent is built the way production builds one: the mail record
	// DEFERS the sender to the ledger, and a verdict admits them. No tier mints
	// a contact from a first inbound message any more — the ladder's job is to
	// capture the mail and leave who it is with to the verdict — so a fixture
	// that expected the ingest alone to leave a person would be describing a
	// pipeline this one does not have.
	if err := e.ingest(t, aProviderRecord("ws-7:2001", sender)); err != nil {
		t.Fatalf("landing the mail record that defers the sender: %v", err)
	}
	admitPendingSender(t, e, sender)
	if got := e.countAsWorkspace(t,
		`SELECT count(*) FROM person_email WHERE email = $1 AND archived_at IS NULL`, sender); got != 1 {
		t.Fatalf("the admitted sender left %d records carrying %s; this test needs exactly the one incumbent", got, sender)
	}

	if err := e.ingest(t, corroboratedChannelMessage("ws-7:2002", sender, "U-2002")); err != nil {
		t.Fatalf("landing the direct message: %v", err)
	}

	if got := e.countAsWorkspace(t,
		`SELECT count(*) FROM person_email WHERE email = $1 AND archived_at IS NULL`, sender); got != 1 {
		t.Fatalf("%d live records carry %s, want 1 — the direct message minted a twin of a human already captured from mail", got, sender)
	}
	if got := e.countAsWorkspace(t, `
		SELECT count(*) FROM person_channel_identity pci
		  JOIN person_email pe ON pe.person_id = pci.person_id
		 WHERE pe.email = $1 AND pci.channel_user_id = $2
		   AND pci.archived_at IS NULL AND pe.archived_at IS NULL`,
		sender, "U-2002"); got != 1 {
		t.Errorf("the account is not bound to the human the address found — without the bind they are unreachable for a reply and re-resolved by address on every later message")
	}
}

// The declaration is what unlocks it, and a source that never made one has the
// record REFUSED rather than quietly stripped. Stripping would be worse: the
// unit would believe it had contributed the evidence, and the duplicate it was
// trying to prevent would appear anyway, with nothing saying why.
func TestAnUndeclaredSourceCannotCorroborateByAddress(t *testing.T) {
	e := setupIngress(t) // declares no merge key
	registerProbeTransport(t, e)
	const sender = "stranger@gmail.com"

	err := e.ingest(t, corroboratedChannelMessage("ws-7:2003", sender, "U-2003"))

	if !errors.Is(err, extension.ErrInvalid) {
		t.Fatalf("ingest = %v, want an ErrInvalid-class refusal naming the missing declaration", err)
	}
	if got := e.countAsWorkspace(t,
		`SELECT count(*) FROM activity WHERE source_id = $1`, "ws-7:2003"); got != 0 {
		t.Errorf("%d activities landed for a refused record, want 0 — the refusal runs before the transaction opens", got)
	}
}

// admitPendingSender runs the real verdict engine over the deferral the ingest
// left, so the incumbent this test needs is written by the code that writes one
// in production rather than inserted by the test.
//
// A `person` answer is the ordinary admission: the sender turned out to be a
// human this business deals with, and that is the moment a contact exists.
func admitPendingSender(t *testing.T, e *ingressEnv, email string) {
	t.Helper()
	var dispositionID ids.UUID
	e.readAsWorkspace(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id FROM capture_pending_counterparty
			 WHERE email = $1 AND status = 'pending'`, email).Scan(&dispositionID)
	})

	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindPerson}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("admitting %s by verdict: %v", email, err)
	}
}
