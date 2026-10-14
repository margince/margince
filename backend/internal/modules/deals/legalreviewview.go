// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// A stored pack as one reviewer sees it.
//
// The legal_review grant opens the review: its status, note, comments and the
// deal's name. Every other fact keeps the reader's own boundary, read now.
// The deal's facts need deal.read, its money the reader's field masks, the
// company their sight of it, and the offer offer.read. A document follows the
// file's read gate, which is where the mail audience lives.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// dealSight is what one reader may see of one deal now.
type dealSight struct {
	facts   bool // deal.read on a visible row: stage, close date, owner, history
	money   bool // facts, and no field mask on the amount or currency
	company bool // facts, no mask on company_id, and the company is visible
}

// reviewerSight is this reader's sight of a page of reviews' deals, and
// whether they read offers.
type reviewerSight struct {
	deals     map[ids.UUID]dealSight
	offerRead bool
}

// readReviewerSight answers the sight for a page of reviews, at read time and
// for this reader. The pack freezes facts, never the requester's masks.
func readReviewerSight(
	ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID, companyOf map[ids.UUID]ids.UUID,
) (reviewerSight, error) {
	out := reviewerSight{deals: map[ids.UUID]dealSight{}}
	p, err := storekit.Actor(ctx)
	if err != nil || !auth.ReadGranted(ctx, "deal") {
		return out, err
	}
	visible, err := auth.VisibleSubset(ctx, tx, dealTable, dealIDs)
	if err != nil {
		return out, err
	}
	writable, err := auth.WritableSubset(ctx, tx, dealTable, dealIDs)
	if err != nil {
		return out, err
	}
	companies, err := visibleCompanies(ctx, tx, companyOf)
	if err != nil {
		return out, err
	}
	out.offerRead = auth.ReadGranted(ctx, "offer")
	for _, id := range dealIDs {
		if !visible[id] {
			continue
		}
		masked := auth.MaskedFields(p, maskObject, writable[id])
		company, hasCompany := companyOf[id]
		out.deals[id] = dealSight{
			facts:   true,
			money:   !slices.Contains(masked, dealAmountField) && !slices.Contains(masked, dealCurrencyField),
			company: hasCompany && !slices.Contains(masked, filterCompanyID) && companies[company],
		}
	}
	return out, nil
}

func visibleCompanies(ctx context.Context, tx pgx.Tx, companyOf map[ids.UUID]ids.UUID) (map[ids.UUID]bool, error) {
	if len(companyOf) == 0 || !auth.ReadGranted(ctx, "company") {
		return map[ids.UUID]bool{}, nil
	}
	companyIDs := make([]ids.UUID, 0, len(companyOf))
	for _, id := range companyOf {
		companyIDs = append(companyIDs, id)
	}
	return auth.VisibleSubset(ctx, tx, "company", companyIDs)
}

// packetNames are the pack's names, read now from their source rows. So an
// erased, anonymised or renamed source shows here as it is today.
type packetNames struct {
	deal    string
	company *string
	owner   *string
}

func readPacketNames(ctx context.Context, tx pgx.Tx, p reviewPacket) (packetNames, error) {
	var out packetNames
	err := tx.QueryRow(ctx, `
		SELECT (SELECT name FROM deal WHERE id = $1),
		       (SELECT display_name FROM company WHERE id = $2),
		       (SELECT display_name FROM app_user WHERE id = $3)`,
		p.Deal.ID, p.Deal.CompanyID, p.Deal.OwnerID).Scan(&out.deal, &out.company, &out.owner)
	if err != nil {
		return out, fmt.Errorf("read the names the review pack shows: %w", err)
	}
	return out, nil
}

// decodePacket reads a stored pack.
func decodePacket(raw []byte) (reviewPacket, error) {
	var p reviewPacket
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("decode the review pack: %w", err)
	}
	return p, nil
}

func packetDocumentIDs(p reviewPacket) []ids.UUID {
	out := make([]ids.UUID, 0, len(p.Documents))
	for _, d := range p.Documents {
		out = append(out, d.AttachmentID)
	}
	return out
}

func packetCompanyOf(p reviewPacket) map[ids.UUID]ids.UUID {
	if p.Deal.CompanyID == nil {
		return nil
	}
	return map[ids.UUID]ids.UUID{p.Deal.ID: *p.Deal.CompanyID}
}

