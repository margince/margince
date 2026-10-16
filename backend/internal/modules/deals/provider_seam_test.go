// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The record seam's refusals that come before any store call. A refusal
// here must name the field the MCP door's caller has to fix.

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// seamProvider has no database: every case below must refuse before a query.
func seamProvider() *Provider { return NewProvider(nil, Installation{}) }

// readerOnly holds read on every catalog and offer object, and no delete.
func readerOnly() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	user := ids.NewV7()
	read := principal.ObjectGrant{Read: true}
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"custom"}, RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{
				"deal": read, "product": read, "offer_template": read, "offer": read,
			},
		},
	})
}

func TestTheSeamRefusesATypeItDoesNotServeOnEveryVerb(t *testing.T) {
	p, ctx := seamProvider(), context.Background()
	contact := datasource.EntityRef{Type: datasource.EntityContact, ID: ids.NewV7()}
	verbs := map[string]func() error{
		"read":   func() error { _, err := p.Read(ctx, contact); return err },
		"search": func() error { _, _, _, err := p.SearchEntity(ctx, contact.Type, nil, 10, nil, nil); return err },
		"create": func() error {
			_, err := p.Create(ctx, datasource.CreateInput{EntityType: contact.Type, Fields: json.RawMessage(`{}`)})
			return err
		},
		"update": func() error {
			_, err := p.Update(ctx, datasource.UpdateInput{Ref: contact, Patch: json.RawMessage(`{}`)})
			return err
		},
		"refuse archive": func() error { return p.RefuseArchive(ctx, contact) },
		"archive":        func() error { _, err := p.Archive(ctx, contact); return err },
	}
	for verb, call := range verbs {
		var unsupported *datasource.UnsupportedEntityError
		if err := call(); !errors.As(err, &unsupported) {
			t.Errorf("%s of a contact answered %v, want the unsupported-type refusal", verb, err)
		}
	}
}

// Every type ArchivableTypes names must reach its own authority probe. A type
// the archive switch did not serve would answer the unsupported-type refusal.
func TestEveryArchivableTypeReachesItsDeleteGrant(t *testing.T) {
	p := seamProvider()
	types, err := p.ArchivableTypes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []datasource.EntityType{
		datasource.EntityDeal, datasource.EntityProduct, datasource.EntityOfferTemplate, datasource.EntityOffer,
	}
	if !slices.Equal(types, want) {
		t.Fatalf("archivable types are %v, want %v", types, want)
	}
	for _, typ := range types {
		err := p.RefuseArchive(readerOnly(), datasource.EntityRef{Type: typ, ID: ids.NewV7()})
		if !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("archiving a %s without its delete grant answered %v, want permission denied", typ, err)
		}
	}
}

func TestAnOfferListNeedsItsDealAndNoText(t *testing.T) {
	p, text := seamProvider(), "renewal"
	for name, tc := range map[string]struct {
		typ     datasource.EntityType
		text    *string
		filters map[string]string
		field   string
	}{
		"offers searched by text":    {datasource.EntityOffer, &text, nil, "q"},
		"offers with no deal":        {datasource.EntityOffer, nil, map[string]string{"status": "draft"}, "deal_id"},
		"templates searched by text": {datasource.EntityOfferTemplate, &text, nil, "q"},
	} {
		_, _, _, err := p.SearchEntity(context.Background(), tc.typ, tc.text, 10, nil, tc.filters)
		if field, _ := refusedOn(t, err); field != tc.field {
			t.Errorf("%s was refused on %q, want %q", name, field, tc.field)
		}
	}
}

func TestACatalogOrOfferListRefusesAFilterItCannotApply(t *testing.T) {
	p := seamProvider()
	for _, typ := range []datasource.EntityType{
		datasource.EntityProduct, datasource.EntityOfferTemplate, datasource.EntityOffer,
	} {
		if _, _, _, err := p.SearchEntity(context.Background(), typ, nil, 10, nil,
			map[string]string{"colour": "red"}); err == nil {
			t.Errorf("a %s list accepted a filter it has no binding for", typ)
		}
	}
}

