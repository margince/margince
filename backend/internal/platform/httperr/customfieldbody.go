// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"errors"
	"io"
	"net/http"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// DecodeCustomFields is Decode for a body whose contract takes custom-field
// values as extra top-level cf_ keys. Every other key is held to the closed
// struct's rules, so a misspelt field is refused by name, as Decode refuses it.
//
//craft:ignore naked-any the JSON deserialization seam: the decode target is whichever contract request struct the handler owns
func DecodeCustomFields(w http.ResponseWriter, r *http.Request, into any) bool {
	err := decodeCustomFieldsRefusal(w, r, into)
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
func decodeCustomFieldsRefusal(w http.ResponseWriter, r *http.Request, into any) error {
	raw, err := readBody(w, r)
	if err != nil {
		return err
	}
	body, err := datasource.SplitCustomFields(raw, into)
	if err != nil {
		return err
	}
	if err := decodeRaw(r, body.Rest(), body.Target(), nil, false); err != nil {
		return err
	}
	body.Adopt()
	return nil
}