// packetWire renders a stored pack for one reader. Without the deal's facts
// the deal reads as its name: the review is about it, and the queue names it.
func packetWire(
	p reviewPacket, names packetNames, openable map[ids.UUID]DocumentName, sight reviewerSight,
) crmcontracts.LegalReviewPacket {
	out := crmcontracts.LegalReviewPacket{
		Deal:      crmcontracts.LegalReviewPacketDeal{Id: openapiUUID(p.Deal.ID), Name: names.deal},
		History:   make([]crmcontracts.LegalReviewPacketStage, 0, len(p.History)),
		Documents: make([]crmcontracts.LegalReviewPacketDocument, 0, len(p.Documents)),
	}
	seen := sight.deals[p.Deal.ID]
	if seen.facts {
		stageID, stageName := openapiUUID(p.Deal.StageID), p.Deal.StageName
		out.Deal.StageId, out.Deal.StageName = &stageID, &stageName
		out.Deal.ExpectedCloseDate = wireDate(p.Deal.ExpectedCloseDate)
		out.Deal.OwnerId, out.Deal.OwnerName = uuidPtr(p.Deal.OwnerID), names.owner
		for _, st := range p.History {
			out.History = append(out.History, crmcontracts.LegalReviewPacketStage{StageName: st.StageName, EnteredAt: st.EnteredAt})
		}
	}
	if seen.money {
		out.Deal.AmountMinor, out.Deal.Currency = p.Deal.AmountMinor, p.Deal.Currency
		out.ValueMinor, out.ValueCurrency = p.ValueMinor, p.ValueCurrency
	}
	if seen.company {
		out.Deal.CompanyId, out.Deal.CompanyName = uuidPtr(p.Deal.CompanyID), names.company
	}
	if seen.facts && sight.offerRead {
		if p.Offer != nil {
			out.Offer = offerWire(*p.Offer, seen.money)
		}
		binding := make([]crmcontracts.LegalReviewPacketOffer, 0, len(p.Binding))
		for _, o := range p.Binding {
			binding = append(binding, *offerWire(o, seen.money))
		}
		out.Binding = &binding
	}
	for _, d := range p.Documents {
		if doc, shown := documentWire(d, openable, seen.facts); shown {
			out.Documents = append(out.Documents, doc)
		}
	}
	return out
}

// offerWire renders the offer. Its money goes only to a reader who may see the
// deal's, because the offer's total is the deal's amount by another name.
func offerWire(o packetOffer, money bool) *crmcontracts.LegalReviewPacketOffer {
	out := &crmcontracts.LegalReviewPacketOffer{
		Id: openapiUUID(o.ID), OfferNumber: o.OfferNumber, Revision: o.Revision,
		Status: o.Status, Currency: o.Currency,
		ValidUntil: wireDate(o.ValidUntil), IntroText: o.IntroText, TermsText: o.TermsText, HasPdf: o.HasPDF,
		Lines: make([]crmcontracts.LegalReviewPacketOfferLine, 0, len(o.Lines)),
	}
	if money {
		out.NetMinor, out.TaxMinor, out.GrossMinor = &o.NetMinor, &o.TaxMinor, &o.GrossMinor
	}
	for _, l := range o.Lines {
		line := crmcontracts.LegalReviewPacketOfferLine{
			Position: l.Position, Description: l.Description, Unit: l.Unit, Quantity: l.Quantity,
			DiscountPct: l.DiscountPct, TaxRate: l.TaxRate, BillingModel: l.BillingModel,
			BillingIntervalMonths: l.IntervalMonths, IntervalCount: l.IntervalCount, Staged: &l.Staged,
		}
		if money {
			line.UnitPriceMinor = &l.UnitPriceMinor
		}
		out.Lines = append(out.Lines, line)
	}
	return out
}

// documentWire renders a frozen document for one reader. One they can open
// carries its name. One from a message outside their audience is left out.
// So is any file for a reader without the deal. They do not learn of it.
// Only a deal upload removed since reads as unavailable, with no name.
func documentWire(
	d PacketDocument, openable map[ids.UUID]DocumentName, readsDeal bool,
) (crmcontracts.LegalReviewPacketDocument, bool) {
	out := crmcontracts.LegalReviewPacketDocument{
		AttachmentId: openapiUUID(d.AttachmentID), AddedAt: d.AddedAt,
		Category: crmcontracts.LegalReviewPacketDocumentCategory(d.Category),
		DocState: crmcontracts.LegalReviewPacketDocumentDocState(d.DocState),
	}
	name, ok := openable[d.AttachmentID]
	if !ok {
		return out, readsDeal && !d.FromMessage
	}
	filename := name.Filename
	out.Available, out.Title, out.Filename = true, name.Title, &filename
	out.ContentType, out.ByteSize = d.ContentType, d.ByteSize
	return out, true
}

func wireDate(t *time.Time) *openapi_types.Date {
	if t == nil {
		return nil
	}
	return &openapi_types.Date{Time: *t}
}
