// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

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
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ensureOpenBirthStage guards create: deals are born open — AdvanceDeal
// is the ONE path that derives won/lost and maintains the
// closed_at/lost_reason/FX invariants. Creating straight onto a terminal
// stage would put an "open" deal on a won column — silent forecast
// corruption, no CHECK trips.
func ensureOpenBirthStage(ctx context.Context, tx pgx.Tx, stageID ids.StageID, pipelineID ids.PipelineID) error {
	var semantic string
	err := tx.QueryRow(ctx,
		`SELECT semantic FROM stage WHERE id = $1 AND pipeline_id = $2 AND archived_at IS NULL`+
			lockLiveStageTarget,
		stageID, pipelineID).Scan(&semantic)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("resolve target stage: %w", err)
	}
	if StageSemantic(semantic).Terminal() {
		return &TerminalStageOnCreateError{Semantic: semantic}
	}
	return nil
}

// recordDealUpdate lands the write shape's audit row and its paired
// outbox events. The fan-out splits by consumer (events.md §5.3): owner
// reassignment is a first-class fact, so it emits deal.owner_changed for
// the owner transition and deal.updated only for the other fields — both
// on this request's correlation_id when they co-occur.
func recordDealUpdate(ctx context.Context, tx pgx.Tx, id ids.DealID, current crmcontracts.Deal, in UpdateDealInput, p *storekit.Patch) error {
	auditID, err := storekit.AuditWithTrail(ctx, tx, in.Trail, "deal", id.UUID, p.Before(), p.After())
	if err != nil {
		return fmt.Errorf("audit deal update: %w", err)
	}
	after := p.After()
	ownerChanged := in.OwnerID != nil && (current.OwnerId == nil || ids.UUID(*current.OwnerId) != in.OwnerID.UUID)
	if ownerChanged {
		payload := crmcontracts.PublicEventDealOwnerChanged{ToOwnerId: openapi_types.UUID(in.OwnerID.UUID)}
		if current.OwnerId != nil {
			payload.FromOwnerId = current.OwnerId
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, payload); err != nil {
			return fmt.Errorf("emit deal.owner_changed: %w", err)
		}
	}
	rest := make(map[string]any, len(after))
	for field, v := range after {
		if ownerChanged && field == "owner_id" {
			continue
		}
		rest[field] = v
	}
	if len(rest) > 0 {
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventDealUpdated{ChangedFields: rest}); err != nil {
			return fmt.Errorf("emit deal.updated: %w", err)
		}
	}
	return nil
}

// moved answers whether a supplied value differs from the one the row holds.
// A request that did not supply the field moved nothing; one that supplied the
// value already there moved nothing either, and the second is the reading a
// sparse update makes it easy to lose.
func moved[T comparable](current, supplied *T) bool {
	if supplied == nil {
		return false
	}
	return current == nil || *current != *supplied
}

