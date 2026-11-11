// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestErasurePhysicallyClearsRecheckedDuplicateEvidence(t *testing.T) {
	e := Setup(t)
	create := func(name, address string) ids.ContactID {
		t.Helper()
		contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
			FullName: name, Source: "manual",
			Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
			Phones: []contacts.ContactPhoneInput{{Phone: "+49305550101", PhoneType: "work", IsPrimary: true}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return ids.From[ids.ContactKind](ids.UUID(contact.Id))
	}
	other := create("Mia Schulz", "mia@one.example")
	subject := create("Evan Karl", "evan@two.example")
	if _, err := e.Contacts.UpdateContact(e.Admin(), subject, contacts.UpdateContactInput{FullName: new("Mia Schulz")}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Contacts.UpdateContact(e.Admin(), subject, contacts.UpdateContactInput{Phones: []contacts.ContactPhoneInput{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Contacts.RecheckOpenDedupeCandidates(principal.SystemActing(e.Admin(), "system:dedupe_recheck")); err != nil {
		t.Fatal(err)
	}
	var pair ids.UUID
	var audits []ids.UUID
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(e.Admin(), `SELECT id FROM dedupe_candidate
 WHERE left_contact_id=ANY(@contacts) AND right_contact_id=ANY(@contacts)`,
			pgx.StrictNamedArgs{"contacts": []ids.UUID{other.UUID, subject.UUID}}).Scan(&pair); err != nil {
			return err
		}
		var snapshots int
		if err := tx.QueryRow(e.Admin(), `SELECT count(*) FROM audit_log WHERE entity_type='dedupe_candidate'
 AND entity_id=@pair AND after->'evidence' IS NOT NULL AND after::text LIKE '%Mia Schulz%'`,
			pgx.StrictNamedArgs{"pair": pair}).Scan(&snapshots); err != nil {
			return err
		}
		if snapshots == 0 {
			t.Fatal("the real phone edit wrote no personal duplicate evidence snapshot")
		}
		return tx.QueryRow(e.Admin(), `SELECT array_agg(id) FROM audit_log
 WHERE entity_id=ANY(@records) AND entity_type IN ('contact','dedupe_candidate')`,
			pgx.StrictNamedArgs{"records": []ids.UUID{subject.UUID, pair}}).Scan(&audits)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) == 0 {
		t.Fatal("the fixture has no audit history")
	}
	if count := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->'trace'->>'audit_log_id'=ANY(@audits)`,
		pgx.StrictNamedArgs{"audits": uuidStrings(audits)}); count == 0 {
		t.Fatal("the real contact writes left no pending event copies")
	}
	if err := compose.ComposedEraser(e.Pool, nil, nil).EraseContact(e.Admin(), subject.UUID, "test"); err != nil {
		t.Fatal(err)
	}
	if count := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE id=ANY(@audits)`, pgx.StrictNamedArgs{"audits": audits}); count != len(audits) {
		t.Fatalf("audit identities remaining=%d, want %d", count, len(audits))
	}
	if count := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE id=ANY(@audits)
 AND (before IS NOT NULL OR after IS NOT NULL OR evidence IS DISTINCT FROM '{"redacted":true}'::jsonb)`,
		pgx.StrictNamedArgs{"audits": audits}); count != 0 {
		t.Fatalf("%d old audit images still carry payloads", count)
	}
	if count := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->'trace'->>'audit_log_id'=ANY(@audits)`,
		pgx.StrictNamedArgs{"audits": uuidStrings(audits)}); count != 0 {
		t.Fatalf("%d linked event copies remain", count)
	}
	if count := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='contact' AND entity_id=@other
 AND after::text LIKE '%Mia Schulz%'`, pgx.StrictNamedArgs{"other": other}); count == 0 {
		t.Fatal("erasure destroyed the unrelated contact's history")
	}
}

func uuidStrings(ids []ids.UUID) []string {
	result := make([]string, len(ids))
	for i, id := range ids {
		result[i] = id.String()
	}
	return result
}
