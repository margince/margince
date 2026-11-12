// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/accounting"
)

// recordingSource is a unit's finance source that remembers what the core
// handed it. It keeps the Runtime, so a test can try it after the call.
type recordingSource struct {
	conn     accounting.Connection
	customer string
	kept     extension.Runtime
	fail     error
}

func (r *recordingSource) declare(provider string) extension.FinanceSource {
	return extension.FinanceSource{
		Provider: provider,
		Check: func(_ context.Context, rt extension.Runtime, conn accounting.Connection) error {
			r.conn, r.kept = conn, rt
			return r.fail
		},
		Customers: func(_ context.Context, rt extension.Runtime, conn accounting.Connection) ([]accounting.Customer, error) {
			r.conn, r.kept = conn, rt
			return []accounting.Customer{{ExternalID: "C-1", DisplayName: "One"}}, r.fail
		},
		InvoicesFor: func(_ context.Context, rt extension.Runtime, conn accounting.Connection, customer string) (accounting.Ledger, error) {
			r.conn, r.kept, r.customer = conn, rt, customer
			return accounting.Ledger{}, r.fail
		},
		InvoiceDocument: func(_ context.Context, rt extension.Runtime, conn accounting.Connection, invoice string) (accounting.Document, error) {
			r.conn, r.kept = conn, rt
			return accounting.Document{ContentType: "application/pdf", Body: []byte(invoice)}, r.fail
		},
	}
}

// composeBooks composes one unit, "books", supplying the given sources.
func composeBooks(t *testing.T, sources ...extension.FinanceSource) {
	t.Helper()
	composeUnit(t, extension.Extension{Name: "books", Version: "1.0.0", Description: "Reads the books.", FinanceSources: sources})
}

func TestThePreflightHoldsFinanceSourcesToTheComposedSet(t *testing.T) {
	source := &recordingSource{}
	mine := extension.Extension{
		Name: "mine", Version: "1.0.0",
		FinanceSources: []extension.FinanceSource{source.declare("mine_books")},
	}

	claimed := map[string]extension.Name{}
	if err := preflightFinanceSources(mine, claimed); err != nil {
		t.Fatalf("a well-formed finance source was refused: %v", err)
	}

	malformed := mine
	malformed.FinanceSources = []extension.FinanceSource{{Provider: "mine-books"}}
	if err := preflightFinanceSources(malformed, map[string]extension.Name{}); err == nil {
		t.Error("a finance source the published Validate refuses was accepted at boot")
	}

	theirs := extension.Extension{
		Name: "theirs", Version: "1.0.0",
		FinanceSources: []extension.FinanceSource{source.declare("mine_books")},
	}
	err := preflightFinanceSources(theirs, claimed)
	if err == nil {
		t.Fatal("two units claiming one provider were accepted; every mirrored row names its provider")
	}
	if !strings.Contains(err.Error(), "mine") || !strings.Contains(err.Error(), "theirs") {
		t.Errorf("the refusal %q names only one side of the collision", err)
	}
}

func TestComposedSourcesOfferEveryUnitsProviderWithItsDescriptionAndDocuments(t *testing.T) {
	source := &recordingSource{}
	bare := source.declare("alpha_books")
	bare.InvoiceDocument = nil
	composeBooks(t, source.declare("zeta_books"), bare)

	offered := composedFinanceSources{}.Providers()
	if len(offered) != 2 || offered[0].Provider != "alpha_books" || offered[1].Provider != "zeta_books" ||
		offered[0].Documents || !offered[1].Documents || offered[0].Description != "Reads the books." {
		t.Fatalf("offered %+v, want both providers sorted with the unit's description, and documents on zeta_books alone", offered)
	}
	if alpha, ok := (composedFinanceSources{}).Open("alpha_books", accounting.Connection{}); !ok {
		t.Fatal("alpha_books was not opened")
	} else if _, err := alpha.InvoiceDocument(context.Background(), "INV-7"); !errors.Is(err, apperrors.ErrUnsupportedBySoR) {
		t.Errorf("a source without documents answered %v, want ErrUnsupportedBySoR", err)
	}
	if _, ok := (composedFinanceSources{}).Open("not_composed", accounting.Connection{}); ok {
		t.Error("a provider no unit supplies was opened")
	}
}

// The unit is handed the connection it was opened for on every call, and the
// Runtime it is handed dies with the call.
func TestAnOpenedSourceReadsWithTheConnectionAndAShortLivedRuntime(t *testing.T) {
	source := &recordingSource{}
	composeBooks(t, source.declare("books"))
	conn := accounting.Connection{ID: "conn-1", ConnectedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}

	provider, ok := composedFinanceSources{}.Open("books", conn)
	if !ok {
		t.Fatal("a composed provider was not opened")
	}
	if provider.Name() != "books" {
		t.Errorf("Name() = %q, want the declared provider", provider.Name())
	}
	if _, err := provider.InvoicesFor(context.Background(), "C-1"); err != nil {
		t.Fatal(err)
	}
	if source.conn != conn || source.customer != "C-1" {
		t.Errorf("the unit was handed %+v / %q, want %+v / C-1", source.conn, source.customer, conn)
	}
	source.conn = accounting.Connection{}
	if document, err := provider.InvoiceDocument(context.Background(), "INV-7"); err != nil || string(document.Body) != "INV-7" || source.conn != conn {
		t.Errorf("InvoiceDocument() = %q, %v with %+v; want the unit's document for INV-7 on %+v", document.Body, err, source.conn, conn)
	}
	err := source.kept.Tx(context.Background(), func(context.Context, extension.Tx) error { return nil })
	if !errors.Is(err, extension.ErrRuntimeExpired) {
		t.Errorf("a Runtime kept past its call answered %v, want ErrRuntimeExpired", err)
	}
}

func TestASourceFailureNamesTheUnitAndKeepsItsCause(t *testing.T) {
	cause := errors.New("vendor said no")
	source := &recordingSource{fail: cause}
	composeBooks(t, source.declare("books"))
	provider, _ := composedFinanceSources{}.Open("books", accounting.Connection{})

	_, customersErr := provider.Customers(context.Background())
	_, invoicesErr := provider.InvoicesFor(context.Background(), "C-1")
	_, documentErr := provider.InvoiceDocument(context.Background(), "INV-7")
	for call, err := range map[string]error{"Check": provider.Check(context.Background()), "Customers": customersErr, "InvoicesFor": invoicesErr, "InvoiceDocument": documentErr} {
		if !errors.Is(err, cause) || !strings.Contains(err.Error(), `"books"`) {
			t.Errorf("%s() = %v, want the cause wrapped with the unit's name", call, err)
		}
	}
}

func TestAnOpenedSourceAnswersItsCheckAndDirectory(t *testing.T) {
	source := &recordingSource{}
	composeBooks(t, source.declare("books"))
	provider, _ := composedFinanceSources{}.Open("books", accounting.Connection{ID: "conn-2"})

	if err := provider.Check(context.Background()); err != nil {
		t.Errorf("Check() = %v, want a readable source", err)
	}
	customers, err := provider.Customers(context.Background())
	if err != nil || len(customers) != 1 || customers[0].ExternalID != "C-1" {
		t.Errorf("Customers() = %+v, %v; want the unit's directory", customers, err)
	}
	if source.conn.ID != "conn-2" {
		t.Errorf("the unit was handed connection %q, want conn-2", source.conn.ID)
	}
}
