// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/magic"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	enrichedAddress  = "Example Street 1, 12345 Example City, Germany"
	enrichedImprint  = "https://undo.example.test/imprint"
	enrichedAddress2 = "Other Road 9, 54321 Other Town, Germany"
)

// enrichAddress runs the real enrichment writer on one company and answers the
// entry it recorded.
func enrichAddress(t *testing.T, e *integration.Env, id ids.UUID, address string) ids.UUID {
	t.Helper()
	if err := e.Contacts.ApplyEnrichment(e.Admin(), ids.From[ids.CompanyKind](id), contacts.ApplyColdStartProfileInput{
		SourceURL: enrichedImprint,
		Fields: []contacts.ColdStartFieldInput{{
			Field: "registered_address", Value: address,
			SourceURL: enrichedImprint, EvidenceSnippet: address, Confidence: 0.9,
		}},
	}); err != nil {
		t.Fatalf("apply the enrichment: %v", err)
	}
	return latestAuditRowID(t, e, "company", id, "update")
}

// judgeEnrichment reads the entry's Undo the way the record history page does, and holds
// the Home and Worklist receipt to the same answer.
func judgeEnrichment(t *testing.T, e *integration.Env, ctx context.Context, seam RestoreSeam, id, auditID ids.UUID) privacy.UndoabilityAnswer {
	t.Helper()
	page, err := NewUndoabilityPage(seam).ForRecord(ctx, "company", id, []ids.UUID{auditID})
	if err != nil {
		t.Fatalf("judge the entry on the history page: %v", err)
	}
	var receipt map[ids.UUID]*crmcontracts.MagicUndo
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		receipt, err = magicUndoJudge{seam: seam}.JudgeUndoPage(ctx, tx,
			[]magic.UndoSubject{{AuditID: auditID, EntityType: "company"}})
		return err
	}); err != nil {
		t.Fatalf("judge the entry on the receipt: %v", err)
	}
	answer, offered := page[auditID], receipt[auditID]
	if offered == nil || offered.Undoable != answer.Undoable ||
		(!answer.Undoable && (offered.Reason == nil || *offered.Reason != answer.Reason)) {
		t.Errorf("the receipt answers %+v where the history page answers %+v", offered, answer)
	}
	return answer
}

func addressParts(t *testing.T, e *integration.Env, id ids.UUID) string {
	t.Helper()
	return e.WsScalar(t, `SELECT concat_ws('|', address_line1, address_line2, address_postal_code,
		address_city, address_region, address_country, '') FROM company WHERE id = $1`, id)
}

func registeredAddressRow(t *testing.T, e *integration.Env, id ids.UUID) string {
	t.Helper()
	return e.WsScalar(t, `SELECT coalesce((SELECT value || ' @ ' || source FROM company_profile_field
		WHERE company_id = $1 AND field = 'registered_address'), 'none')`, id)
}

func refusalOf(t *testing.T, err error) Reason {
	t.Helper()
	refused, ok := errors.AsType[RefusedRestore](err)
	if !ok {
		t.Fatalf("the restore answered %v, want a refusal", err)
	}
	return refused.Reason
}

func TestAnEnrichedAddressIsUndoneWithTheProfileFieldBehindIt(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	id := e.SeedCompany(t, "Undo Address GmbH", nil)
	empty := addressParts(t, e, id)
	auditID := enrichAddress(t, e, id, enrichedAddress)
	if addressParts(t, e, id) == empty {
		t.Fatal("the enrichment landed no address; the test would then undo nothing")
	}

	seam := restoreSeamFor(e)
	if verdict := judgeEnrichment(t, e, ctx, seam, id, auditID); !verdict.Undoable {
		t.Fatalf("the enrichment is refused as %s (%s), want undoable", verdict.Reason, verdict.Detail)
	}
	if _, err := seam.Restore(ctx, "company", id, auditID, currentVersion(t, e, "company", id)); err != nil {
		t.Fatalf("undo the enrichment: %v", err)
	}
	if got := addressParts(t, e, id); got != empty {
		t.Errorf("address after undo = %q, want every part back to %q", got, empty)
	}
	if got := registeredAddressRow(t, e, id); got != "none" {
		t.Errorf("registered_address after undo = %q, want no row: the read put it there", got)
	}

	_, err := seam.Restore(ctx, "company", id, auditID, currentVersion(t, e, "company", id))
	if reason := refusalOf(t, err); reason != ReasonAlreadyUndone {
		t.Errorf("a second undo answered %s, want %s", reason, ReasonAlreadyUndone)
	}
}

