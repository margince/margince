// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package crmhello is the walking-skeleton reference extension: the
// smallest unit that exercises the whole stable-tier path —
// scanned from extensions/, composed by gen-composition, reconciled into
// the core registries at boot, enumerated in the boot inventory. The CI
// extension lane copies it under extensions/; the vanilla tree never
// compiles it. Its module path is deliberately non-fetchable: an enabled
// extension resolves through the composed workspace, never a proxy.
package crmhello

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
)

// New returns the unit's declaration. hello_ping in api/crm.yaml has no Tools
// entry: a contract-only request reaches the manifest and serves
// nothing. The subscription puts a declared system read in the extension lane,
// and the panel puts the frontend layer and a panel slot there. The hooks put
// a write hook and an operation hook there.
func New() extension.Extension {
	return extension.Extension{
		Name:          "crm-hello",
		Version:       "0.1.0",
		Description:   "The tier's smoke fixture: one jurisdiction pack, and a note of each deal change.",
		Jurisdictions: []jurisdiction.Pack{pack{}},
		Subscriptions: []extension.Subscription{
			{Name: "note_deal_change", Events: []string{"deal.updated"}, Handle: noteDealChange},
		},
		SystemReads: []extension.SystemRead{extension.SystemReadDeals},
		Settings: []extension.Setting{
			{Key: "greeting", Label: "Greeting", Hint: "A line the smoke fixture keeps.", Kind: extension.SettingText},
		},
		Panels: []extension.Panel{
			{Slot: extension.SlotDealPanel, Export: "DealHello", Object: "ext_crm_hello_ping"},
		},
		WriteHooks: []extension.WriteHook{
			{Entity: "deal", Actions: []string{"create"}, Handle: refuseSentinelWrite},
		},
		OperationHooks: []extension.OperationHook{
			{Operations: []string{"createDeal"}, Handle: refuseSentinelOperation},
		},
	}
}

// The hooks refuse only their own sentinel names, so a composed lane runs them
// on every deal create without changing what any other test sees.
const (
	sentinelWrite     = "crm-hello: refuse this write"
	sentinelOperation = "crm-hello: refuse this call"
)

func refuseSentinelWrite(_ context.Context, _ extension.Runtime, w extension.Write) error {
	if bytes.Contains(w.After, []byte(sentinelWrite)) {
		return fmt.Errorf("%w: crm-hello refuses this deal", extension.ErrConflict)
	}
	return nil
}

func refuseSentinelOperation(_ context.Context, _ extension.Runtime, op *extension.Operation) error {
	if bytes.Contains(op.Body, []byte(sentinelOperation)) {
		return fmt.Errorf("%w: crm-hello refuses this call", extension.ErrInvalid)
	}
	return nil
}

// noteDealChange reads the deal a delivery names and logs its stage, the
// smallest real use of a declared system read.
func noteDealChange(ctx context.Context, rt extension.Runtime, d extension.Delivery) error {
	deal, err := rt.Deals().Get(ctx, d.Entity.ID)
	if errors.Is(err, extension.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "crm-hello: a deal changed", "deal", deal.ID, "stage", deal.StageName)
	return nil
}

// pack registers under "zz" — an ISO 3166-1 user-assigned code, so the
// fixture can never collide with a real jurisdiction pack.
type pack struct{}

func (pack) Code() jurisdiction.Code { return "zz" }

func (pack) Retention() jurisdiction.Retention { return nil }
