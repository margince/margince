// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The language table is the one source of which languages a writer accepts.
// Writers read it in their own transaction. A code a removed unit once declared
// keeps working, and a code another process wrote needs no restart.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func declareLanguage(t *testing.T, code string) {
	t.Helper()
	owner := OwnerConn(t)
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO language (code, label, source) VALUES ($1, $1, 'unit') ON CONFLICT DO NOTHING`, code); err != nil {
		t.Fatalf("declaring %s: %v", code, err)
	}
	t.Cleanup(func() {
		for _, q := range []string{
			`DELETE FROM product_translation WHERE language = $1`,
			`UPDATE legal_entity SET default_language = NULL WHERE default_language = $1`,
			`UPDATE offer SET language = NULL WHERE language = $1`,
			`DELETE FROM language WHERE code = $1`,
		} {
			if _, err := owner.Exec(context.Background(), q, code); err != nil {
				t.Errorf("cleaning up %s: %v", code, err)
			}
		}
	})
}

func TestACodeAnotherProcessWroteIsAcceptedWithNoRestartAndStaysAcceptedAfterItsUnitIsGone(t *testing.T) {
	e := Setup(t)
	desk := e.As(e.AdminUser, nil, offerLanguagePerms)
	product := seedRetainer(t, e)

	_, err := e.Deals.PutProductTranslation(desk, product, "ar", "Retainer", nil)
	if fault, ok := errors.AsType[*values.ParseError](err); !ok || fault.Field != "language" {
		t.Fatalf("a language no unit declared answered %v, want a refusal on the language field", err)
	}
	declareLanguage(t, "ar")
	if _, err := e.Deals.PutProductTranslation(desk, product, "ar", "Retainer", nil); err != nil {
		t.Fatalf("a language another process wrote to the table was refused: %v", err)
	}
	// The unit is gone from this process, and the row is not: its rows stay editable.
	if _, err := e.Deals.PutProductTranslation(desk, product, "ar", "Retainer 2", nil); err != nil {
		t.Fatalf("a language a removed unit once declared locked its rows: %v", err)
	}
	if err := e.Deals.DeleteProductTranslation(desk, product, "ar"); err != nil {
		t.Fatalf("deleting a row in a language a removed unit declared: %v", err)
	}
}

func TestADeleteNeverValidatesTheLanguage(t *testing.T) {
	e := Setup(t)
	desk := e.As(e.AdminUser, nil, offerLanguagePerms)
	product := seedRetainer(t, e)
	if err := e.Deals.DeleteProductTranslation(desk, product, "zz"); err != nil {
		t.Fatalf("a delete in a language the table does not hold was refused: %v", err)
	}
}

func TestAnEntityLanguageNoUnitDeclaredIsAFieldRefusalAndADeclaredOneIsStored(t *testing.T) {
	e := Setup(t)
	vienna := seedVienna(t, e)
	_, err := e.Contacts.UpdateLegalEntity(e.Admin(), vienna.ID, contacts.LegalEntityInput{DefaultLanguage: new("ar")}, nil)
	if fault, ok := errors.AsType[*values.ParseError](err); !ok || fault.Field != "default_language" {
		t.Fatalf("an undeclared language answered %v, want a refusal on default_language", err)
	}
	declareLanguage(t, "ar")
	if _, err := e.Contacts.UpdateLegalEntity(e.Admin(), vienna.ID, contacts.LegalEntityInput{DefaultLanguage: new("ar")}, nil); err != nil {
		t.Fatalf("a declared language was refused: %v", err)
	}
}

func TestUpdatingATemplateToABadLocaleIsAFieldRefusalNotAServerError(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	tmpl, err := e.Deals.CreateOfferTemplate(admin, deals.CreateOfferTemplateInput{Name: "Locale update", Locale: "de-AT"})
	if err != nil {
		t.Fatalf("create the template: %v", err)
	}
	_, err = e.Deals.UpdateOfferTemplate(admin, ids.From[ids.OfferTemplateKind](ids.UUID(tmpl.Id)), deals.UpdateOfferTemplateInput{Name: "Locale update", Locale: "DE"})
	if fault, ok := errors.AsType[*values.ParseError](err); !ok || fault.Field != "locale" {
		t.Fatalf("an update to DE answered %v, want a refusal on locale", err)
	}
}