// The second read replaced the registered_address row the first one left; its
// undo puts that earlier row back rather than removing the field.
func TestUndoingAnEnrichmentPutsBackTheProfileFieldItReplaced(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	id := e.SeedCompany(t, "Twice Read GmbH", nil)
	enrichAddress(t, e, id, enrichedAddress)
	if _, err := e.Contacts.UpdateCompany(ctx, ids.From[ids.CompanyKind](id), contacts.UpdateCompanyInput{
		Address: &crmcontracts.Address{},
	}); err != nil {
		t.Fatalf("clear the address by hand: %v", err)
	}
	earlier := registeredAddressRow(t, e, id)
	auditID := enrichAddress(t, e, id, enrichedAddress2)

	seam := restoreSeamFor(e)
	if _, err := seam.Restore(ctx, "company", id, auditID, currentVersion(t, e, "company", id)); err != nil {
		t.Fatalf("undo the second enrichment: %v", err)
	}
	if got := registeredAddressRow(t, e, id); got != earlier {
		t.Errorf("registered_address after undo = %q, want the row the read replaced, %q", got, earlier)
	}
}

func TestAnEnrichedAddressSomebodyChangedSinceIsNotUndone(t *testing.T) {
	cases := map[string]func(t *testing.T, e *integration.Env, id ids.UUID){
		"the address columns": func(t *testing.T, e *integration.Env, id ids.UUID) {
			city := "Elsewhere"
			if _, err := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](id), contacts.UpdateCompanyInput{
				Address: &crmcontracts.Address{City: &city},
			}); err != nil {
				t.Fatalf("edit the address by hand: %v", err)
			}
		},
		"the registered_address row": func(t *testing.T, e *integration.Env, id ids.UUID) {
			corrected := "Corrected Street 2, 12345 Example City"
			if _, err := e.Contacts.UpdateCompanyProfileField(e.Admin(), ids.From[ids.CompanyKind](id),
				"registered_address", contacts.ProfileFieldWriteInput{Value: &corrected}); err != nil {
				t.Fatalf("correct the registered address by hand: %v", err)
			}
		},
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.Admin()
			id := e.SeedCompany(t, "Edited Since GmbH", nil)
			auditID := enrichAddress(t, e, id, enrichedAddress)
			edit(t, e, id)

			seam := restoreSeamFor(e)
			if verdict := judgeEnrichment(t, e, ctx, seam, id, auditID); verdict.Reason != string(ReasonSuperseded) {
				t.Errorf("the entry is judged %+v, want %s", verdict, ReasonSuperseded)
			}
			_, err := seam.Restore(ctx, "company", id, auditID, currentVersion(t, e, "company", id))
			if reason := refusalOf(t, err); reason != ReasonSuperseded {
				t.Errorf("the undo answered %s, want %s", reason, ReasonSuperseded)
			}
		})
	}
}

func TestAReaderIsNotOfferedTheUndoOfAnEnrichedAddress(t *testing.T) {
	e := integration.Setup(t)
	id := e.SeedCompany(t, "Read Only GmbH", nil)
	auditID := enrichAddress(t, e, id, enrichedAddress)
	reader := e.As(e.AdminUser, nil, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  map[string]principal.ObjectGrant{"company": {Read: true}},
	})
	if verdict := judgeEnrichment(t, e, reader, restoreSeamFor(e), id, auditID); verdict.Reason != string(ReasonNotWritableByCaller) {
		t.Errorf("a reader is answered %+v, want %s", verdict, ReasonNotWritableByCaller)
	}
}

