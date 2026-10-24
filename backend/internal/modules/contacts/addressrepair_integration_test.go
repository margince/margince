// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const legacyAddress = "Example Street 4\n12345 Example City\nGermany"

var legacyAddressParts = [6]string{"Example Street 4", "<nil>", "<nil>", "12345", "Example City", "DE"}

// legacyAddressCompany lands a company the way a read before the splitter
// did. The profile text goes through today's writer. The columns are then set
// as the old fill statement left them, with the whole text in line 1.
func legacyAddressCompany(t *testing.T, e *dedupeEnv, host, text string) ids.CompanyID {
	t.Helper()
	ctx := e.as()
	source := "https://" + host + ".example.test/imprint"
	field := ColdStartFieldInput{Field: fieldRegisteredAddress, Value: text, EvidenceSnippet: "Registered office: " + text, SourceURL: source, Confidence: 0.9}
	id, err := e.store.ApplyColdStartProfile(ctx, ApplyColdStartProfileInput{SourceURL: source, Fields: []ColdStartFieldInput{field}})
	if err != nil {
		t.Fatalf("applying the read: %v", err)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE company SET address_line1 = $2, address_line2 = NULL, address_region = NULL,
		       address_postal_code = NULL, address_city = NULL, address_country = NULL WHERE id = $1`, id, text)
		return err
	}); err != nil {
		t.Fatalf("storing the address as the old writer did (test setup): %v", err)
	}
	return id
}

func repairAllAddresses(t *testing.T, e *dedupeEnv, limit int) (scanned, changed int) {
	t.Helper()
	system := principal.SystemActing(e.as(), "system:test")
	after := ids.Nil
	for {
		var page AddressRepairPage
		if err := e.store.tx(system, func(tx pgx.Tx) error {
			var err error
			page, err = RepairUnsplitCompanyAddressesTx(system, tx, after, limit)
			return err
		}); err != nil {
			t.Fatalf("repair: %v", err)
		}
		if len(page.Scanned) > limit {
			t.Fatalf("a page inspected %d records, past its limit %d", len(page.Scanned), limit)
		}
		if len(page.Scanned) == 0 {
			return scanned, changed
		}
		scanned += len(page.Scanned)
		changed += page.Changed
		after = page.Scanned[len(page.Scanned)-1]
	}
}

func companyUpdateEvents(t *testing.T, e *dedupeEnv, id ids.CompanyID) (audits, events int) {
	t.Helper()
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT
		        (SELECT count(*) FROM audit_log WHERE entity_type = 'company' AND entity_id = $1 AND action = 'update'),
		        (SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id' = $1::text AND envelope->>'type' = 'company.updated')`,
			id).Scan(&audits, &events)
	}); err != nil {
		t.Fatal(err)
	}
	return audits, events
}

func TestTheAddressRepairSplitsAnOldLineOnceWithAnAuditAndAnEvent(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	legacy := legacyAddressCompany(t, e, "legacy", legacyAddress)
	refused := legacyAddressCompany(t, e, "refused", unsplitAddress)
	auditsBefore, eventsBefore := companyUpdateEvents(t, e, legacy)

	scanned, changed := repairAllAddresses(t, e, 1)
	if scanned < 2 || changed != 1 {
		t.Errorf("scanned %d, changed %d, want every company inspected and one split", scanned, changed)
	}
	if got := storedAddressParts(ctx, t, e.store, legacy); got != legacyAddressParts {
		t.Errorf("address = %q, want %q", got, legacyAddressParts)
	}
	if got := storedAddressParts(ctx, t, e.store, refused); got != [6]string{unsplitAddress, "<nil>", "<nil>", "<nil>", "<nil>", "<nil>"} {
		t.Errorf("an address the splitter refuses = %q, want it whole in line 1", got)
	}
	audits, events := companyUpdateEvents(t, e, legacy)
	if audits != auditsBefore+1 || events != eventsBefore+1 {
		t.Errorf("audits %d→%d, events %d→%d, want one more of each", auditsBefore, audits, eventsBefore, events)
	}
	if _, again := repairAllAddresses(t, e, 10); again != 0 {
		t.Errorf("a second walk split %d addresses, want 0", again)
	}
	if audits, _ := companyUpdateEvents(t, e, legacy); audits != auditsBefore+1 {
		t.Errorf("a second walk wrote audit rows: %d, want %d", audits, auditsBefore+1)
	}
}

