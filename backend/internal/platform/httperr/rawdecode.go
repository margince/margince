// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// UnmarshalOrRefusal decodes a value a handler already read, answering a bad
// one with the refusal Decode gives under field. No route then shows the
// decoder's own words.
//
//craft:ignore naked-any mirror of Decode's seam target
func UnmarshalOrRefusal(ctx context.Context, field string, raw json.RawMessage, into any) error {
	err := json.Unmarshal(raw, into)
	if err == nil {
		return nil
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		err = io.EOF
	}
	return undecodedRefusal(ctx, "", "", field, raw, into, err)
}

//craft:ignore naked-any mirror of Decode's seam target
func undecodedRefusal(ctx context.Context, method, path, field string, raw json.RawMessage, into any, err error) error {
	// A target's own unmarshaler may refuse a key (the lead patch does).
	if unknown, ok := errors.AsType[*datasource.UnknownFieldError](err); ok {
		return Validation(field, "unknown_field", unknown.Error())
	}
	if refusal := datasource.LocalizeFieldFault(raw, into, err, bodyProbe); refusal != nil {
		detail, withheld := fieldShapeDetail(refusal)
		if withheld {
			slog.WarnContext(ctx, "unnamed request-body field decode failure",
				"method", method, "path", path, "field", refusal.Field, "err", err)
		}
		return Validation(field, "malformed_json", detail)
	}
	safe, withheld := SafeDecodeError(err)
	if withheld {
		slog.WarnContext(ctx, "unnamed request-body decode failure", "method", method, "path", path, "err", err)
	}
	return Validation(field, "malformed_json", safe.Error())
}
