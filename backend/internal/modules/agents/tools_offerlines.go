// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// OfferLines changes one offer's line items through the store calls and
// request rules the line-item routes run. Money totals are derived, never
// taken, and a line changes only on an offer the caller may write.
type OfferLines interface {
	AddOfferLine(ctx context.Context, offerID ids.UUID, line json.RawMessage) (crmcontracts.Offer, error)
	UpdateOfferLine(ctx context.Context, offerID, lineID ids.UUID, line json.RawMessage) (crmcontracts.Offer, error)
	RemoveOfferLine(ctx context.Context, offerID, lineID ids.UUID) (crmcontracts.Offer, error)
}

// RegisterOfferLinesTool joins change_offer to the surface.
func RegisterOfferLinesTool(r *Registry, lines OfferLines) {
	r.Register(changeOffer{lines: lines})
}

// The actions change_offer takes, one per line-item route.
const (
	offerAddLine    = "add_line"
	offerUpdateLine = "update_line"
	offerRemoveLine = "remove_line"
)

// changeOffer is one verb with an action argument rather than a router. Every
// action is the same write on the same offer at the same tier.
type changeOffer struct{ lines OfferLines }

func (t changeOffer) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "change_offer", Title: "Change an offer's line items", Version: toolVersionV1,
		Description:   changeOfferCopy.render(),
		Instead:       changeOfferCopy.Instead,
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "addOfferLineItem/updateOfferLineItem/removeOfferLineItem",
		InputSchema: schema(`{"type":"object","required":["action","offer_id"],"properties":{
			"action":{"type":"string","enum":["add_line","update_line","remove_line"]},
			"offer_id":{"type":"string","format":"uuid"},
			"line_item_id":{"type":"string","format":"uuid","description":"The line to update or remove"},
			"line":{"type":"object","description":"The line to add, or the fields of it to change. quantity is required to add one; product_id is taken only on add.","properties":{
				"product_id":{"type":"string","format":"uuid"},
				"description":{"type":"string"},
				"quantity":{"type":"number"},
				"unit":{"type":"string"},
				"unit_price_minor":{"type":"integer"},
				"discount_pct":{"type":"number"},
				"tax_rate":{"type":"number"},
				"position":{"type":"integer"},
				"billing_model":{"type":"string","enum":["one_time","recurring"]},
				"billing_interval_months":{"type":"string","enum":["1","3","6","12"]},
				"interval_count":{"type":"integer"}},"additionalProperties":false}},
			"additionalProperties":false}`),
		OutputSchema: schema(`{"type":"object","required":["offer"],"properties":{
			"offer":{"type":"object","description":"The offer after the change, with its lines and its new totals"}}}`),
	}
}

func (t changeOffer) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		Action     string          `json:"action"`
		OfferID    ids.UUID        `json:"offer_id"`
		LineItemID ids.UUID        `json:"line_item_id"`
		Line       json.RawMessage `json:"line"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	if args.Action != offerAddLine && args.LineItemID.IsZero() {
		return nil, &BadArgsError{Field: "line_item_id", Cause: errors.New("line_item_id names the line to " +
			"update or remove; read the offer for its line ids")}
	}
	if args.Action != offerRemoveLine && len(args.Line) == 0 {
		return nil, &BadArgsError{Field: "line", Cause: errors.New("line carries the line to add or the fields to change")}
	}
	var (
		offer crmcontracts.Offer
		err   error
	)
	switch args.Action {
	case offerAddLine:
		offer, err = t.lines.AddOfferLine(ctx, args.OfferID, args.Line)
	case offerUpdateLine:
		offer, err = t.lines.UpdateOfferLine(ctx, args.OfferID, args.LineItemID, args.Line)
	default:
		offer, err = t.lines.RemoveOfferLine(ctx, args.OfferID, args.LineItemID)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Offer crmcontracts.Offer `json:"offer"`
	}{offer})
}