func TestTheAddressRepairLeavesAnAddressSomebodyTypedOnTheRecord(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	retyped := legacyAddressCompany(t, e, "retyped", legacyAddress)
	restored := legacyAddressCompany(t, e, "restored", legacyAddress)
	other, same := "Other Street 9, 54321 Other Town", legacyAddress
	// The second company's line goes back to the profile text by hand, which
	// only the record's edit history can tell from a line nobody touched.
	for _, edit := range []struct {
		id   ids.CompanyID
		line *string
	}{{retyped, &other}, {restored, &other}, {restored, &same}} {
		if _, err := e.store.UpdateCompany(ctx, edit.id, UpdateCompanyInput{Address: &crmcontracts.Address{Line1: edit.line}}); err != nil {
			t.Fatalf("editing the address: %v", err)
		}
	}
	// A line typed on create has no profile text it was copied from.
	created, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Typed On Create GmbH", Source: "manual", Address: &crmcontracts.Address{Line1: &same}})
	if err != nil {
		t.Fatalf("creating the company: %v", err)
	}
	if _, changed := repairAllAddresses(t, e, 50); changed != 0 {
		t.Errorf("the repair split %d addresses, want every typed one left alone", changed)
	}
	if got := storedAddressParts(ctx, t, e.store, ids.From[ids.CompanyKind](ids.UUID(created.Id))); got[0] != legacyAddress || got[4] != "<nil>" {
		t.Errorf("address typed on create = %q, want it whole", got)
	}
	if got := storedAddressParts(ctx, t, e.store, retyped); got[0] != other || got[4] != "<nil>" {
		t.Errorf("retyped address = %q, want the typed line as it was", got)
	}
	if got := storedAddressParts(ctx, t, e.store, restored); got[0] != legacyAddress || got[4] != "<nil>" {
		t.Errorf("restored address = %q, want the line somebody saved, still whole", got)
	}
}

func TestTheAddressRepairNeverReplacesAStoredPart(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contradicted := legacyAddressCompany(t, e, "contradicted", legacyAddress)
	kept := legacyAddressCompany(t, e, "kept", "Example Street 4, 12345 Example City")
	// A road named by number reads as a postal code unless a house number or a
	// separator sets the street apart.
	highway := legacyAddressCompany(t, e, "highway", "Highway 1234 Dallas")
	// The old replacement wrote line 1 alone, beside a city already stored.
	beside := legacyAddressCompany(t, e, "beside", legacyAddress)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE company SET address_city = 'Old Town' WHERE id = $1`, beside)
		return err
	}); err != nil {
		t.Fatalf("storing the city (test setup): %v", err)
	}
	for id, country := range map[ids.CompanyID]string{contradicted: "CA", kept: "AT"} {
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE company SET address_country = $2 WHERE id = $1`, id, country)
			return err
		}); err != nil {
			t.Fatalf("storing the country (test setup): %v", err)
		}
	}
	repairAllAddresses(t, e, 50)
	if got := storedAddressParts(ctx, t, e.store, contradicted); got[0] != legacyAddress || got[5] != "CA" {
		t.Errorf("contradicted address = %q, want it whole with CA", got)
	}
	if got := storedAddressParts(ctx, t, e.store, kept); got != [6]string{"Example Street 4", "<nil>", "<nil>", "12345", "Example City", "AT"} {
		t.Errorf("address = %q, want it split with the stored country", got)
	}
	if got := storedAddressParts(ctx, t, e.store, beside); got[0] != legacyAddress || got[4] != "Old Town" {
		t.Errorf("address beside a stored city = %q, want both kept", got)
	}
	if got := storedAddressParts(ctx, t, e.store, highway); got[0] != "Highway 1234 Dallas" || got[3] != "<nil>" {
		t.Errorf("a run-on line with no house number = %q, want it whole", got)
	}
}

func TestTheAddressRepairEstimateCountsWhatTheWalkInspects(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	for _, host := range []string{"one", "two", "three"} {
		legacyAddressCompany(t, e, host, legacyAddress)
	}
	var capped, whole int64
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		if capped, err = CountCompanyAddressRepairRows(ctx, tx, 1); err != nil {
			return err
		}
		whole, err = CountCompanyAddressRepairRows(ctx, tx, 1000)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	scanned, _ := repairAllAddresses(t, e, 2)
	if capped != 2 || whole != int64(scanned) {
		t.Errorf("estimate capped at 1 = %d, want 2; uncapped = %d, want the %d records the walk inspected", capped, whole, scanned)
	}
}

func TestACardsAddressLandsInItsParts(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	cards, err := ParseVCards(strings.NewReader("BEGIN:VCARD\nFN:Parts Contact\nEMAIL:parts@split.example.test\nADR;TYPE=WORK:;;Example Street 4;Example City;;12345;Germany\nEND:VCARD\n"))
	if err != nil {
		t.Fatal(err)
	}
	results, err := e.store.ImportVCards(ctx, cards)
	if err != nil || len(results) != 1 || results[0].ContactID == nil {
		t.Fatalf("importing the card: %+v, %v", results, err)
	}
	got, err := e.store.GetContact(ctx, *results[0].ContactID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if parts := addressPartsOf(addressColumns(got.Address)); parts != legacyAddressParts {
		t.Errorf("contact address = %q, want %q", parts, legacyAddressParts)
	}
}
