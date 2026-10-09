// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The field faults a deal write answers with, each mapped to its own 422 code.

// PastCloseDateError maps to 422 close_date_past (INV-CLOSE-PAST).
type PastCloseDateError struct{}

func (e *PastCloseDateError) Error() string {
	return "an open deal cannot claim a close date in the past; pick today or later"
}

// FieldFault refuses an expected close date already in the past.
func (e *PastCloseDateError) FieldFault() (field, code, message string) {
	return closeDateField, "close_date_past", e.Error()
}

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

// ClosingViaPatchError maps to 422: closing a deal, and the rate frozen with
// it, is POST /deals/{id}/advance's alone, so a patch naming them is refused
// rather than answered 200 with nothing changed.
type ClosingViaPatchError struct{ Field string }

func (e *ClosingViaPatchError) Error() string {
	return e.Field + " is set by POST /deals/{id}/advance, not by a patch"
}

// FieldFault names the closing field the patch carried.
func (e *ClosingViaPatchError) FieldFault() (field, code, message string) {
	return e.Field, "set_by_advance", e.Error()
}
