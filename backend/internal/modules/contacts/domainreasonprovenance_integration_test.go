// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	contracts "github.com/margince/margince/backend/internal/contracts"
)

func TestOlderWriterCannotReuseANewerDecisionsReasonCode(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	if _, err := e.store.KeepDomainAsCompany(ctx, "changed.example"); err != nil {
		t.Fatal(err)
	}
	e.inTx(ctx, t, "legacy decision replaces coded decision", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE company_domain_disposition
 SET admission = 'suppressed', admission_reason = 'Vendor, not a prospect',
 admission_at = admission_at + interval '1 second'
 WHERE domain = 'changed.example'`)
		return err
	})
	got := e.listedByDomain(ctx, t)["changed.example"]
	if got.ReasonCode != contracts.BlockedDomainReasonCodeStated || got.Note == nil || *got.Note != "Vendor, not a prospect" {
		t.Fatalf("legacy decision inherited code: %+v", got)
	}
	e.inTx(ctx, t, "new bulk refusal", func(tx pgx.Tx) error {
		return e.store.SuppressBulkSenderDomainTx(ctx, tx, "bulkchange.example", "newsletter")
	})
	e.inTx(ctx, t, "legacy sender kind replaces coded kind", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE company_domain_disposition SET admission_reason = @reason,
 admission_at = admission_at + interval '1 second' WHERE domain = @domain`, pgx.NamedArgs{
			"reason": bulkSenderReason("transactional"), "domain": "bulkchange.example",
		})
		return err
	})
	got = e.listedByDomain(ctx, t)["bulkchange.example"]
	if got.SenderKind == nil || string(*got.SenderKind) != "transactional" {
		t.Fatalf("stale sender kind: %+v", got)
	}
}

func TestHumanAdmissionKeepsQuotedWordsAndClearsMachineMetadata(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.inTx(ctx, t, "bulk refusal", func(tx pgx.Tx) error {
		return e.store.SuppressBulkSenderDomainTx(ctx, tx, "quoted.example", "newsletter")
	})
	row, err := e.store.SetDomainAdmission(ctx, "quoted.example", DomainSuppressed, keptDomainReason)
	if err != nil {
		t.Fatal(err)
	}
	got := ToContractBlockedDomain(row)
	if got.ReasonCode != contracts.BlockedDomainReasonCodeStated || got.Note == nil || *got.Note != keptDomainReason || got.SenderKind != nil {
		t.Fatalf("human words interpreted as machine code: %+v", got)
	}
	e.inTx(ctx, t, "machine cannot replace a human decision", func(tx pgx.Tx) error {
		return e.store.SuppressBulkSenderDomainTx(ctx, tx, "quoted.example", "transactional")
	})
	listed := e.listedByDomain(ctx, t)["quoted.example"]
	if listed.ReasonCode != got.ReasonCode || listed.Note == nil || *listed.Note != *got.Note {
		t.Fatalf("human code overwritten: %+v", listed)
	}
	e.inTx(ctx, t, "machine metadata cleared", func(tx pgx.Tx) error {
		var cleared bool
		if err := tx.QueryRow(ctx, `SELECT admission_sender_kind IS NULL FROM company_domain_disposition WHERE domain='quoted.example'`).Scan(&cleared); err != nil {
			return err
		}
		if !cleared {
			t.Error("human decision retained machine sender metadata")
		}
		return nil
	})
}

func TestAdmissionAuditExcludesCodesFromAReplacedLegacyDecision(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	row, err := e.store.KeepDomainAsCompany(ctx, "auditcode.example")
	if err != nil {
		t.Fatal(err)
	}
	e.inTx(ctx, t, "legacy overwrite", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE company_domain_disposition SET admission='suppressed',
 admission_reason='Vendor', admission_at=admission_at + interval '1 second'
 WHERE domain='auditcode.example'`)
		return err
	})
	if _, err := e.store.SetDomainAdmission(ctx, "auditcode.example", DomainSuppressed, "Still a vendor"); err != nil {
		t.Fatal(err)
	}
	var before map[string]any
	if err := json.Unmarshal(admissionBeforeImage(ctx, t, e, row.ID, 2), &before); err != nil {
		t.Fatal(err)
	}
	wantImage(t, before, "before", "admission_reason", "Vendor")
	wantImage(t, before, "before", "admission_reason_code", "")
	wantImage(t, before, "before", "admission_sender_kind", "")
}
