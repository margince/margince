// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package completedchanges connects completed changes to receipt history.
package completedchanges

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/magic"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Reader preserves receipt metadata while using Home's completed-change population.
type Reader struct{ Magic *magic.Service }

type completedReceiptsKey struct{}

// Recent supplies the receipt interface without a page cutoff.
func (c Reader) Recent(ctx context.Context, since time.Time, limit int) ([]attention.Receipt, error) {
	read, err := c.RecentWithState(ctx, since, time.Time{}, limit)
	return read.Receipts, err
}

// RecentWithState preserves withholding and floor information across history pages.
func (c Reader) RecentWithState(ctx context.Context, since, through time.Time, limit int) (attention.ReceiptRead, error) {
	originals := map[ids.UUID]attention.Receipt{}
	ctx = context.WithValue(ctx, completedReceiptsKey{}, originals)
	read, err := c.Magic.CompletedChanges(ctx, since, through, limit)
	if err != nil {
		return attention.ReceiptRead{}, err
	}
	out := attention.ReceiptRead{Floor: read.Floor, Unavailable: read.Unavailable, Receipts: make([]attention.Receipt, 0, len(read.Lines))}
	for _, line := range read.Lines {
		receipt := attention.Receipt{ID: ids.UUID(line.Id), Source: attention.ReceiptFromCorrection, AuditLogID: ids.UUID(line.Id), Kind: line.Summary.Key, OccurredAt: line.OccurredAt, Actor: attention.ReceiptActor{Type: string(line.Actor.Type), ID: line.Actor.Id}}
		if line.Actor.OnBehalfOf != nil {
			receipt.Actor.OnBehalfOf = new(ids.UUID(*line.Actor.OnBehalfOf))
		}
		identity := receipt.ID
		if line.ReceiptIds != nil && len(*line.ReceiptIds) == 1 {
			identity = ids.UUID((*line.ReceiptIds)[0])
		}
		if original, ok := originals[identity]; ok && (line.ReceiptIds == nil || len(*line.ReceiptIds) == 1) {
			receipt = original
		}
		receipt.CompletedChange = new(line)
		if line.Entity != nil {
			receipt.TargetType, receipt.TargetID, receipt.TargetLabel = line.Entity.Type, ids.UUID(line.Entity.Id), line.Entity.Label
		}
		out.Receipts = append(out.Receipts, receipt)
	}
	return out, nil
}

var _ attention.Receipts = Reader{}

// Remember retains original receipt metadata within one completed-change read.
func Remember(ctx context.Context, receipt attention.Receipt) {
	if originals, ok := ctx.Value(completedReceiptsKey{}).(map[ids.UUID]attention.Receipt); ok {
		originals[receipt.ID] = receipt
	}
}
