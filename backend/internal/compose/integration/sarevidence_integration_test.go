// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The evidence rows the access export hands a subject: which door attested a
// message's identity, and which wording version a consent state rests on.

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// evidenceSubject creates a contact through the real contact writer.
func evidenceSubject(t *testing.T, e *Env) ids.UUID {
	t.Helper()
	created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Alice Kessler", Source: "manual"})
	if err != nil {
		t.Fatalf("creating the subject: %v", err)
	}
	return ids.UUID(created.Id)
}

func exportOf(t *testing.T, e *Env, contact ids.UUID) privacy.SARPackage {
	t.Helper()
	pkg, err := privacy.AssembleSAR(e.Admin(), e.DB(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}
	return pkg
}

func TestTheAccessExportNamesTheDoorThatAttestedAMessage(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	contact := evidenceSubject(t, e)
	mail := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, direction, subject, body, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'Terms', 'Sounds good.', now(), 'manual', 'human:seed')`, mail)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id)
		VALUES ($1, 'contact', $2)`, mail, contact)
	const messageID = "<terms-1@buyer.test>"
	ctx := e.Admin()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := activities.ClaimIdentity(ctx, tx, ids.From[ids.ActivityKind](mail),
			activities.IdentityKindMail, messageID, "connector:gmail")
		return err
	}); err != nil {
		t.Fatalf("claiming the message identity: %v", err)
	}

	pkg := exportOf(t, e, contact)
	for _, row := range pkg.MessageIdentities {
		if row["identity_key"] == messageID {
			if row["attested_by"] != "connector:gmail" {
				t.Fatalf("attested_by = %v, want the door that claimed it", row["attested_by"])
			}
			return
		}
	}
	t.Fatalf("the export carries no identity for the subject's mail: %v", pkg.MessageIdentities)
}

func TestTheAccessExportNamesTheWordingVersionOfAConsentState(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	contact := evidenceSubject(t, e)
	// No writer stamps the version on the state row today; an older grant may
	// have, and the export reads what the row holds.
	purpose := ids.NewV7()
	e.WsExec(t, `INSERT INTO consent_purpose (id, key, label) VALUES ($1, $2, 'Evidence')`, purpose, "evidence_"+strings.ReplaceAll(purpose.String(), "-", ""))
	e.WsExec(t, `INSERT INTO contact_consent (contact_id, purpose_id, state, lawful_basis, policy_version, captured_at, source)
		VALUES ($1, $2, 'granted', 'consent', 'v7', now(), 'api')`, contact, purpose)

	pkg := exportOf(t, e, contact)
	if len(pkg.Consent) != 1 || pkg.Consent[0]["policy_version"] != "v7" {
		t.Fatalf("consent = %v, want the one state with its wording version v7", pkg.Consent)
	}
}
