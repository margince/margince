// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"errors"
	"io"
	"net/http"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// DecodeClaiming is Decode for a body some of whose keys the handler judges in
// its own words. claim runs on each top-level key first, and a refusal it
// returns is written as it stands, where Decode would name the key unknown.
//
//craft:ignore naked-any the JSON deserialization seam: the decode target is whichever contract request struct the handler owns
func DecodeClaiming(w http.ResponseWriter, r *http.Request, into any, claim func(key string) error) bool {
	err := decodeClaimingRefusal(w, r, into, claim)
	if err == nil {
		return true
	}
	if errors.Is(err, io.EOF) {
		err = bodyDecodeRefusal(r, nil, into, err)
	}
	Write(w, r, err)
	return false
}

//craft:ignore naked-any the JSON deserialization seam: the decode target is whichever contract request struct the handler owns
func decodeClaimingRefusal(w http.ResponseWriter, r *http.Request, into any, claim func(key string) error) error {
	raw, err := readBody(w, r)
	if err != nil {
		return err
	}
	if err := datasource.ClaimKeys(raw, claim); err != nil {
		return err
	}
	return decodeRaw(r, raw, into, nil, false)
}
