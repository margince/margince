// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"math/big"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// RateValidationError is the ai module's typed 422 for a rejected model-rate
// write; the rate handlers map it to httperr.Validation on the wire.
type RateValidationError struct {
	Field   string
	Code    string
	Message string
}

func (e *RateValidationError) Error() string { return e.Message }

func rateInvalid(field, code, message string) error {
	return &RateValidationError{Field: field, Code: code, Message: message}
}

const microUSDPerMTok = 1_000_000

// UsdPerMTokToMicroUSD converts a USD-per-million-tokens decimal string
// (e.g. "5.00") into the µUSD/MTok integer the ai_model_rate table stores
// ("5.00" -> 5_000_000). It rejects a non-plain-decimal (the rational "1/3"
// and scientific "1e3" forms big.Rat also accepts), negative, or too-large
// value (exceeding int64 after scaling). Rounds half-up at µUSD.
func UsdPerMTokToMicroUSD(field, usd string) (int64, error) {
	s := strings.TrimSpace(usd)
	// 12 integer digits (not 13): the ×1e6 scale to µUSD keeps every accepted
	// value within int64, so the advertised contract pattern and the server's
	// accepted domain agree exactly (no schema-valid price is 422'd on overflow).
	// s != usd rejects surrounding whitespace the anchored pattern also rejects,
	// keeping that parity exact.
	if s != usd || !values.PlainDecimal(s, 12, 6) {
		return 0, rateInvalid(field, "rate_price_nonnegative",
			field+" must be a plain non-negative decimal (USD per 1M tokens, up to 12 integer and 6 fractional digits)")
	}
	r, _ := new(big.Rat).SetString(s)
	r.Mul(r, new(big.Rat).SetInt64(microUSDPerMTok))
	// The 6-fractional-digit cap above makes num/den exact after scaling by
	// 1e6, so the round-half-up step never actually rounds today; the guard
	// stays anyway in case that cap is ever widened.
	micro, ok := scaleRatToMicroUSD(r)
	if !ok {
		return 0, rateInvalid(field, "rate_price_too_large", field+" is too large")
	}
	return micro, nil
}

// scaleRatToMicroUSD rounds an already-scaled exact rational to the nearest
// integer, half-up, reporting false when it would not fit int64. Both
// money-scale conversions this module makes (per-MTok here, per-token in
// catalogueranking.go) scale a decimal string by a fixed power of ten and then need
// this same round-and-guard step, so the two share this helper instead of
// each carrying its own copy.
func scaleRatToMicroUSD(r *big.Rat) (int64, bool) {
	num, den := r.Num(), r.Denom()
	q := new(big.Int).Quo(num, den)
	if new(big.Int).Mul(new(big.Int).Rem(num, den), big.NewInt(2)).CmpAbs(den) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if !q.IsInt64() {
		return 0, false
	}
	return q.Int64(), true
}

// MicroUSDToUsdPerMTok formats a stored µUSD/MTok integer back to a trimmed
// USD-per-million-tokens decimal string (5_000_000 -> "5", 150_000 -> "0.15").
func MicroUSDToUsdPerMTok(micro int64) string {
	s := new(big.Rat).SetFrac(big.NewInt(micro), big.NewInt(microUSDPerMTok)).FloatString(6)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}