// dealUpdatePatch folds the caller's sparse update onto the current row
// as a field patch. Re-pointing the deal at a company (or partner
// company) is a read of that record, so each link target must be
// visible under the caller's row scope before it lands in the patch.
func (s *Store) dealUpdatePatch(ctx context.Context, tx pgx.Tx, current crmcontracts.Deal, in UpdateDealInput) (*storekit.Patch, error) {
	p := storekit.NewPatch()
	// FORGETTING a link is a write about the record it points at, so it needs
	// the same permission naming that record would — the same rule the
	// re-attribution branch below already states, and the reason it cannot wait
	// until after the patch is built.
	if err := ensureClearedLinksVisible(ctx, tx, current, in.Clear); err != nil {
		return nil, err
	}
	// A paired clear leaves the generic path because it writes two columns, not
	// one (dealClearPairs); everything else in the list is a plain column.
	clears, clearPartner := splitDealClears(p, in.Clear, current)
	if err := storekit.ApplyClears(p, clears, clearableDealColumns(current)); err != nil {
		return nil, err
	}
	if in.Name != nil {
		p.Set(dealNameColumn, current.Name, *in.Name)
	}
	// The forecast fields are assigned only where the request actually moves
	// them. Every other column may be re-set freely — the audit diff records a
	// no-op edit and nothing reads it as an event — but these three are what
	// deal_forecast_history is keyed on, and a row saying the forecast moved on a
	// day it did not is a move a reconstruction has to explain.
	if moved(current.AmountMinor, in.AmountMinor) {
		p.Set(amountField, current.AmountMinor, *in.AmountMinor)
	}
	if moved(current.Currency, in.Currency) {
		p.Set(currencyField, current.Currency, *in.Currency)
	}
	// ARR moves the forecast the same way the one-off amount does, so it is
	// assigned under the same "only where it actually moved" rule.
	if moved(current.ExpectedArrMinor, in.ExpectedArrMinor) {
		p.Set(arrField, current.ExpectedArrMinor, *in.ExpectedArrMinor)
	}
	if err := applyDealLinkPatches(ctx, tx, current, in, p, clearPartner,
		s.installation.EnsurePartner, s.ensureProjectAttachable); err != nil {
		return nil, err
	}
	if in.Description != nil {
		p.Set("description", current.Description, *in.Description)
	}
	if in.CommercialMotion != nil {
		p.Set(filterCommercialMotion, motionOf(current), *in.CommercialMotion)
	}
	if in.Priority != nil {
		p.Set(filterPriority, priorityOf(current), *in.Priority)
	}
	if in.AcquisitionSource != nil {
		// Checked against the catalog HERE rather than at the handler, because
		// only the patch knows the deal's current value — and the rule is about
		// the change, not the value: a retired key a deal already holds survives
		// an unrelated edit, and the same key cannot be newly chosen.
		//
		// The deal row is locked FIRST, and the current value re-read under that
		// lock. `current` was read without one, so between that read and this
		// check another transaction can clear or replace the source — and the
		// "it already holds this key" exemption would then re-assign a retired
		// key nobody is allowed to choose any more. Deal before catalog, which
		// is the order every other writer here takes.
		held, err := lockedAcquisitionSource(ctx, tx, current)
		if err != nil {
			return nil, err
		}
		if err := ensureAssignableAcquisitionSource(ctx, tx, *in.AcquisitionSource, held); err != nil {
			return nil, err
		}
		p.Set(filterAcquisitionSource, held, *in.AcquisitionSource)
	}
	if in.ExpectedClose != nil {
		// INV-CLOSE-PAST (formulas §11): an open deal never claims a past
		// close date. Closed deals keep their historical dates editable.
		if string(current.Status) == "open" {
			if err := s.rejectPastCloseDate(ctx, tx, in.ExpectedClose); err != nil {
				return nil, err
			}
		}
		if current.ExpectedCloseDate == nil || !current.ExpectedCloseDate.Equal(*in.ExpectedClose) {
			p.SetDate(closeDateField, storekit.PlainDate(current.ExpectedCloseDate), in.ExpectedClose)
		}
		// A human setting the date IS the §11 confirmation — the machine's
		// provisional guess stops excluding the deal from Commit. This one turns
		// on the request, not on the date moving: re-sending the provisional date
		// unchanged is exactly how a human confirms it.
		if current.CloseDateProvisional != nil && *current.CloseDateProvisional {
			p.Set("close_date_provisional", true, false)
		}
	}
	if in.ForecastCategory != nil {
		p.Set("forecast_category", current.ForecastCategory, *in.ForecastCategory)
	}
	if in.WaitUntil != nil {
		p.SetDate("wait_until", storekit.PlainDate(current.WaitUntil), in.WaitUntil)
	}
	return p, nil
}

