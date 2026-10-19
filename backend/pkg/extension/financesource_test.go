// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/accounting"
)

func aFinanceSource(provider string) extension.FinanceSource {
	return extension.FinanceSource{
		Provider: provider,
		Check:    func(context.Context, extension.Runtime, accounting.Connection) error { return nil },
		Customers: func(context.Context, extension.Runtime, accounting.Connection) ([]accounting.Customer, error) {
			return nil, nil
		},
		InvoicesFor: func(context.Context, extension.Runtime, accounting.Connection, string) (accounting.Ledger, error) {
			return accounting.Ledger{}, nil
		},
	}
}

func TestAWellFormedFinanceSourceIsAccepted(t *testing.T) {
	for _, provider := range []string{"ledgerly", "offline_demo", "books2"} {
		if err := aFinanceSource(provider).Validate(); err != nil {
			t.Errorf("provider %q was refused: %v", provider, err)
		}
	}
}

// The provider grammar is Channel's, so one key spelling serves every provider
// a unit names.
func TestAFinanceSourceProviderFollowsTheChannelProviderGrammar(t *testing.T) {
	for name, provider := range map[string]string{
		"empty":               "",
		"kebab":               "ledger-ly",
		"leading digit":       "1books",
		"upper case":          "Ledgerly",
		"over the length cap": strings.Repeat("b", 33),
	} {
		t.Run(name, func(t *testing.T) {
			err := aFinanceSource(provider).Validate()
			if !errors.Is(err, extension.ErrInvalid) {
				t.Fatalf("provider %q: Validate() = %v, want ErrInvalid", provider, err)
			}
		})
	}
}

// The core calls all three functions, so a missing one is refused at boot
// rather than found as a nil call on the first sync.
func TestAFinanceSourceMissingAFunctionIsRefused(t *testing.T) {
	for name, strip := range map[string]func(*extension.FinanceSource){
		"no Check":       func(s *extension.FinanceSource) { s.Check = nil },
		"no Customers":   func(s *extension.FinanceSource) { s.Customers = nil },
		"no InvoicesFor": func(s *extension.FinanceSource) { s.InvoicesFor = nil },
	} {
		t.Run(name, func(t *testing.T) {
			source := aFinanceSource("ledgerly")
			strip(&source)
			if err := source.Validate(); !errors.Is(err, extension.ErrInvalid) {
				t.Fatalf("Validate() = %v, want ErrInvalid", err)
			}
		})
	}
}