func TestAnOfferCreateOverTheSeamNamesItsDeal(t *testing.T) {
	p := seamProvider()
	for name, tc := range map[string]struct{ fields, field, code string }{
		"fields not an object": {`[1]`, "fields", "malformed_json"},
		"no deal named":        {`{"currency":"EUR"}`, "deal_id", "required"},
		"a deal of zeros":      {`{"deal_id":"00000000-0000-0000-0000-000000000000"}`, "deal_id", "required"},
		"a smuggled total": {
			`{"deal_id":"` + ids.NewV7().String() + `","currency":"EUR","net_minor":1}`,
			"net_minor", "totals_derived",
		},
	} {
		_, err := p.Create(context.Background(), datasource.CreateInput{
			EntityType: datasource.EntityOffer, Fields: json.RawMessage(tc.fields), Source: "manual",
		})
		if field, code := refusedOn(t, err); field != tc.field || code != tc.code {
			t.Errorf("%s was refused on %s/%s, want %s/%s", name, field, code, tc.field, tc.code)
		}
	}
}

func TestACatalogCreateHoldsTheRulesTheRESTDoorHolds(t *testing.T) {
	p := seamProvider()
	for name, tc := range map[string]struct {
		typ         datasource.EntityType
		fields      string
		field, code string
	}{
		"a product with no price": {
			datasource.EntityProduct,
			`{"name":"Seat","currency":"EUR"}`, "unit_price_minor", "required",
		},
		"a product with a null price": {
			datasource.EntityProduct,
			`{"name":"Seat","currency":"EUR","unit_price_minor":null}`, "unit_price_minor", "required",
		},
		"a template with no layout": {
			datasource.EntityOfferTemplate,
			`{"name":"Standard"}`, "layout", "required",
		},
		"a template with a blank name": {
			datasource.EntityOfferTemplate,
			`{"name":" ","layout":{}}`, "name", "required",
		},
	} {
		_, err := p.Create(context.Background(), datasource.CreateInput{
			EntityType: tc.typ, Fields: json.RawMessage(tc.fields), Source: "manual",
		})
		if field, code := refusedOn(t, err); field != tc.field || code != tc.code {
			t.Errorf("%s was refused on %s/%s, want %s/%s", name, field, code, tc.field, tc.code)
		}
	}
}

// The seam stamps the source a product carries, and a blank one is refused
// as the REST door refuses it.
func TestAProductCreateNeedsASource(t *testing.T) {
	_, err := seamProvider().Create(context.Background(), datasource.CreateInput{
		EntityType: datasource.EntityProduct, Fields: json.RawMessage(`{"name":"Seat","currency":"EUR","unit_price_minor":1}`),
	})
	if field, code := refusedOn(t, err); field != "source" || code != "required" {
		t.Errorf("a product with no source was refused on %s/%s, want source/required", field, code)
	}
}

// A template's contract carries no extension map, so a misspelt key cannot
// ride along unread.
func TestATemplateRefusesAFieldItsContractDoesNotName(t *testing.T) {
	p, ctx := seamProvider(), context.Background()
	typ := datasource.EntityOfferTemplate
	var decode *datasource.FieldDecodeError
	_, err := p.Create(ctx, datasource.CreateInput{EntityType: typ, Fields: json.RawMessage(`{"title":"x"}`)})
	if !errors.As(err, &decode) {
		t.Errorf("a template create naming an unknown title answered %v, want a decode refusal", err)
	}
	_, err = p.Update(ctx, datasource.UpdateInput{
		Ref: datasource.EntityRef{Type: typ, ID: ids.NewV7()}, Patch: json.RawMessage(`{"title":"x"}`),
	})
	if !errors.As(err, &decode) {
		t.Errorf("a template patch naming an unknown title answered %v, want a decode refusal", err)
	}
}

func TestACatalogOrOfferPatchHoldsTheRulesTheRESTDoorHolds(t *testing.T) {
	p := seamProvider()
	for name, tc := range map[string]struct {
		typ         datasource.EntityType
		patch       string
		clear       []string
		field, code string
	}{
		"a product's name cleared":  {datasource.EntityProduct, `{}`, []string{"name"}, "name", "required"},
		"a template with no layout": {datasource.EntityOfferTemplate, `{"name":"Standard","locale":"de-DE"}`, nil, "layout", "required"},
		"a template blanked":        {datasource.EntityOfferTemplate, `{"name":"","layout":{}}`, nil, "name", "required"},
		"an offer naming a total":   {datasource.EntityOffer, `{"total_minor":5}`, nil, "total_minor", "totals_derived"},
	} {
		_, err := p.Update(context.Background(), datasource.UpdateInput{
			Ref: datasource.EntityRef{Type: tc.typ, ID: ids.NewV7()}, Patch: json.RawMessage(tc.patch), Clear: tc.clear,
		})
		if field, code := refusedOn(t, err); field != tc.field || code != tc.code {
			t.Errorf("%s was refused on %s/%s, want %s/%s", name, field, code, tc.field, tc.code)
		}
	}
}

