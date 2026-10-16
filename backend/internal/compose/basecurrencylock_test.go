// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/shared/kernel/currencylock"
)

func TestTheLockFactsTravelExactlyWithTheirProse(t *testing.T) {
	h := installationSettingsHandlers{now: func() time.Time { return time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC) }}
	allowances := agentvolume.Policy{Window: agentvolume.WindowPlan{Length: 24 * time.Hour}}

	open := h.toContract(identity.InstallationSettings{AgentAllowances: allowances})
	if open.BaseCurrencyLock != nil || open.BaseCurrencyLockedReason != nil {
		t.Errorf("a changeable currency sent lock %+v and reason %v, want neither", open.BaseCurrencyLock, open.BaseCurrencyLockedReason)
	}

	lock := currencylock.Lock{Reason: currencylock.RateSheet, Count: 3, BaseCurrency: "EUR"}
	locked := h.toContract(identity.InstallationSettings{
		BaseCurrencyLocked: true, BaseCurrencyLock: lock, AgentAllowances: allowances,
	})
	if locked.BaseCurrencyLockedReason == nil || *locked.BaseCurrencyLockedReason != lock.Prose() {
		t.Errorf("reason = %v, want the prose of the facts sent beside it: %q", locked.BaseCurrencyLockedReason, lock.Prose())
	}
	got := locked.BaseCurrencyLock
	if got == nil || got.Reason != crmcontracts.BaseCurrencyLockReasonRateSheet || got.Count != 3 ||
		got.BaseCurrency == nil || *got.BaseCurrency != "EUR" {
		t.Errorf("lock = %+v, want rate_sheet, 3, EUR", got)
	}

	unnamed := toBaseCurrencyLock(currencylock.Lock{Reason: currencylock.RateSheet, Count: 1})
	if unnamed.BaseCurrency != nil {
		t.Errorf("base_currency = %q for a lock with no stored base, want it absent", *unnamed.BaseCurrency)
	}
}

// The reasons are read from the kernel's source. A reason added there without
// a contract value fails here, not on a client that cannot render it.
func TestEveryLockReasonIsOneTheContractPublishes(t *testing.T) {
	reasons := lockReasonsDeclared(t)
	if len(reasons) == 0 {
		t.Fatal("no currencylock.Reason constants found; the census read the wrong package")
	}
	for _, reason := range reasons {
		wire := toBaseCurrencyLock(currencylock.Lock{Reason: reason, Count: 1}).Reason
		if !wire.Valid() {
			t.Errorf("currencylock reason %q is not a BaseCurrencyLock.reason value in crm.yaml", reason)
		}
	}
}

func lockReasonsDeclared(t *testing.T) []currencylock.Reason {
	t.Helper()
	const dir = "../shared/kernel/currencylock"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the currencylock package: %v", err)
	}
	var reasons []currencylock.Reason
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, entry.Name()), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", entry.Name(), err)
		}
		reasons = append(reasons, reasonConstants(t, file)...)
	}
	return reasons
}

func reasonConstants(t *testing.T, file *ast.File) []currencylock.Reason {
	t.Helper()
	var reasons []currencylock.Reason
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if ident, ok := spec.Type.(*ast.Ident); !ok || ident.Name != "Reason" {
			return true
		}
		for _, value := range spec.Values {
			lit, ok := value.(*ast.BasicLit)
			if !ok {
				t.Fatalf("a Reason constant is not a string literal: %T", value)
			}
			unquoted, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("unquoting %s: %v", lit.Value, err)
			}
			reasons = append(reasons, currencylock.Reason(unquoted))
		}
		return true
	})
	return reasons
}
