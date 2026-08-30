// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The offer lifecycle (B-E03.19): draft → sent → accepted/rejected, with
// regenerate minting the next revision. Send freezes the FX rate and the
// buyer/issuer snapshots — a sent offer is a fixed record; accept syncs
// the deal's headline amount from the accepted gross (the offer becomes
// the deal's value source, restoring forecast honesty). The 🟡 gate on
// send is transport policy (the contract's x-mcp-tool tier + the agent
// gate), not re-implemented here: a human's direct call IS the approval.

package deals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// OfferNotSentError maps to 422: accept/reject/regenerate act on a SENT
// offer — a draft has not left the workspace and a terminal offer
// already has its answer.
type OfferNotSentError struct{ Status string }

func (e *OfferNotSentError) Error() string {
	return "the offer is " + e.Status + "; this transition applies to a sent offer"
}

// FieldFault refuses a transition that presupposes the offer was sent.
func (e *OfferNotSentError) FieldFault() (field, code, message string) {
	return offerStatusField, "offer_not_sent", e.Error()
}

// SendOffer runs draft → sent: freezes fx_rate_to_base as of today (422
// when the daily rate is missing — never rate=1, RT-PR-C2), captures the
// buyer/issuer snapshots and emits offer.sent. An empty offer has
// nothing to send and is refused.
func (s *Store) SendOffer(ctx context.Context, id ids.OfferID, ifVersion *int64) (crmcontracts.Offer, error) {
	if err := auth.Require(ctx, "offer", principal.ActionUpdate); err != nil {
		return crmcontracts.Offer{}, err
	}
	var out crmcontracts.Offer
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		current, _, err := visibleOfferLocked(ctx, tx, id, storekit.LiveOnly)
		if err != nil {
			return err
		}
		if err := ensureDraft(current); err != nil {
			return err
		}
		var lineCount int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM offer_line_item WHERE offer_id = $1`, id).Scan(&lineCount); err != nil {
			return fmt.Errorf("count offer lines: %w", err)
		}
		if lineCount == 0 {
			return &OfferEmptyError{}
		}

		// Resolved ONCE for the whole send: the frozen rate and the issuer
		// snapshot must name the same basis, and two reads of the setting in
		// one transaction are two READ COMMITTED snapshots — a concurrent
		// change between them would price the offer in one currency and
		// record it in another. Nothing has frozen a rate at the first send,
		// so the settings freeze probe does not close that window either.
		base, err := s.installation.BaseCurrency(ctx, tx)
		if err != nil {
			return err
		}
		rate, rateDate, err := s.freezeFx(ctx, tx, base, current.Currency, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("freeze fx at send: %w", err)
		}
		buyer, issuer, err := s.sendSnapshots(ctx, tx, base, current)
		if err != nil {
			return err
		}

		p := storekit.NewPatch()
		p.Set("status", current.Status, "sent")
		p.Set(fxRateColumn, current.FxRateToBase, rate)
		p.SetDate(fxRateDateColumn, storekit.PlainDate(current.FxRateDate), &rateDate)
		p.Set("buyer_snapshot", nil, storekit.JSONArg(buyer))
		p.Set("issuer_snapshot", nil, storekit.JSONArg(issuer))
		if err := p.ApplyGuarded(ctx, tx, "offer", id.UUID, ifVersion); err != nil {
			return fmt.Errorf("apply send transition: %w", err)
		}

		auditID, err := storekit.Audit(ctx, tx, "update", "offer", id.UUID, p.Before(), p.After())
		if err != nil {
			return fmt.Errorf("audit offer send: %w", err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, offerSentPayload(current, rate)); err != nil {
			return fmt.Errorf("emit offer.sent: %w", err)
		}
		// An offer leaving draft is the preparation of a Handelsgeschäft
		// whether or not it closes, so the deal's correspondence qualifies now
		// (A165/ADR-0114). Sending is the ONLY transition out of draft, so
		// stamping here covers accepted, rejected, expired and superseded too:
		// each is reached through a sent offer.
		if err := s.stampCorrespondence(ctx, tx, ids.DealID{UUID: ids.UUID(current.DealId)}, BasisOfferBeyondDraft); err != nil {
			return fmt.Errorf("stamp sent offer's correspondence: %w", err)
		}
		if out, err = readOfferWithLines(ctx, tx, id, storekit.LiveOnly); err != nil {
			return fmt.Errorf("read sent offer: %w", err)
		}
		return nil
	})
	return out, err
}

// offerSentPayload builds the offer.sent wire payload from the pre-send
// offer snapshot and the FX rate frozen for this send — the ONE place that
// maps SendOffer's local values onto the published schema, so a future
// field rename shows up here (and at its call site) rather than at a
// map literal that drifts silently from the schema.
func offerSentPayload(current crmcontracts.Offer, rate string) crmcontracts.PublicEventOfferSent {
	return crmcontracts.PublicEventOfferSent{
		OfferId:      current.Id,
		DealId:       current.DealId,
		Revision:     current.Revision,
		GrossMinor:   current.GrossMinor,
		FxRateToBase: rate,
		ValidUntil:   current.ValidUntil,
	}
}

// sendSnapshots captures the buyer and issuer legal blocks at send time:
// the sent document stays truthful even when the org or workspace is
// later renamed.
func (s *Store) sendSnapshots(ctx context.Context, tx pgx.Tx, baseCurrency string,
	offer crmcontracts.Offer,
) (buyer, issuer map[string]any, err error) {
	if offer.BuyerOrgId != nil {
		var displayName string
		var legalName *string
		err := tx.QueryRow(ctx,
			`SELECT display_name, legal_name FROM organization WHERE id = $1`, offer.BuyerOrgId).
			Scan(&displayName, &legalName)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, fmt.Errorf("snapshot buyer organization: %w", err)
		}
		if err == nil {
			buyer = map[string]any{"organization_id": offer.BuyerOrgId, "display_name": displayName}
			if legalName != nil {
				buyer["legal_name"] = *legalName
			}
		}
	}
	// The currency is the caller's resolved base, not a second read: the
	// snapshot must record the basis this offer was actually priced in.
	name, err := s.installation.Name(ctx, tx)
	if err != nil {
		return nil, nil, fmt.Errorf("snapshot issuer name: %w", err)
	}
	issuer = map[string]any{"workspace_name": name, "base_currency": baseCurrency}
	return buyer, issuer, nil
}

// AcceptOffer runs sent → accepted: sets accepted_at, SYNCS the deal's
// amount_minor/currency from the accepted offer's gross (P12: one audit
// row; the paired deal.updated event rides the same correlation), and
// emits offer.accepted. Accepting re-prices the deal, so the caller
// needs the deal update grant as well as the offer's.
func (s *Store) AcceptOffer(ctx context.Context, id ids.OfferID, ifVersion *int64) (crmcontracts.Offer, error) {
	if err := auth.Require(ctx, "offer", principal.ActionUpdate); err != nil {
		return crmcontracts.Offer{}, err
	}
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return crmcontracts.Offer{}, err
	}
	var out crmcontracts.Offer
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		current, _, err := visibleOfferLocked(ctx, tx, id, storekit.LiveOnly)
		if err != nil {
			return err
		}
		if current.Status != crmcontracts.OfferStatusSent {
			return &OfferNotSentError{Status: string(current.Status)}
		}

		now := time.Now().UTC()
		p := storekit.NewPatch()
		p.Set("status", current.Status, "accepted")
		p.Set("accepted_at", nil, now)
		if err := p.ApplyGuarded(ctx, tx, "offer", id.UUID, ifVersion); err != nil {
			return fmt.Errorf("apply accept transition: %w", err)
		}

		dealID := ids.From[ids.DealKind](ids.UUID(current.DealId))
		dealChanged, err := s.syncDealAmountFromOffer(ctx, tx, dealID, current)
		if err != nil {
			return err
		}

		auditID, err := storekit.Audit(ctx, tx, "update", "offer", id.UUID, p.Before(),
			acceptedOfferImage(now, dealChanged))
		if err != nil {
			return fmt.Errorf("audit offer accept: %w", err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventOfferAccepted{
			OfferId: current.Id, DealId: current.DealId, Revision: current.Revision,
			GrossMinor: current.GrossMinor,
		}); err != nil {
			return fmt.Errorf("emit offer.accepted: %w", err)
		}
		// The paired deal.updated carries the FULL set of deal columns the sync
		// actually wrote — including the re-frozen fx_rate_to_base/fx_rate_date
		// on a closed deal — so a subscriber never retains stale
		// base-currency state. An accept that priced the deal at what it already
		// held wrote none, and emits nothing: a deal.updated naming no field
		// tells a subscriber the deal changed when it did not.
		if len(dealChanged) > 0 {
			if err := storekit.EmitEvent(ctx, tx, auditID, dealID.UUID, crmcontracts.PublicEventDealUpdated{
				ChangedFields: dealChanged,
			}); err != nil {
				return fmt.Errorf("emit paired deal.updated: %w", err)
			}
		}
		if out, err = readOfferWithLines(ctx, tx, id, storekit.LiveOnly); err != nil {
			return fmt.Errorf("read accepted offer: %w", err)
		}
		return nil
	})
	return out, err
}

// acceptedOfferImage is the accept's audit after-image: the offer's own
// transition, plus what the accept did to the DEAL.
//
// The deal's columns are prefixed because this row's entity_type is `offer`, and
// a bare `amount_minor` on it would read as the offer's own. They are taken from
// what the sync WROTE rather than from the offer's gross, so an accept onto a
// deal that already held the price records no re-price — the same fact the paired
// deal.updated declines to announce, said the same way.
func acceptedOfferImage(acceptedAt time.Time, dealChanged map[string]any) map[string]any {
	image := map[string]any{"status": "accepted", "accepted_at": acceptedAt}
	for column, value := range dealChanged {
		image["deal_"+column] = value
	}
	return image
}

// syncDealAmountFromOffer writes the accepted gross onto the deal. A
// still-open deal takes the amount as-is; a deal that already closed carries the
// re-freeze freezeBaseRate states, which is the invariant applyMoneyInvariants
// enforces on direct deal edits: re-pricing a deal is one rule whichever door
// the price arrives through.
//
// The write goes through the deal patch seam rather than its own UPDATE, so the
// deal's forecast move is recorded with it. Pricing a deal from its accepted
// offer changes no stage, which is exactly the move deal_stage_history cannot
// carry.
//
// It writes the money only where the deal does not already hold it. An accept
// that prices the deal at what it was already priced at moved no forecast, and a
// history row saying otherwise is a move a reconstruction would have to explain.
//
// It returns the deal columns the sync actually wrote — nothing when it wrote
// nothing — so the caller's paired deal.updated reports the complete delta: on a
// closed deal that includes the re-frozen fx_rate_to_base/fx_rate_date, not just
// amount_minor/currency.
func (s *Store) syncDealAmountFromOffer(ctx context.Context, tx pgx.Tx,
	dealID ids.DealID, offer crmcontracts.Offer,
) (map[string]any, error) {
	if offer.GrossMinor == nil {
		// The totals engine derives a gross for every line and send refuses an
		// offer with none, so a sent offer always carries one. Refusing rather
		// than writing NULL keeps that reasoning falsifiable: a null amount
		// beside the currency below would trip deal_amount_currency_pair, and
		// the row would fail with nothing naming the offer that did it.
		return nil, fmt.Errorf("accepted offer %s carries no gross to price deal %s with", offer.Id, dealID)
	}
	// The row lock makes the status read and the amount write below one
	// race-free unit. IncludeArchived preserves the read below, which
	// follows the deal row regardless of archived state.
	lock, err := storekit.LockRow(ctx, tx, dealTable, dealID.UUID, storekit.IncludeArchived)
	if err != nil {
		return nil, fmt.Errorf("lock deal for amount sync: %w", err)
	}
	var status string
	var closedAt *time.Time
	var amountBefore *int64
	var currencyBefore *string
	var rateBefore *string
	var rateDateBefore *time.Time
	// The frozen pair is read for its pre-image, not for a decision: re-pricing a
	// closed deal replaces a rate it already carries, and the audit diff has to
	// say which one.
	if err := tx.QueryRow(ctx,
		`SELECT status, closed_at, amount_minor, currency, fx_rate_to_base::text, fx_rate_date
		   FROM deal WHERE id = $1`,
		dealID).Scan(&status, &closedAt, &amountBefore, &currencyBefore,
		&rateBefore, &rateDateBefore); err != nil {
		return nil, fmt.Errorf("read deal for amount sync: %w", err)
	}

	// The columns are nullable and the offer's figures are not, so each half is
	// compared on its own terms. An unpriced deal and a priced one are different
	// forecasts; so are two different prices; and re-pricing at the figure the
	// deal already carries is neither.
	p := storekit.NewPatch()
	if amountBefore == nil || *amountBefore != *offer.GrossMinor {
		p.Set(amountField, amountBefore, *offer.GrossMinor)
	}
	if currencyBefore == nil || *currencyBefore != offer.Currency {
		p.Set(currencyField, currencyBefore, offer.Currency)
	}
	if p.Empty() {
		// The deal already holds this offer's figures: no write, so no history
		// row, and an empty delta for the caller's paired event to find nothing in.
		return p.After(), nil
	}
	if DealStatus(status) != DealOpen {
		// deal_closed_at guarantees closedAt on a non-open row.
		if err := s.freezeBaseRate(ctx, tx, p, offer.Currency, *closedAt, rateBefore, rateDateBefore); err != nil {
			return nil, fmt.Errorf("re-freeze fx for closed deal on accept: %w", err)
		}
	}
	if err := applyDealPatchLocked(ctx, tx, p, lock); err != nil {
		return nil, fmt.Errorf("sync deal amount from offer: %w", err)
	}
	return p.After(), nil
}

// RejectOffer runs sent → rejected. The optional reason rides the event
// and the audit trail; the row itself only records the state.
func (s *Store) RejectOffer(ctx context.Context, id ids.OfferID, reason *string, ifVersion *int64) (crmcontracts.Offer, error) {
	if err := auth.Require(ctx, "offer", principal.ActionUpdate); err != nil {
		return crmcontracts.Offer{}, err
	}
	var out crmcontracts.Offer
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		current, _, err := visibleOfferLocked(ctx, tx, id, storekit.LiveOnly)
		if err != nil {
			return err
		}
		if current.Status != crmcontracts.OfferStatusSent {
			return &OfferNotSentError{Status: string(current.Status)}
		}

		p := storekit.NewPatch()
		p.Set("status", current.Status, "rejected")
		if err := p.ApplyGuarded(ctx, tx, "offer", id.UUID, ifVersion); err != nil {
			return fmt.Errorf("apply reject transition: %w", err)
		}
		after := p.After()
		if reason != nil {
			after["reason"] = *reason
		}
		auditID, err := storekit.Audit(ctx, tx, "update", "offer", id.UUID, p.Before(), after)
		if err != nil {
			return fmt.Errorf("audit offer reject: %w", err)
		}
		payload := crmcontracts.PublicEventOfferRejected{
			OfferId: current.Id, DealId: current.DealId, Revision: current.Revision,
		}
		if reason != nil {
			payload.Reason = reason
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, payload); err != nil {
			return fmt.Errorf("emit offer.rejected: %w", err)
		}
		if out, err = readOfferWithLines(ctx, tx, id, storekit.LiveOnly); err != nil {
			return fmt.Errorf("read rejected offer: %w", err)
		}
		return nil
	})
	return out, err
}

// RegenerateOffer mints revision N+1 of a SENT offer as a fresh draft —
// header and line snapshots copied verbatim — and marks the original
// superseded. A sent offer is never mutated in place (B-E03.19); the
// produced draft is a reversible internal write (🟢) that still cannot
// leave without the send gate.
func (s *Store) RegenerateOffer(ctx context.Context, id ids.OfferID) (crmcontracts.Offer, error) {
	if err := auth.Require(ctx, "offer", principal.ActionCreate); err != nil {
		return crmcontracts.Offer{}, err
	}
	if err := auth.Require(ctx, "offer", principal.ActionUpdate); err != nil {
		return crmcontracts.Offer{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.Offer{}, err
	}

	var out crmcontracts.Offer
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		wsID := storekit.MustWorkspace(ctx)
		current, lock, err := visibleOfferLocked(ctx, tx, id, storekit.LiveOnly)
		if err != nil {
			return err
		}
		if current.Status != crmcontracts.OfferStatusSent {
			return &OfferNotSentError{Status: string(current.Status)}
		}

		nextRevision, err := nextOfferRevision(ctx, tx, wsID, *current.OfferNumber)
		if err != nil {
			return err
		}

		newID := ids.New[ids.OfferKind]()
		if err := copyOfferIntoRevision(ctx, tx, id, newID, nextRevision, by); err != nil {
			return err
		}

		supersede := storekit.NewPatch()
		supersede.Set("status", current.Status, "superseded")
		if err := supersede.ApplyLocked(ctx, tx, lock); err != nil {
			return fmt.Errorf("mark prior revision superseded: %w", err)
		}

		auditID, err := storekit.Audit(ctx, tx, "create", "offer", newID.UUID, nil, map[string]any{
			"offer_number": current.OfferNumber, "from_revision": current.Revision, "revision": nextRevision,
		})
		if err != nil {
			return fmt.Errorf("audit offer regenerate: %w", err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventOfferSuperseded{
			OfferId: current.Id, DealId: current.DealId,
			FromRevision: current.Revision, ToRevision: nextRevision,
		}); err != nil {
			return fmt.Errorf("emit offer.superseded: %w", err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, newID.UUID, crmcontracts.PublicEventOfferCreated{
			OfferId: openapi_types.UUID(newID.UUID), DealId: current.DealId, Revision: nextRevision,
			Currency: current.Currency, Source: current.Source, CapturedBy: by,
		}); err != nil {
			return fmt.Errorf("emit offer.created for new revision: %w", err)
		}
		if out, err = readOfferWithLines(ctx, tx, newID, storekit.LiveOnly); err != nil {
			return fmt.Errorf("read regenerated offer: %w", err)
		}
		return nil
	})
	return out, err
}

// nextOfferRevision mints revision N+1 for one offer number. Serialize
// the mint per offer number: two concurrent regenerations must produce
// N+1 and N+2, not collide on the unique (workspace, number, revision)
// key.
func nextOfferRevision(ctx context.Context, tx pgx.Tx, wsID ids.UUID, offerNumber string) (int, error) {
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended('offer_revision:' || $1::text || ':' || $2, 0))`,
		wsID, offerNumber); err != nil {
		return 0, fmt.Errorf("acquire revision lock: %w", err)
	}
	var nextRevision int
	if err := tx.QueryRow(ctx,
		`SELECT MAX(revision) + 1 FROM offer WHERE offer_number = $1`,
		offerNumber).Scan(&nextRevision); err != nil {
		return 0, fmt.Errorf("mint next revision: %w", err)
	}
	return nextRevision, nil
}