// applyPartnerAttributionPatch writes the partner link and what that partner
// did for the deal as ONE fact, because the schema stores them as one: the
// deal_partner_attribution_pairing CHECK rejects either half alone.
//
// Three shapes reach here. Naming a partner with no attribution means
// "sourced" — that is what the link meant for every row written before the
// column existed, so the default keeps old and new callers saying the same
// thing. Naming an attribution with no partner is refused rather than
// defaulted: there is no partner to attribute it to, and inventing one is
// worse than saying no. Re-attributing a deal that already names a partner
// leaves the link alone and moves only the claim.
func applyPartnerAttributionPatch(ctx context.Context, tx pgx.Tx,
	current crmcontracts.Deal, in UpdateDealInput, p *storekit.Patch, clearPartner bool,
	ensurePartner EnsurePartner,
) error {
	if clearPartner {
		// A request that forgets the partner while naming what they did has
		// nobody left to attribute it to, which is the refusal an attribution
		// standing alone already earns.
		if in.PartnerAttribution != nil {
			return &PartnerAttributionUnpairedError{}
		}
		p.Set("partner_company_id", current.PartnerCompanyId, nil)
		p.Set("partner_attribution", current.PartnerAttribution, nil)
		return nil
	}
	if in.PartnerAttribution != nil {
		if err := validPartnerAttribution(*in.PartnerAttribution); err != nil {
			return err
		}
	}
	if in.PartnerCompanyID == nil {
		if in.PartnerAttribution == nil {
			return nil
		}
		// An attribution alone is only meaningful when the deal already
		// names the partner it describes.
		if current.PartnerCompanyId == nil {
			return &PartnerAttributionUnpairedError{}
		}
		// Re-attributing is a write ABOUT that partner, so it needs the same
		// permission naming them would: a caller who can no longer open the
		// company — it became capture-private after the link was made —
		// may not change what the deal claims they did.
		if err := auth.EnsureLinkTarget(ctx, tx, "company", ids.UUID(*current.PartnerCompanyId)); err != nil {
			return err
		}
		p.Set("partner_attribution", current.PartnerAttribution, *in.PartnerAttribution)
		return nil
	}
	if err := auth.EnsureLinkTarget(ctx, tx, "company", in.PartnerCompanyID.UUID); err != nil {
		return err
	}
	// Visible is not enough: it must actually BE a partner, or the deal reads
	// as credited to somebody the accrual can never price.
	if err := ensurePartner(ctx, tx, *in.PartnerCompanyID); err != nil {
		return err
	}
	p.Set("partner_company_id", current.PartnerCompanyId, *in.PartnerCompanyID)
	p.Set("partner_attribution", current.PartnerAttribution, resolvedAttribution(current, in))
	return nil
}

// resolvedAttribution decides what a deal that names a partner claims about
// them. An explicit attribution wins.
//
// Otherwise the claim is "sourced", including when the deal already carried a
// different one: an attribution describes a PARTNER, so it does not follow the
// deal to whoever is named next. Carrying "influenced" over from the previous
// partner would quietly decide that the new one — who may well have brought
// the deal — earns nothing, on the strength of a claim made about somebody
// else. Re-attributing without moving the partner is the separate path above.
func resolvedAttribution(current crmcontracts.Deal, in UpdateDealInput) string {
	if in.PartnerAttribution != nil {
		return *in.PartnerAttribution
	}
	if samePartner(current, in) && current.PartnerAttribution != nil {
		// Naming the partner the deal already has is not a change of partner,
		// so the claim already made about them stands.
		return string(*current.PartnerAttribution)
	}
	return attributionSourced
}

// samePartner reports whether the update names the partner the deal already
// carries, rather than pointing it at a different one.
func samePartner(current crmcontracts.Deal, in UpdateDealInput) bool {
	return current.PartnerCompanyId != nil && in.PartnerCompanyID != nil &&
		ids.UUID(*current.PartnerCompanyId) == in.PartnerCompanyID.UUID
}

// validPartnerAttribution keeps the vocabulary refusal in the store, where it
// produces a 422 naming the field, rather than letting the row hit the CHECK
// constraint and surface as an opaque database error.
func validPartnerAttribution(v string) error {
	if v != attributionSourced && v != attributionInfluenced {
		return &PartnerAttributionValueError{Got: v}
	}
	return nil
}

