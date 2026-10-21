// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"encoding/json"
	"net/http"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// decodeCustomFieldBody decodes a body that takes custom-field values as extra
// top-level cf_ keys. The cf_ keys are lifted out. The rest is held to the closed
// struct's rules, nested keys included, so a misspelt field is refused by name.
//
//craft:ignore naked-any the JSON deserialization seam: the decode target is whichever contract request struct the handler owns
func decodeCustomFieldBody(r *http.Request, raw json.RawMessage, into any) error {
	body, err := datasource.SplitCustomFields(raw, into)
	if err != nil {
		return err
	}
	if err := body.RejectUnknownKeys(); err != nil {
		return Validation("body", "unknown_field", err.Error())
	}
	if err := decodeRaw(r, body.Rest(), body.Target(), nil, true); err != nil {
		return err
	}
	body.Adopt()
	return nil
}
