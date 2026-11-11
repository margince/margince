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
	"context"
	"embed"
	"errors"
	"log/slog"

	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/privacy"
	"github.com/margince/margince/backend/pkg/extension/reach"
)

//go:embed migrations
var migrations embed.FS

// note is the unit's one table. It names a company and a contact, so the
// core's merge and erasure must reach it.
var note = reach.Table{
	Name:  "ext_crm_hello_note",
	Links: []reach.Link{{Column: "company_id", Record: reach.Company}, {Column: "contact_id", Record: reach.Contact}},
	Erase: []reach.Clear{{Column: "body", To: reach.Tombstone}, {Column: "contact_id", To: reach.Null}},
}

// New returns the unit's declaration. hello_ping in api/crm.yaml has no Tools
// entry: a contract-only request reaches the manifest and serves
// nothing. The subscription puts a declared system read in the extension lane,
// and the panel puts the frontend layer and a panel slot there. The note
// table puts a migrations layer, a Reach and a Privacy declaration there.
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
		Instrumentation: &extension.Instrumentation{Start: startInstrumentation, Browser: "instrument", Edge: instrumentationEdge()},
		Migrations:      migrations,
		Reach:           []reach.Table{note},
		// The same links as note, which the merge moves, so they never refuse
		// a merge. A subject export serves the body.
		Privacy: []privacy.Table{{
			Name:      "ext_crm_hello_note",
			Links:     []privacy.Link{{Column: "company_id", Record: privacy.Company}, {Column: "contact_id", Record: privacy.Contact}},
			Personal:  []string{"body"},
			Retention: jurisdiction.CommercialCorrespondence,
		}},
	}
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
