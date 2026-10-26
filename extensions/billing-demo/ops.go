// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package billingdemo

// The operations an admin drives the demo with. Each is human-only and holds no
// rights of its own. The connect step files the feed secret under the service
// member. That is the operator agreeing that the feed lands deals as it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/pkg/extension"
)

const jobSync extension.JobName = "sync"

// sampleInvoices are what a connection starts with: open invoices that the feed
// turns into deals on the open stage.
var sampleInvoices = []invoice{
	{erpID: "INV-1001", name: "Acme Corp - annual licence", currency: "EUR", amountMinor: 1200000},
	{erpID: "INV-1002", name: "Borealis Foods - rollout", currency: "EUR", amountMinor: 840000},
	{erpID: "INV-1003", name: "Cinder Logistics - support plan", currency: "EUR", amountMinor: 365000},
}

// isUUID reports whether v is a hyphenated hexadecimal id, checked here because
// the database would turn a bare string into a 500 at the ::uuid cast.
func isUUID(v string) bool {
	if len(v) != 36 {
		return false
	}
	for i, r := range v {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if r != '-' {
				return false
			}
		case !strings.ContainsRune("0123456789abcdefABCDEF", r):
			return false
		}
	}
	return true
}

func humanCaller(rt extension.Runtime) error {
	c := rt.Caller()
	if c.Type != extension.CallerHuman || c.IsAgent {
		return fmt.Errorf("%w: only a signed-in admin runs the billing demo", extension.ErrForbidden)
	}
	return nil
}

func checkUUIDs(named map[string]string) error {
	for name, v := range named {
		if !isUUID(v) {
			return fmt.Errorf("%w: %s is not an id", extension.ErrInvalid, name)
		}
	}
	return nil
}

type connectArgs struct {
	MemberID    string `json:"member_id"`
	PipelineID  string `json:"pipeline_id"`
	OpenStageID string `json:"open_stage_id"`
	WonStageID  string `json:"won_stage_id"`
}

// connect names the service member and the pipeline the deals land in, seeds the
// sample invoices, files the feed secret and asks for a sync.
func connect(ctx context.Context, rt extension.Runtime, in json.RawMessage) (json.RawMessage, error) {
	if err := humanCaller(rt); err != nil {
		return nil, err
	}
	args, err := extension.DecodeArgs[connectArgs](in)
	if err != nil {
		return nil, err
	}
	if err := checkUUIDs(map[string]string{"member_id": args.MemberID, "pipeline_id": args.PipelineID,
		"open_stage_id": args.OpenStageID, "won_stage_id": args.WonStageID}); err != nil {
		return nil, err
	}
	if err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO `+connectionTable+` (member_id, pipeline_id, open_stage_id, won_stage_id)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid)
			ON CONFLICT (only_row) DO UPDATE SET member_id = EXCLUDED.member_id, pipeline_id = EXCLUDED.pipeline_id,
			open_stage_id = EXCLUDED.open_stage_id, won_stage_id = EXCLUDED.won_stage_id`,
			args.MemberID, args.PipelineID, args.OpenStageID, args.WonStageID); err != nil {
			return err
		}
		for _, inv := range sampleInvoices {
			if _, err := tx.Exec(ctx, `INSERT INTO `+invoiceTable+` (erp_id, name, currency, amount_minor)
				VALUES ($1, $2, $3, $4) ON CONFLICT (erp_id) DO NOTHING`,
				inv.erpID, inv.name, inv.currency, inv.amountMinor); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	// Filed outside the transaction: PutUser opens its own.
	if err := rt.Secrets().PutUser(ctx, extension.UserID(args.MemberID), feedSecret, []byte("connected")); err != nil {
		return nil, err
	}
	if err := rt.SyncNow(ctx, jobSync); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"connected": true, "invoices": len(sampleInvoices)})
}

type updateArgs struct {
	ERPID       string `json:"erp_id"`
	Paid        *bool  `json:"paid"`
	AmountMinor *int64 `json:"amount_minor"`
}

// updateInvoice is the billing system's side: an invoice is paid or its amount is
// revised. The next sync offers the change to the core.
func updateInvoice(ctx context.Context, rt extension.Runtime, in json.RawMessage) (json.RawMessage, error) {
	if err := humanCaller(rt); err != nil {
		return nil, err
	}
	args, err := extension.DecodeArgs[updateArgs](in)
	if err != nil {
		return nil, err
	}
	if args.Paid == nil && args.AmountMinor == nil {
		return nil, fmt.Errorf("%w: name paid or amount_minor", extension.ErrInvalid)
	}
	if args.AmountMinor != nil && *args.AmountMinor < 0 {
		return nil, fmt.Errorf("%w: amount_minor is not negative", extension.ErrInvalid)
	}
	var changed int64
	if err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		n, err := tx.Exec(ctx, `UPDATE `+invoiceTable+` SET paid = COALESCE($2, paid),
			amount_minor = COALESCE($3, amount_minor) WHERE erp_id = $1`, args.ERPID, args.Paid, args.AmountMinor)
		changed = n
		return err
	}); err != nil {
		return nil, err
	}
	if changed == 0 {
		return nil, extension.ErrNotFound
	}
	if err := rt.SyncNow(ctx, jobSync); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"updated": true})
}

type statusRow struct {
	ERPID       string   `json:"erp_id"`
	Name        string   `json:"name"`
	Currency    string   `json:"currency"`
	AmountMinor int64    `json:"amount_minor"`
	Paid        bool     `json:"paid"`
	Synced      bool     `json:"synced"`
	DealID      string   `json:"deal_id"`
	Withheld    []string `json:"withheld"`
}

// status lists each invoice with the deal it became and the fields the core left
// as a member edited them.
func status(ctx context.Context, rt extension.Runtime, _ json.RawMessage) (json.RawMessage, error) {
	if err := humanCaller(rt); err != nil {
		return nil, err
	}
	out := []statusRow{}
	err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		rows, err := tx.Query(ctx, `SELECT erp_id, name, currency, amount_minor, paid,
			COALESCE(synced_paid = paid AND synced_amount_minor = amount_minor, false),
			COALESCE(deal_id::text, ''), withheld FROM `+invoiceTable+` ORDER BY created_at, erp_id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r statusRow
			if err := rows.Scan(&r.ERPID, &r.Name, &r.Currency, &r.AmountMinor, &r.Paid, &r.Synced, &r.DealID, &r.Withheld); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	if err != nil && !errors.Is(err, extension.ErrNoRows) {
		return nil, err
	}
	return json.Marshal(map[string]any{"invoices": out})
}