// applyMoneyInvariants enforces the amount/currency rules on the
// RESULTING row, not just the request. The pair comes together or not at
// all: an amount stranded without a currency would skip the FX freeze at
// close and then violate deal_closed_fx. Re-pricing a CLOSED deal carries
// the freeze freezeBaseRate states.
func (s *Store) applyMoneyInvariants(ctx context.Context, tx pgx.Tx,
	current crmcontracts.Deal, in UpdateDealInput, p *storekit.Patch,
) error {
	// Read off the PATCH, not off the request. A clear writes its null into the
	// patch before this runs and never appears on the input struct at all, so a
	// resulting row derived from `in` alone would still hold every figure the
	// caller just cleared — and the pairing would then be checked against a row
	// that is not the one about to be written.
	after := p.After()
	resultingAmount := patchedMoney(after, amountField, current.AmountMinor)
	resultingArr := patchedMoney(after, arrField, current.ExpectedArrMinor)
	resultingCurrency := current.Currency
	if v, ok := after[currencyField]; ok {
		resultingCurrency = nil
		if code, isString := v.(string); isString {
			resultingCurrency = &code
		}
	}
	if err := moneyPairError(resultingAmount, resultingArr, resultingCurrency); err != nil {
		return err
	}

	// Keyed on what the PATCH carries for the MONEY, not on what the request
	// supplied and not on the forecast fields at large: a request that re-sent
	// the price the deal already has re-prices nothing, and a slipped close date
	// is not a re-price at all. Either would otherwise re-freeze, and the frozen
	// rate is supposed to answer for the close.
	_, amountMoved := p.After()[amountField]
	_, currencyMoved := p.After()[currencyField]
	// Restatement is judged on what the REQUEST carried, not on what the patch
	// wrote. A caller who sends the same numeral back under a new currency has
	// restated the figure deliberately — they are saying 5000 is still the
	// price, now in yen — and the patch records no move for it because the
	// integer did not change. Judging on the patch would refuse exactly that
	// caller, who did the one thing this rule asks for.
	if err := currencyRestatementError(current, resultingCurrency, moneyRestatement{
		Currency: currencyMoved,
		Amount:   in.AmountMinor != nil,
		Arr:      in.ExpectedArrMinor != nil,
	}); err != nil {
		return err
	}
	// While an accepted offer states the recurring figure, the figure is the
	// offer's. An ordinary edit that would move or clear it is refused rather
	// than silently overwriting what the signed document says — accepting
	// another offer is how it changes, and that path replaces the figure and
	// its provenance together.
	_, arrMoved := after[arrField]
	if err := refuseManualArrEdit(current, resultingArr, arrEditMove{Arr: arrMoved, Currency: currencyMoved}); err != nil {
		return err
	}
	if string(current.Status) != "open" && resultingAmount != nil && (amountMoved || currencyMoved) {
		// deal_closed_at guarantees ClosedAt on a non-open row.
		rateBefore, rateDateBefore := frozenBefore(current)
		if err := s.freezeBaseRate(ctx, tx, p, current.Id, string(*resultingCurrency),
			*resultingAmount, *current.ClosedAt, rateBefore, rateDateBefore); err != nil {
			return fmt.Errorf("re-freeze fx for closed deal: %w", err)
		}
	}
	return nil
}

// rejectPastCloseDate is the write-layer half of INV-CLOSE-PAST: saving
// expected_close_date earlier than today (in the workspace zone,
// data-semantics §2 r4) on an open deal is an invalid state, not a
// hygiene warning. The nightly corrector is the other half — it clears
// rows that age into the past.
func (s *Store) rejectPastCloseDate(ctx context.Context, tx pgx.Tx, expectedClose *time.Time) error {
	if expectedClose == nil {
		return nil
	}
	today, err := s.installationToday(ctx, tx)
	if err != nil {
		return err
	}
	y, m, d := expectedClose.Date()
	if time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Before(today) {
		return &PastCloseDateError{}
	}
	return nil
}

// installationToday reads "today" as the installation's reporting zone sees it
// (data-semantics §2 r4), returned as UTC midnight like every scanned
// date column.
func (s *Store) installationToday(ctx context.Context, tx pgx.Tx) (time.Time, error) {
	zone, err := s.installation.Timezone(ctx, tx)
	if err != nil {
		return time.Time{}, err
	}
	// Postgres still does the arithmetic: the zone is now a bind parameter
	// instead of a column on the row, so the DST rules and the date boundary
	// stay where they were rather than being re-derived in Go.
	var today time.Time
	if err := tx.QueryRow(ctx, `SELECT (timezone($1, now()))::date`, zone).Scan(&today); err != nil {
		return time.Time{}, fmt.Errorf("resolve the installation's today: %w", err)
	}
	return dateOnly(today), nil
}

