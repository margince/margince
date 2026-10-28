// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Who did each act in Done for you, and what it moved, through the owners
// the `/magic` receipt names them with. Home and the Worklist then describe one
// act the same way.

import (
	"context"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/magic"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type attentionReceiptDetails struct {
	magic *magic.Service
	names attentionNames
}

var _ attention.ReceiptDetails = attentionReceiptDetails{}

func (d attentionReceiptDetails) Describe(ctx context.Context, receipts []attention.Receipt) ([]attention.ReceiptDetail, error) {
	members, err := d.membersNamed(ctx, receipts)
	if err != nil {
		return nil, err
	}
	records := make([]magic.ReceiptRecord, 0, len(receipts))
	for _, receipt := range receipts {
		if receipt.TargetType != "" && !receipt.TargetID.IsZero() {
			records = append(records, magic.ReceiptRecord{
				ReceiptID: receipt.ID, EntityType: receipt.TargetType, EntityID: receipt.TargetID,
			})
		}
	}
	changes, err := d.magic.ReceiptChanges(ctx, records)
	if err != nil {
		return nil, err
	}
	out := make([]attention.ReceiptDetail, len(receipts))
	for i, receipt := range receipts {
		out[i].Actor = labelledActor(receipt.Actor, members)
		if moved, read := changes[receipt.ID]; read {
			out[i].Changes = &moved
		}
	}
	return out, nil
}

// membersNamed reads the name of every member who acted on the page, in one
// read under the reader's own grants.
func (d attentionReceiptDetails) membersNamed(ctx context.Context, receipts []attention.Receipt) (map[ids.UUID]string, error) {
	var seats []ids.UUID
	for _, receipt := range receipts {
		if seat, ok := memberOf(receipt.Actor); ok {
			seats = append(seats, seat)
		}
	}
	if len(seats) == 0 {
		return map[ids.UUID]string{}, nil
	}
	return d.names.Labels(ctx, entityUser, seats)
}

func labelledActor(actor attention.ReceiptActor, members map[ids.UUID]string) *crmcontracts.ReceiptActor {
	if actor.Type == "" {
		return nil
	}
	seat, _ := memberOf(actor)
	out := &crmcontracts.ReceiptActor{
		Type:  crmcontracts.ReceiptActorType(actor.Type),
		Id:    actor.ID,
		Label: magic.ActorLabel(actor.Type, actor.ID, members[seat]),
	}
	if actor.OnBehalfOf != nil {
		on := openapi_types.UUID(*actor.OnBehalfOf)
		out.OnBehalfOf = &on
	}
	return out
}

// memberOf is the seat behind a human actor id ("human:<uuid>").
func memberOf(actor attention.ReceiptActor) (ids.UUID, bool) {
	if actor.Type != string(principal.PrincipalHuman) {
		return ids.Nil, false
	}
	seat, err := ids.Parse(strings.TrimPrefix(actor.ID, string(principal.PrincipalHuman)+":"))
	if err != nil || seat.IsZero() {
		return ids.Nil, false
	}
	return seat, true
}