// An enrichment over a filled address writes no address column. Its entry
// carries no address and no receipt for the row it refreshed.
func TestAnEnrichmentOverAFilledAddressRecordsNoAddressToUndo(t *testing.T) {
	e := integration.Setup(t)
	id := e.SeedCompany(t, "Already Filled GmbH", nil)
	enrichAddress(t, e, id, enrichedAddress)
	if err := e.Contacts.ApplyEnrichment(e.Admin(), ids.From[ids.CompanyKind](id), contacts.ApplyColdStartProfileInput{
		SourceURL: enrichedImprint,
		Fields: []contacts.ColdStartFieldInput{
			{Field: "registered_address", Value: enrichedAddress2, SourceURL: enrichedImprint, EvidenceSnippet: enrichedAddress2, Confidence: 0.9},
			{Field: "industry", Value: "Logistics", SourceURL: enrichedImprint, EvidenceSnippet: "Logistics", Confidence: 0.9},
		},
	}); err != nil {
		t.Fatalf("apply the second enrichment: %v", err)
	}
	auditID := latestAuditRowID(t, e, "company", id, "update")
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE id = $1
		AND (after ? 'address_line1' OR evidence -> 'profile_fields' ? 'registered_address')`, auditID); n != 0 {
		t.Error("an enrichment that wrote no address recorded one, or a receipt for the registered_address row behind it")
	}
}

// Undoing the Undo brings back what the enrichment wrote: the address and the
// registered_address row both.
func TestUndoingTheUndoOfAnEnrichedAddressBringsBothBack(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	id := e.SeedCompany(t, "Undo Twice GmbH", nil)
	auditID := enrichAddress(t, e, id, enrichedAddress)
	enriched, enrichedRow := addressParts(t, e, id), registeredAddressRow(t, e, id)

	seam := restoreSeamFor(e)
	if _, err := seam.Restore(ctx, "company", id, auditID, currentVersion(t, e, "company", id)); err != nil {
		t.Fatalf("undo the enrichment: %v", err)
	}
	undo := latestAuditRowID(t, e, "company", id, "restore")
	if verdict := judgeEnrichment(t, e, ctx, seam, id, undo); !verdict.Undoable {
		t.Fatalf("the undo is refused as %s (%s), want undoable", verdict.Reason, verdict.Detail)
	}
	if _, err := seam.Restore(ctx, "company", id, undo, currentVersion(t, e, "company", id)); err != nil {
		t.Fatalf("undo the undo: %v", err)
	}
	if got := addressParts(t, e, id); got != enriched {
		t.Errorf("address after undoing the undo = %q, want %q", got, enriched)
	}
	if got := registeredAddressRow(t, e, id); got != enrichedRow {
		t.Errorf("registered_address after undoing the undo = %q, want %q", got, enrichedRow)
	}
}

// Walking back an enrichment, a hand edit and an earlier enrichment in order.
// Each Undo leaves the profile row as the earlier entry left it. The next step
// is not refused for a row that only holds a new version.
func TestEnrichedAddressesWalkBackInOrder(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	id := e.SeedCompany(t, "Walk Back GmbH", nil)
	empty := addressParts(t, e, id)
	first := enrichAddress(t, e, id, enrichedAddress)
	if _, err := e.Contacts.UpdateCompany(ctx, ids.From[ids.CompanyKind](id), contacts.UpdateCompanyInput{
		Address: &crmcontracts.Address{},
	}); err != nil {
		t.Fatalf("clear the address by hand: %v", err)
	}
	cleared := latestAuditRowID(t, e, "company", id, "update")
	second := enrichAddress(t, e, id, enrichedAddress2)

	seam := restoreSeamFor(e)
	for _, step := range []ids.UUID{second, cleared, first} {
		if _, err := seam.Restore(ctx, "company", id, step, currentVersion(t, e, "company", id)); err != nil {
			t.Fatalf("undo %s: %v", step, err)
		}
	}
	if got := addressParts(t, e, id); got != empty {
		t.Errorf("address after the walk back = %q, want %q", got, empty)
	}
	if got := registeredAddressRow(t, e, id); got != "none" {
		t.Errorf("registered_address after the walk back = %q, want no row", got)
	}
}
