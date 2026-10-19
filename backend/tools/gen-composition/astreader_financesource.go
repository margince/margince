// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Reading a unit's declared finance sources into the manifest. An operator sees
// which accounting systems a unit can read before it runs. The reason is a
// channel's: the unit reaches outside the installation for data.

import (
	"context"
	"go/ast"
	"sort"

	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/accounting"
)

// declaredFinanceSource is one accounting system a unit reads.
type declaredFinanceSource struct {
	Provider string `json:"provider"`
}

// readFinanceSources reads the FinanceSources slice literal.
func (r *unitReader) readFinanceSources(expr ast.Expr, file *ast.File) ([]declaredFinanceSource, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "FinanceSources must be a slice literal")
	}
	ext := importAlias(file, extensionPkgPath)
	out := make([]declaredFinanceSource, 0, len(lit.Elts))
	seen := map[string]bool{}
	for _, elt := range lit.Elts {
		source, err := r.readFinanceSource(elt, ext)
		if err != nil {
			return nil, err
		}
		if seen[source.Provider] {
			return nil, r.errAt(elt, "finance source provider %q declared twice", source.Provider)
		}
		seen[source.Provider] = true
		out = append(out, source)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return out, nil
}

// readFinanceSource reads one extension.FinanceSource literal. The functions
// are read as presence only. A stand-in for each goes through the published
// Validate, so generation and boot refuse the same declarations.
func (r *unitReader) readFinanceSource(elt ast.Expr, ext string) (declaredFinanceSource, error) {
	lit, ok := elt.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "FinanceSource")) {
		return declaredFinanceSource{}, r.errAt(elt, "a FinanceSources entry must be an extension.FinanceSource literal")
	}
	var (
		source declaredFinanceSource
		probe  extension.FinanceSource
	)
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return declaredFinanceSource{}, r.errAt(e, "FinanceSource fields must be keyed")
		}
		k, ok := kv.Key.(*ast.Ident)
		if !ok {
			return declaredFinanceSource{}, r.errAt(kv.Key, "FinanceSource fields must be keyed by name")
		}
		present := !isNilIdent(kv.Value)
		var err error
		switch k.Name {
		case "Provider":
			source.Provider, err = r.stringLit(kv.Value, "FinanceSource.Provider")
			probe.Provider = source.Provider
		case "Check":
			if present {
				probe.Check = presentFinanceCheck
			}
		case "Customers":
			if present {
				probe.Customers = presentFinanceCustomers
			}
		case "InvoicesFor":
			if present {
				probe.InvoicesFor = presentFinanceInvoices
			}
		default:
			err = r.errAt(kv, "FinanceSource field %s is not derivable by this generator", k.Name)
		}
		if err != nil {
			return declaredFinanceSource{}, err
		}
	}
	if err := probe.Validate(); err != nil {
		return declaredFinanceSource{}, r.errPos(lit, "%v", err)
	}
	return source, nil
}

// The stand-ins mark a function as declared; Validate asks only whether each
// is non-nil, and the generator never calls them.
var (
	presentFinanceCheck     extension.FinanceCheck     = func(_ context.Context, _ extension.Runtime, _ accounting.Connection) error { return nil }
	presentFinanceCustomers extension.FinanceCustomers = func(_ context.Context, _ extension.Runtime, _ accounting.Connection) ([]accounting.Customer, error) {
		return nil, nil
	}
	presentFinanceInvoices extension.FinanceInvoices = func(_ context.Context, _ extension.Runtime, _ accounting.Connection, _ string) (accounting.Ledger, error) {
		return accounting.Ledger{}, nil
	}
)
