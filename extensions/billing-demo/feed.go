// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package billingdemo

// The sync: one tick offers every invoice whose amount or paid flag differs from
// what the core last answered. It reads, closes its transaction, then ingests.
// A second transaction records the answer, so a stop in between costs one
// repeated offer, which the outside id makes a no-op.

import (
	"context"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/pkg/extension"
)

const (
	connectionTable = "ext.ext_billing_demo_connection"
	invoiceTable    = "ext.ext_billing_demo_invoice"
	syncBatch       = 100
)

type connection struct {
	member, pipeline, openStage, wonStage string
}

type invoice struct {
	erpID, name, currency string
	amountMinor           int64
	paid                  bool
}

func syncTick(ctx context.Context, rt extension.Runtime) error {
	var (
		conn    connection
		pending []invoice
	)
	err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		var err error
		if conn, err = readConnection(ctx, tx); err != nil {
			return err
		}
		pending, err = pendingInvoices(ctx, tx)
		return err
	})
	if errors.Is(err, extension.ErrNoRows) {
		return nil
	}
	if err != nil {
		return extension.Failure(classSyncFailed, err)
	}
	for _, inv := range pending {
		res, err := rt.Ingest(ctx, extension.UserID(conn.member), recordFor(conn, inv))
		if err != nil {
			return extension.Failure(classSyncFailed, fmt.Errorf("invoice %s: %w", inv.erpID, err))
		}
		if res.Disposition == extension.DispositionUnrepresentable {
			return extension.Failure(classSyncFailed, fmt.Errorf("invoice %s: the core refused it as unrepresentable (%s)", inv.erpID, res.Reason))
		}
		if err := recordAnswer(ctx, rt, inv, res); err != nil {
			return extension.Failure(classSyncFailed, err)
		}
	}
	return nil
}

func readConnection(ctx context.Context, tx extension.Tx) (connection, error) {
	var c connection
	err := tx.QueryRow(ctx, `SELECT member_id::text, pipeline_id::text, open_stage_id::text, won_stage_id::text
		FROM `+connectionTable).Scan(&c.member, &c.pipeline, &c.openStage, &c.wonStage)
	return c, err
}

func pendingInvoices(ctx context.Context, tx extension.Tx) ([]invoice, error) {
	rows, err := tx.Query(ctx, `SELECT erp_id, name, currency, amount_minor, paid FROM `+invoiceTable+`
		WHERE synced_paid IS NULL OR synced_paid <> paid
		   OR synced_amount_minor IS NULL OR synced_amount_minor <> amount_minor
		ORDER BY created_at, erp_id LIMIT $1`, syncBatch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []invoice
	for rows.Next() {
		var inv invoice
		if err := rows.Scan(&inv.erpID, &inv.name, &inv.currency, &inv.amountMinor, &inv.paid); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// recordFor is the deal an invoice stands for. A paid invoice sits on the won
// stage, an unpaid one on the open stage.
func recordFor(conn connection, inv invoice) extension.Record {
	stage := conn.openStage
	if inv.paid {
		stage = conn.wonStage
	}
	amount := inv.amountMinor
	return extension.Record{
		System: system,
		Key:    inv.erpID,
		Deal: &extension.DealFields{
			Name:        inv.name,
			PipelineID:  conn.pipeline,
			StageID:     stage,
			AmountMinor: &amount,
			Currency:    inv.currency,
		},
	}
}

func recordAnswer(ctx context.Context, rt extension.Runtime, inv invoice, res extension.Result) error {
	return rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		withheld := res.Withheld
		if withheld == nil {
			withheld = []string{}
		}
		_, err := tx.Exec(ctx, `UPDATE `+invoiceTable+` SET synced_paid = $2, synced_amount_minor = $3,
			deal_id = NULLIF($4, '')::uuid, withheld = $5, synced_at = now() WHERE erp_id = $1`,
			inv.erpID, inv.paid, inv.amountMinor, res.Ref.ID, withheld)
		return err
	})
}
