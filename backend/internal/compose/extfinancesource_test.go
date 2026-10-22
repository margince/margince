// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
	}
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

func TestComposedSourcesOfferEveryUnitsProviderWithItsDescription(t *testing.T) {
	source := &recordingSource{}
	composeUnit(t, extension.Extension{
		Name: "books", Version: "1.0.0", Description: "Reads the books.",
		FinanceSources: []extension.FinanceSource{source.declare("zeta_books"), source.declare("alpha_books")},
	})

	offered := composedFinanceSources{}.Providers()
	if len(offered) != 2 || offered[0].Provider != "alpha_books" || offered[1].Provider != "zeta_books" {
		t.Fatalf("offered %+v, want both providers sorted", offered)
	}
	if offered[0].Description != "Reads the books." {
		t.Errorf("description = %q, want the unit's own", offered[0].Description)
	}
	if _, ok := (composedFinanceSources{}).Open("not_composed", accounting.Connection{}); ok {
		t.Error("a provider no unit supplies was opened")
	}
}

// The unit is handed the connection it was opened for on every call, and the
// Runtime it is handed dies with the call.
func TestAnOpenedSourceReadsWithTheConnectionAndAShortLivedRuntime(t *testing.T) {
	source := &recordingSource{}
	composeUnit(t, extension.Extension{
		Name: "books", Version: "1.0.0", Description: "Reads the books.",
		FinanceSources: []extension.FinanceSource{source.declare("books")},
	})
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
	err := source.kept.Tx(context.Background(), func(context.Context, extension.Tx) error { return nil })
	if !errors.Is(err, extension.ErrRuntimeExpired) {
		t.Errorf("a Runtime kept past its call answered %v, want ErrRuntimeExpired", err)
	}
}

func TestASourceFailureNamesTheUnitAndKeepsItsCause(t *testing.T) {
	cause := errors.New("vendor said no")
	source := &recordingSource{fail: cause}
	composeUnit(t, extension.Extension{
		Name: "books", Version: "1.0.0", Description: "Reads the books.",
		FinanceSources: []extension.FinanceSource{source.declare("books")},
	})
	provider, _ := composedFinanceSources{}.Open("books", accounting.Connection{})

	err := provider.Check(context.Background())
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), `"books"`) {
		t.Errorf("Check() = %v, want the cause wrapped with the unit's name", err)
	}
	if _, err := provider.Customers(context.Background()); !errors.Is(err, cause) || !strings.Contains(err.Error(), `"books"`) {
		t.Errorf("Customers() = %v, want the cause wrapped with the unit's name", err)
	}
	if _, err := provider.InvoicesFor(context.Background(), "C-1"); !errors.Is(err, cause) || !strings.Contains(err.Error(), `"books"`) {
		t.Errorf("InvoicesFor() = %v, want the cause wrapped with the unit's name", err)
	}
}

func TestAnOpenedSourceAnswersItsCheckAndDirectory(t *testing.T) {
	source := &recordingSource{}
	composeUnit(t, extension.Extension{
		Name: "books", Version: "1.0.0", Description: "Reads the books.",
		FinanceSources: []extension.FinanceSource{source.declare("books")},
	})
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
