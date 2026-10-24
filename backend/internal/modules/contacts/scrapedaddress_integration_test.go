// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	siteAddress      = "Example Street 1, Tower B, 12345 Example City, Germany"
	siteAddressURL   = "https://split-address.example.test/imprint"
	unsplitAddress   = "Website Road 2"
	siteAddressQuote = "Registered office: " + siteAddress
)

var siteAddressParts = [6]string{"Example Street 1", "Tower B", "<nil>", "12345", "Example City", "DE"}

func addressField(value string) ColdStartFieldInput {
	return ColdStartFieldInput{Field: fieldRegisteredAddress, Value: value, EvidenceSnippet: siteAddressQuote, SourceURL: siteAddressURL, Confidence: 0.9}
}

// countGeocodes wires a spy for the in-transaction geocode enqueue and
// returns the count it keeps.
func countGeocodes(s *Store) *int {
	queued := new(int)
	s.WithGeocodeEnqueue(func(context.Context, pgx.Tx, ids.CompanyID) error { *queued++; return nil })
	return queued
}

func storedAddressParts(ctx context.Context, t *testing.T, s *Store, id ids.CompanyID) [6]string {
	t.Helper()
	company, err := s.GetCompany(ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the company back: %v", err)
	}
	return addressPartsOf(addressColumns(company.Address))
}

func storedProfileValue(ctx context.Context, t *testing.T, s *Store, id ids.CompanyID, field string) string {
	t.Helper()
	fields, err := s.ListCompanyProfileFields(ctx, id)
	if err != nil {
		t.Fatalf("listing profile fields: %v", err)
	}
	for _, f := range fields {
		if string(f.Field) == field {
			return f.Value
		}
	}
	return ""
}