func TestOfferLineVerbsRefuseATotalBeforeTheStore(t *testing.T) {
	p, offer, line := seamProvider(), ids.NewV7(), ids.NewV7()
	if _, err := p.AddOfferLine(context.Background(), offer, json.RawMessage(`{"quantity":1,"gross_minor":9}`)); err == nil {
		t.Error("adding a line that names gross_minor was accepted")
	}
	if _, err := p.UpdateOfferLine(context.Background(), offer, line, json.RawMessage(`{"line_tax":9}`)); err == nil {
		t.Error("a line patch naming line_tax was accepted")
	}
}

func TestAPriceIsStatedOnlyByAValue(t *testing.T) {
	for body, want := range map[string]bool{
		`{"unit_price_minor":0}`:    true,
		`{"unit_price_minor":null}`: false,
		`{"name":"Seat"}`:           false,
		`not json`:                  false,
	} {
		if got := statesAValue(json.RawMessage(body), "unit_price_minor"); got != want {
			t.Errorf("statesAValue(%s) = %v, want %v", body, got, want)
		}
	}
}

// noCatalogGrant holds deal read and nothing on the catalog or offers.
func noCatalogGrant() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	user := ids.NewV7()
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"custom"}, RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{"deal": {Read: true}},
		},
	})
}

func TestTheSeamReadsAndListsOnlyUnderTheReadGrant(t *testing.T) {
	p, ctx := seamProvider(), noCatalogGrant()
	for _, typ := range []datasource.EntityType{
		datasource.EntityProduct, datasource.EntityOfferTemplate, datasource.EntityOffer,
	} {
		if _, err := p.Read(ctx, datasource.EntityRef{Type: typ, ID: ids.NewV7()}); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("reading a %s without its read grant answered %v, want permission denied", typ, err)
		}
		filters := map[string]string{}
		if typ == datasource.EntityOffer {
			filters["deal_id"] = ids.NewV7().String()
		}
		if _, _, _, err := p.SearchEntity(ctx, typ, nil, 10, nil, filters); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("listing %s without its read grant answered %v, want permission denied", typ, err)
		}
	}
}

func TestAFieldOfTheWrongShapeIsRefusedOnTheSeam(t *testing.T) {
	p, ctx := seamProvider(), context.Background()
	var decode *datasource.FieldDecodeError
	_, err := p.Create(ctx, datasource.CreateInput{
		EntityType: datasource.EntityProduct, Fields: json.RawMessage(`{"name":5}`), Source: "manual",
	})
	if !errors.As(err, &decode) {
		t.Errorf("a product named by a number answered %v, want a decode refusal", err)
	}
	for _, typ := range []datasource.EntityType{datasource.EntityProduct, datasource.EntityOffer} {
		_, err := p.Update(ctx, datasource.UpdateInput{
			Ref: datasource.EntityRef{Type: typ, ID: ids.NewV7()}, Patch: json.RawMessage(`{"currency":5}`),
		})
		if !errors.As(err, &decode) {
			t.Errorf("a %s patch with a numeric currency answered %v, want a decode refusal", typ, err)
		}
	}
}

func TestADealListByListGoesThroughTheListResolver(t *testing.T) {
	list := ids.NewV7()
	filters := map[string]string{"list_id": list.String()}
	if _, _, _, err := seamProvider().SearchEntity(context.Background(), datasource.EntityDeal, nil, 10, nil,
		filters); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a list filter with lists off answered %v, want not found", err)
	}

	refusal := errors.New("the list is private to its owner")
	var asked ids.UUID
	p := seamProvider().WithListMembers(func(_ context.Context, id ids.UUID, _ string) (storekit.ListMemberFilter, error) {
		asked = id
		return nil, refusal
	})
	if _, _, _, err := p.SearchEntity(context.Background(), datasource.EntityDeal, nil, 10, nil, filters); !errors.Is(err, refusal) {
		t.Errorf("the resolver's refusal came back as %v", err)
	}
	if asked != list {
		t.Errorf("the resolver was asked about list %v, want %v", asked, list)
	}
}
