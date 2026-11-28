// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importrecords

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// TextFields narrows the engine's JSON-shaped row to the text a delimited file
// actually carried. Every value in it came from a CSV cell, so the map the rest
// of the import works with says so in its type.
//
//craft:ignore naked-any the engine's row is a JSON-shaped map, which this narrows
func TextFields(fields map[string]any) map[string]string {
	out := make(map[string]string, len(fields))
	for name, value := range fields {
		out[name] = strings.TrimSpace(fmt.Sprint(value))
	}
	return out
}

// storeFieldRefusal reads a store's refusal of one named field as the reason
// the report shows: the cell's own text and the sentence the store gave. A
// refusal that names no field is not a bad cell.
func storeFieldRefusal(err error, fields map[string]string, targetOf map[string]string) (migration.Reason, bool) {
	field, message, ok := faultOf(err)
	if !ok {
		return migration.Reason{}, false
	}
	if target, renamed := targetOf[field]; renamed {
		field = target
	}
	return migration.Why(migration.ReasonValueRefused, "value", fields[field], "detail", message), true
}

// faultOf is the field and sentence of a validation refusal, however the store
// raised it.
func faultOf(err error) (field, message string, ok bool) {
	if fault, found := errors.AsType[apperrors.FieldFault](err); found {
		field, _, message = fault.FieldFault()
		return field, message, true
	}
	if detailed, found := errors.AsType[*httperr.DetailedError](err); found &&
		detailed.Status == http.StatusUnprocessableEntity && len(detailed.Fields) > 0 {
		return detailed.Fields[0].Field, detailed.Fields[0].Message, true
	}
	return "", "", false
}
