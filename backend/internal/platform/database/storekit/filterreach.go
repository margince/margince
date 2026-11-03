// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// Which list filters a caller would be refused, said before they ask. It is read off
// the list's own narrowing, never a typed list, so a dial offered is a dial the list takes.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// FilterReach is one list filter as the probe sent it, and whether the caller
// is refused it.
type FilterReach struct {
	Parameter string
	Operand   string
	Refused   bool
}

// ProbeFilterReach runs each filter of set through narrow, the list's own clauses, with the
// first operand narrow accepts. A filter is refused when narrow answers 403 or a masked field.
func ProbeFilterReach[I any](ctx context.Context, set FilterSet[I], operands []string,
	narrow func(context.Context, *I) error,
) ([]FilterReach, error) {
	reach := make([]FilterReach, 0, len(set))
	for _, parameter := range set.Names() {
		probe, err := probeFilter(ctx, set, parameter, operands, narrow)
		if err != nil {
			return nil, err
		}
		reach = append(reach, probe)
	}
	return reach, nil
}

// RefusedFilters names the filters of reach the caller is refused, never nil.
func RefusedFilters(reach []FilterReach) []string {
	refused := []string{}
	for _, probe := range reach {
		if probe.Refused {
			refused = append(refused, probe.Parameter)
		}
	}
	return refused
}

func probeFilter[I any](ctx context.Context, set FilterSet[I], parameter string, operands []string,
	narrow func(context.Context, *I) error,
) (FilterReach, error) {
	fault := fmt.Errorf("list filter %s binds none of the probe operands", parameter)
	for _, operand := range operands {
		var in I
		if set.Apply(&in, map[string]string{parameter: operand}) != nil {
			continue
		}
		err := narrow(ctx, &in)
		if err == nil || refusedForReach(err) {
			return FilterReach{Parameter: parameter, Operand: operand, Refused: err != nil}, nil
		}
		fault = fmt.Errorf("probing list filter %s: %w", parameter, err)
	}
	return FilterReach{}, fault
}

// refusedForReach tells a refusal of the caller's grant or mask from a fault
// in the probe's own operand.
func refusedForReach(err error) bool {
	if masked, ok := errors.AsType[*values.ParseError](err); ok {
		return masked.Code == auth.CodeFieldMasked
	}
	return errors.Is(err, apperrors.ErrPermissionDenied)
}

// CodeOperand upper-cases a code dial and refuses one of the wrong shape: it could match
// no row, and the empty page would read as "none".
func CodeOperand(raw, field string, shaped func(string) bool, shape string) (string, error) {
	code := strings.Map(upperASCII, raw)
	if shaped(code) {
		return code, nil
	}
	return "", &values.ParseError{Field: field, Code: field + "_malformed", Message: field + " must be " + shape}
}

// ProbeAlpha2 is a country operand a reach probe may send. ISO reserves it for
// user assignment, so it names no country.
const ProbeAlpha2 = "ZZ"

// CountryOperand is CodeOperand for an ISO 3166-1 alpha-2 dial, the shape a
// company's address country is stored in.
func CountryOperand(raw, field string) (string, error) {
	return CodeOperand(raw, field, isAlpha2, "a two-letter ISO 3166-1 code, such as DE")
}

func isAlpha2(code string) bool {
	return len(code) == 2 && 'A' <= code[0] && code[0] <= 'Z' && 'A' <= code[1] && code[1] <= 'Z'
}

// upperASCII folds an ASCII letter alone. strings.ToUpper also folds some
// non-ASCII letters into ASCII ones, which would pass a non-code as a code.
func upperASCII(r rune) rune {
	if 'a' <= r && r <= 'z' {
		return r - ('a' - 'A')
	}
	return r
}