func TestASiteReadSplitsTheAddressItCreatesACompanyWith(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	queued := countGeocodes(e.store)
	id, err := e.store.ApplyColdStartProfile(ctx, ApplyColdStartProfileInput{SourceURL: siteAddressURL, Fields: []ColdStartFieldInput{addressField(siteAddress)}})
	if err != nil {
		t.Fatalf("applying the read: %v", err)
	}
	if got := storedAddressParts(ctx, t, e.store, id); got != siteAddressParts {
		t.Fatalf("address columns = %q, want %q", got, siteAddressParts)
	}
	if *queued != 1 {
		t.Fatalf("the read queued %d geocode lookups, want 1 for the address it wrote", *queued)
	}
	if got := storedProfileValue(ctx, t, e.store, id, fieldRegisteredAddress); got != siteAddress {
		t.Fatalf("registered_address profile value = %q, want the whole text %q", got, siteAddress)
	}
	var line string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT after->>'registered_address' FROM audit_log
			WHERE entity_type = 'company' AND entity_id = $1 AND action = 'create'`, id).Scan(&line)
	}); err != nil {
		t.Fatalf("reading the create audit row: %v", err)
	}
	if want := "Example Street 1, Tower B, 12345 Example City, DE"; line != want {
		t.Fatalf("audit after registered_address = %q, want the six columns as one line %q", line, want)
	}
}

// Each populated part must protect the whole address from automatic fills.
func TestASiteReadLeavesAnAddressSomebodyStartedAlone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	protected := map[string]bool{}
	for _, held := range []crmcontracts.Address{
		{Line1: new("Human Street 9")},
		{Line2: new("Back office")},
		{Region: new("Bavaria")},
		{PostalCode: new("80331")},
		{City: new("Munich")},
		{Country: new("AT")},
	} {
		for _, part := range addressColumnParts(held) {
			if part.value != "" {
				protected[part.column] = true
			}
		}
		company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Held " + formatAddressLine(held), Source: "manual", Address: &held})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.From[ids.CompanyKind](ids.UUID(company.Id))
		queued := countGeocodes(e.store)
		if err := e.store.ApplyEnrichment(ctx, id, ApplyColdStartProfileInput{SourceURL: siteAddressURL, Fields: []ColdStartFieldInput{addressField(siteAddress)}}); err != nil {
			t.Fatalf("enriching a company holding %s: %v", formatAddressLine(held), err)
		}
		if got, want := storedAddressParts(ctx, t, e.store, id), addressPartsOf(held); got != want {
			t.Errorf("a company holding only %s now holds %q", formatAddressLine(held), got)
		}
		if *queued != 0 {
			t.Errorf("a read that wrote no address queued %d lookups for %s", *queued, formatAddressLine(held))
		}
	}
	if len(protected) != len(companyAddressColumns) {
		t.Fatalf("fill protection covers %d address columns, want %d", len(protected), len(companyAddressColumns))
	}
}

func TestASiteReadWritesNoAddressItCannotSplit(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Unsplit Glass", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	queued := countGeocodes(e.store)
	if err := e.store.ApplyEnrichment(ctx, id, ApplyColdStartProfileInput{SourceURL: siteAddressURL, Fields: []ColdStartFieldInput{addressField(unsplitAddress)}}); err != nil {
		t.Fatal(err)
	}
	if got := storedAddressParts(ctx, t, e.store, id); got != addressPartsOf(crmcontracts.Address{}) {
		t.Fatalf("an address the split refused landed as %q", got)
	}
	if *queued != 0 {
		t.Fatalf("an unwritten address queued %d lookups", *queued)
	}
	if got := storedProfileValue(ctx, t, e.store, id, fieldRegisteredAddress); got != unsplitAddress {
		t.Fatalf("registered_address profile value = %q, want the text the read found", got)
	}
}

func TestEveryAutomatedReadSplitsTheAddressAndQueuesALookup(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	for name, apply := range map[string]func(ids.CompanyID) error{
		"enrichment": func(id ids.CompanyID) error {
			return e.store.ApplyEnrichment(ctx, id, ApplyColdStartProfileInput{SourceURL: siteAddressURL, Fields: []ColdStartFieldInput{addressField(siteAddress)}})
		},
		"deep read": func(id ids.CompanyID) error {
			return e.store.ApplyDeepRead(ctx, DeepReadProposal{CompanyID: id, SourceURL: siteAddressURL, Fields: []DeepReadField{
				{Field: fieldRegisteredAddress, Value: siteAddress, EvidenceSnippet: siteAddressQuote, SourceURL: siteAddressURL, Confidence: 0.9},
			}})
		},
	} {
		company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Read by " + name, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.From[ids.CompanyKind](ids.UUID(company.Id))
		queued := countGeocodes(e.store)
		if err := apply(id); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := storedAddressParts(ctx, t, e.store, id); got != siteAddressParts {
			t.Errorf("%s: address columns = %q, want %q", name, got, siteAddressParts)
		}
		if *queued != 1 {
			t.Errorf("%s queued %d lookups, want 1", name, *queued)
		}
	}
}

func TestTheReadPreviewShowsTheAddressPartsTheWriterWouldFill(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	empty, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Preview Empty", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	held, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Preview Held", Source: "manual",
		Address: &crmcontracts.Address{Line1: new("Human Street 9"), PostalCode: new("80331"), City: new("Munich")},
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal := func(id ids.UUID) DeepReadProposal {
		return DeepReadProposal{CompanyID: ids.From[ids.CompanyKind](id), SourceURL: siteAddressURL, Fields: []DeepReadField{
			{Field: fieldRegisteredAddress, Value: siteAddress, EvidenceSnippet: siteAddressQuote, SourceURL: siteAddressURL, Confidence: 0.9},
		}}
	}
	plan, err := e.store.PreviewCompanyRead(ctx, proposal(ids.UUID(empty.Id)))
	if err != nil {
		t.Fatal(err)
	}
	filled := map[string]string{}
	for _, column := range plan.Columns {
		if column.Retained || column.Before != "" {
			t.Errorf("empty company: %s previewed as kept %q", column.Field, column.Before)
		}
		filled[column.Field] = column.After
	}
	want := map[string]string{"address_line1": "Example Street 1", "address_line2": "Tower B", "address_postal_code": "12345", "address_city": "Example City", "address_country": "DE"}
	if len(filled) != len(want) {
		t.Fatalf("empty company preview = %v, want %v", filled, want)
	}
	for column, value := range want {
		if filled[column] != value {
			t.Errorf("empty company preview %s = %q, want %q", column, filled[column], value)
		}
	}

	plan, err = e.store.PreviewCompanyRead(ctx, proposal(ids.UUID(held.Id)))
	if err != nil {
		t.Fatal(err)
	}
	kept := map[string]string{}
	for _, column := range plan.Columns {
		if !column.Retained || column.After != column.Before {
			t.Errorf("held company: %s previewed as %q -> %q, want the stored part kept", column.Field, column.Before, column.After)
		}
		kept[column.Field] = column.Before
	}
	if kept["address_line1"] != "Human Street 9" || kept["address_postal_code"] != "80331" || kept["address_city"] != "Munich" {
		t.Fatalf("held company preview shows %v as the current address, want the stored parts", kept)
	}
}

func TestTheCompanyFormSplitsTheAddressItIsGiven(t *testing.T) {
	a := newAnchorEnv(t)
	queued := countGeocodes(a.store)
	save := func(address string) {
		t.Helper()
		if _, err := a.store.SaveCompany(a.ctx, SaveCompanyInput{DisplayName: "Our Company", Fields: map[string]*string{fieldRegisteredAddress: &address}}); err != nil {
			t.Fatalf("saving the address %q: %v", address, err)
		}
	}
	save(siteAddress)
	if got := storedAddressParts(a.ctx, t, a.store, a.anchorID); got != siteAddressParts {
		t.Fatalf("form address columns = %q, want %q", got, siteAddressParts)
	}
	if *queued != 1 {
		t.Fatalf("the form queued %d lookups, want 1", *queued)
	}

	// The form sends the same line back on every save, so a street edited on
	// the record since then stays.
	if _, err := a.store.UpdateCompany(a.ctx, a.anchorID, UpdateCompanyInput{Address: &crmcontracts.Address{
		Line1: new("Example Street 3"), PostalCode: new("12345"), City: new("Example City"), Country: new("DE"),
	}}); err != nil {
		t.Fatalf("editing the address on the record: %v", err)
	}
	save(siteAddress)
	if got := storedAddressParts(a.ctx, t, a.store, a.anchorID); got[0] != "Example Street 3" || got[1] != "<nil>" {
		t.Fatalf("an unchanged form line overwrote the edited address: %q", got)
	}

	save(unsplitAddress)
	if got := storedAddressParts(a.ctx, t, a.store, a.anchorID); got != addressPartsOf(crmcontracts.Address{}) {
		t.Fatalf("a line the split refuses left %q beside it", got)
	}
	if got := storedProfileValue(a.ctx, t, a.store, a.anchorID, fieldRegisteredAddress); got != unsplitAddress {
		t.Fatalf("registered_address profile value = %q, want %q", got, unsplitAddress)
	}

	save(siteAddress)
	save("")
	if got := storedAddressParts(a.ctx, t, a.store, a.anchorID); got != addressPartsOf(crmcontracts.Address{}) {
		t.Fatalf("an emptied address left %q", got)
	}
}

func TestOnlyAnAdministratorSavesTheCompanyAddress(t *testing.T) {
	a := newAnchorEnv(t)
	actor, ok := principal.Actor(a.ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	actor.Permissions.RoleKeys = []string{"rep"}
	rep := principal.WithActor(a.ctx, actor)
	address := siteAddress
	_, err := a.store.SaveCompany(rep, SaveCompanyInput{DisplayName: "Our Company", Fields: map[string]*string{fieldRegisteredAddress: &address}})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a rep saving the company address = %v, want a permission refusal", err)
	}
	if got := storedAddressParts(a.ctx, t, a.store, a.anchorID); got != addressPartsOf(crmcontracts.Address{}) {
		t.Fatalf("a refused save wrote %q", got)
	}
}

func TestCompanyAddressPreviewKeepsObjectRowAndFieldScope(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Private Preview", Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.rep))})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	if _, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{Visibility: new("owner"), IfVersion: company.Version}); err != nil {
		t.Fatal(err)
	}
	proposal := DeepReadProposal{CompanyID: id, SourceURL: siteAddressURL, Fields: []DeepReadField{
		{Field: fieldRegisteredAddress, Value: siteAddress, EvidenceSnippet: siteAddressQuote, SourceURL: siteAddressURL, Confidence: 0.9},
	}}
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing fixture actor")
	}
	actor.Permissions.RowScope = principal.RowScopeOwn
	allowed := principal.WithActor(ctx, actor)
	plan, err := e.store.PreviewCompanyRead(allowed, proposal)
	if err != nil || len(plan.Columns) != 5 {
		t.Fatalf("owner preview = %+v, %v", plan, err)
	}
	noRead := actor
	noRead.Permissions.Objects = map[string]principal.ObjectGrant{companyEntity: {Update: true}}
	masked := actor
	masked.Permissions.FieldMasks = []principal.FieldMask{{Object: companyEntity, Field: "address_city", Condition: principal.MaskAlways}}
	for name, tc := range map[string]struct {
		ctx  context.Context
		want error
	}{
		"no object grant":                 {principal.WithActor(ctx, noRead), apperrors.ErrPermissionDenied},
		"another owner's private company": {e.asOwnScoped(e.otherRep), apperrors.ErrNotFound},
		"a withheld address part":         {principal.WithActor(ctx, masked), apperrors.ErrPermissionDenied},
	} {
		t.Run(name, func(t *testing.T) {
			preview, err := e.store.PreviewCompanyRead(tc.ctx, proposal)
			if !errors.Is(err, tc.want) || preview.Company.DisplayName != "" || len(preview.Columns)+len(preview.Profiles)+len(preview.Facts) != 0 {
				t.Fatalf("refused preview disclosed a company: %+v, %v; want %v", preview, err, tc.want)
			}
		})
	}
}