// PastCloseDateError maps to 422 close_date_past (INV-CLOSE-PAST).
type PastCloseDateError struct{}

func (e *PastCloseDateError) Error() string {
	return "an open deal cannot claim a close date in the past; pick today or later"
}

// FieldFault refuses an expected close date already in the past.
func (e *PastCloseDateError) FieldFault() (field, code, message string) {
	return closeDateField, "close_date_past", e.Error()
}

// AmountCurrencyPairError maps to 422: amount_minor and currency come
// together or not at all (data-model §6 money rules).
// The deal's money and forecast fields, whose wire name and column name are the
// SAME word — deliberately, because the contract was written from the schema, and
// a refusal that named a different field from the column it guards would send a
// caller looking for an input they did not send. So these three do duty at both
// layers: a FieldFault names them, every p.Set on the deal row spells them, and
// forecastColumns is built from them.
//
// dealTable is the opposite case and says so: a table name and an RBAC object
// that happen to share a word are two subjects, so the constant stands for the
// table and the RBAC object stays a literal.
//
// currencyField names the wire field a money-pair refusal points at: amount and
// currency are atomic, and the currency is the half a caller can supply.
const currencyField = "currency"

// amountField is the other half of a money value.
const amountField = "amount_minor"

// arrField is the recurring figure. It shares the currency with amountField
// rather than carrying one of its own: a deal quoting its one-off price in one
// currency and its subscription in another is not a deal anyone can forecast.
const arrField = "expected_arr_minor"

// closeDateField names the column a slipped forecast moves.
const closeDateField = "expected_close_date"

// The frozen base-currency columns, which move together or not at all: a rate
// without the date it was taken on cannot be reproduced, a date without a rate
// converts nothing, and the converted amount is stored beside them because
// deriving it later would ask every reader to apply both minor-unit scales.
const (
	fxRateColumn     = "fx_rate_to_base"
	fxRateDateColumn = "fx_rate_date"
	baseAmountColumn = "amount_minor_base"
)

// The two things a partner can have done for a deal. Sourced means they
// brought it; influenced means they helped one we already had. Commission
// accrues on sourced only, which is why the difference is stored and not
// inferred.
const (
	attributionSourced    = "sourced"
	attributionInfluenced = "influenced"
)

// partnerAttributionField names the wire field both attribution refusals
// point at.
const partnerAttributionField = "partner_attribution"

// PartnerAttributionUnpairedError maps to 422: an attribution describes a
// partner, so a deal that names none has nothing to attribute.
type PartnerAttributionUnpairedError struct{}

func (e *PartnerAttributionUnpairedError) Error() string {
	return "partner_attribution needs a partner_company_id — set the partner in the same request, or clear the attribution"
}

// FieldFault refuses an attribution on a deal that names no partner.
func (e *PartnerAttributionUnpairedError) FieldFault() (field, code, message string) {
	return partnerAttributionField, "partner_attribution_unpaired", e.Error()
}

// PartnerAttributionValueError maps to 422: the vocabulary is closed.
type PartnerAttributionValueError struct{ Got string }

func (e *PartnerAttributionValueError) Error() string {
	return "partner_attribution must be " + attributionSourced + " or " + attributionInfluenced
}

// FieldFault refuses an attribution outside the two-value vocabulary.
func (e *PartnerAttributionValueError) FieldFault() (field, code, message string) {
	return partnerAttributionField, "partner_attribution_invalid", e.Error()
}

// TerminalStageOnCreateError maps to 422: create on an open stage, then
// advance — won/lost is derived, never asserted at birth.
type TerminalStageOnCreateError struct{ Semantic string }

func (e *TerminalStageOnCreateError) Error() string {
	return "deals cannot be created on a " + e.Semantic + " stage; create open, then advance"
}

// FieldFault refuses creating a deal directly into a won/lost stage.
func (e *TerminalStageOnCreateError) FieldFault() (field, code, message string) {
	return "stage_id", "terminal_stage_on_create", e.Error()
}
