// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The finance mirror's sources: every accounting system this build can read is
// a composed unit's FinanceSource, adapted here onto finance.Provider. The core
// names no provider of its own.

import (
	"context"
	"fmt"
	"sort"

	"github.com/margince/margince/backend/internal/modules/finance"
	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/accounting"
)

// composedFinanceSources implements finance.Sources over this boot's
// composition. The boot has already refused a provider claimed twice.
type composedFinanceSources struct{}

var _ finance.Sources = composedFinanceSources{}

func (composedFinanceSources) Providers() []finance.SourceOption {
	var out []finance.SourceOption
	for _, ext := range ComposedExtensions() {
		for _, source := range ext.FinanceSources {
			out = append(out, finance.SourceOption{Provider: source.Provider, Description: string(ext.Description)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return out
}

//nolint:ireturn // the seam IS an interface: this resolves WHICH unit reads the source
func (composedFinanceSources) Open(provider string, conn accounting.Connection) (finance.Provider, bool) {
	for _, ext := range ComposedExtensions() {
		for _, source := range ext.FinanceSources {
			if source.Provider == provider {
				return unitFinanceProvider{
					unit: string(ext.Name), version: string(ext.Version),
					source: source, conn: conn, deps: boundExtensionRuntime(),
				}, true
			}
		}
	}
	return nil, false
}

// unitFinanceProvider is one unit's source bound to one connection. Each call
// mints its own Runtime and releases it on return, so a unit cannot keep one
// across calls.
type unitFinanceProvider struct {
	unit    string
	version string
	source  extension.FinanceSource
	conn    accounting.Connection
	deps    extensionRuntimeBinding
}

func (p unitFinanceProvider) Name() string { return p.source.Provider }

func (p unitFinanceProvider) Check(ctx context.Context) error {
	rt := financeRuntimeFor(ctx, p.unit, p.version, p.via(), p.deps)
	defer rt.release()
	if err := p.source.Check(ctx, rt, p.conn); err != nil {
		return fmt.Errorf("extension %q: check %s: %w", p.unit, p.source.Provider, err)
	}
	return nil
}

func (p unitFinanceProvider) Customers(ctx context.Context) ([]accounting.Customer, error) {
	rt := financeRuntimeFor(ctx, p.unit, p.version, p.via(), p.deps)
	defer rt.release()
	customers, err := p.source.Customers(ctx, rt, p.conn)
	if err != nil {
		return nil, fmt.Errorf("extension %q: customers of %s: %w", p.unit, p.source.Provider, err)
	}
	return customers, nil
}

func (p unitFinanceProvider) InvoicesFor(ctx context.Context, externalCustomerID string) (accounting.Ledger, error) {
	rt := financeRuntimeFor(ctx, p.unit, p.version, p.via(), p.deps)
	defer rt.release()
	ledger, err := p.source.InvoicesFor(ctx, rt, p.conn, externalCustomerID)
	if err != nil {
		return accounting.Ledger{}, fmt.Errorf("extension %q: ledger of %s: %w", p.unit, p.source.Provider, err)
	}
	return ledger, nil
}

func (p unitFinanceProvider) via() string { return "finance/" + p.source.Provider }

// preflightFinanceSources refuses a malformed source, and a provider two units
// both claim. The mirror stamps the provider on every row, so one name read by
// two units would leave a row's origin unknown.
func preflightFinanceSources(e extension.Extension, claimed map[string]extension.Name) error {
	for _, source := range e.FinanceSources {
		if err := source.Validate(); err != nil {
			return fmt.Errorf("compose: extension %q: %w", e.Name, err)
		}
		if other, taken := claimed[source.Provider]; taken {
			return fmt.Errorf("compose: extension %q declares finance source %q, already claimed by extension %q — one provider names one source",
				e.Name, source.Provider, other)
		}
		claimed[source.Provider] = e.Name
	}
	return nil
}
