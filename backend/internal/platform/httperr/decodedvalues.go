// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"encoding/json"
	"net/http"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// DecodedValueRefusal holds a decoded body to the rules a JSON decode cannot
// see: the date-time range, number bounds and lists that refuse repeats. Every
// decoder of a contract body calls it with the raw body, so each door answers
// with one code. owned names the paths whose bounds a later layer judges.
//
//craft:ignore naked-any the JSON deserialization seam: the decode target is whichever contract request struct the handler owns
func DecodedValueRefusal(into any, raw json.RawMessage, owned func(path string) bool) error {
	if refused := datasource.RefuseDecodedValue(into, raw, owned); refused != nil {
		return Validation(refused.Field, refused.Code, refused.Error())
	}
	return nil
}

// DecodeJudged is Decode for a body whose handler judges the number bounds at
// the paths `owned` names, and reports every problem of the body in one 422.
// The decode leaves those bounds to it, so one refusal cannot hide the rest.
// Each caller is listed in gates/decodejudged_test.go with the check that holds
// its bounds.
//
//craft:ignore naked-any the JSON deserialization seam: the decode target is whichever contract request struct the handler owns
func DecodeJudged(w http.ResponseWriter, r *http.Request, into any, owned func(path string) bool) bool {
	return decodeWrite(w, r, into, owned, false)
}

// DecodeJudgedOrRefusal is DecodeOrRefusal with DecodeJudged's `owned` paths,
// for a handler that answers an empty body itself.
//
//craft:ignore naked-any the JSON deserialization seam: the decode target is whichever contract request struct the handler owns
func DecodeJudgedOrRefusal(w http.ResponseWriter, r *http.Request, into any, owned func(path string) bool) error {
	return decodeRefusal(w, r, into, owned, false)
}