// copyOfferIntoRevision clones the sent offer's header and line
// snapshots verbatim into the new draft revision — the copy IS the
// snapshot semantics: nothing is re-derived from today's products.
func copyOfferIntoRevision(ctx context.Context, tx pgx.Tx, fromID, newID ids.OfferID, nextRevision int, by string) error {
	if _, err := tx.Exec(ctx,
		`INSERT INTO offer (id, deal_id, offer_number, revision, status, currency,
		                    buyer_org_id, valid_until, intro_text, terms_text,
		                    net_minor, tax_minor, gross_minor, source, captured_by)
		 SELECT $1, deal_id, offer_number, $3, 'draft', currency,
		        buyer_org_id, valid_until, intro_text, terms_text,
		        net_minor, tax_minor, gross_minor, source, $4
		 FROM offer WHERE id = $2`,
		newID, fromID, nextRevision, by); err != nil {
		return fmt.Errorf("copy offer into new revision: %w", err)
	}
	// proposal_state travels with the line: a still-staged proposal must
	// not silently become accepted (and start counting toward totals)
	// just because the offer grew a revision.
	if _, err := tx.Exec(ctx,
		`INSERT INTO offer_line_item (id, offer_id, position, product_id, description,
		                              unit, quantity, unit_price_minor, discount_pct, tax_rate, evidence, proposal_state, price_grounded)
		 SELECT uuidv7(), $2, position, product_id, description,
		        unit, quantity, unit_price_minor, discount_pct, tax_rate, evidence, proposal_state, price_grounded
		 FROM offer_line_item WHERE offer_id = $1`,
		fromID, newID); err != nil {
		return fmt.Errorf("copy lines into new revision: %w", err)
	}
	return nil
}
