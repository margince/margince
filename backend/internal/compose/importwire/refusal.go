// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package importwire holds the import surface's refusals, each with the values
// a screen words it from. It also types a CSV sent without a type.
package importwire

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

type refusal = map[migration.RefusalParam]string

// Problem answers a file's own failure as the caller's to fix, never as a
// server fault.
func Problem(err error) error {
	switch {
	case errors.Is(err, migration.ErrHeaderInvalid):
		return fileRefusal("header_unusable", err)
	case errors.Is(err, migration.ErrNotUTF8):
		return fileRefusal("not_utf8", err)
	case errors.Is(err, migration.ErrSourceUnreadable):
		return fileRefusal("unreadable", err)
	case errors.Is(err, migration.ErrBundleRefused):
		return fileRefusal("bundle_refused", err)
	case errors.Is(err, migration.ErrObjectNotInSource):
		return httperr.Validation(string(migration.ParamObject), "mismatch", err.Error())
	case errors.Is(err, blobstore.ErrNotFound):
		// A 500 would blame the server for a source the caller can upload again.
		return fmt.Errorf("the uploaded file behind this run is no longer stored: %w", apperrors.ErrNotFound)
	default:
		return err
	}
}

// fileRefusal is the 422 of a file the import cannot read, in the words and
// with the values its FileFault carries.
func fileRefusal(code string, err error) error {
	fault, ok := errors.AsType[*migration.FileFault](err)
	if !ok {
		return httperr.Validation("file", code, err.Error())
	}
	return httperr.Validation("file", code, fault.Message).WithParams(fault.Params)
}

// TooManyRows refuses an upload of more rows than one test run reads.
func TooManyRows(rows, limit int) error {
	return httperr.Validation("file", "too_many_rows", fmt.Sprintf(
		"This upload has %d rows, and a test run reads at most %d. Split it into smaller files and import each one.",
		rows, limit)).WithMax(limit).WithParams(migration.RefusalValues(refusal{migration.ParamRows: strconv.Itoa(rows)}))
}

// UnsupportedObject refuses an object no import lands.
func UnsupportedObject() error {
	types := migration.ImportOrder
	return httperr.Validation(string(migration.ParamObject), "unsupported", fmt.Sprintf(
		"An import takes %s or %s rows. Name the type the file holds.",
		strings.Join(types[:len(types)-1], ", "), types[len(types)-1]))
}

// DuplicateTarget refuses two columns mapped onto one field.
func DuplicateTarget(target, column, otherColumn string) error {
	return httperr.Validation("mapping", "duplicate_target",
		fmt.Sprintf("The columns “%s” and “%s” both map to “%s”. Map one column to each field.", otherColumn, column, target),
	).WithParams(migration.RefusalValues(refusal{
		migration.ParamTarget: target, migration.ParamColumn: column, migration.ParamOtherColumn: otherColumn,
	}))
}

// UnmappableKey refuses a mapping with no column to tell its rows apart. It
// names the object's key target, and refTarget beside it where the two differ.
func UnmappableKey(object, target, refTarget string) error {
	if target == refTarget {
		return httperr.Validation("source_key", "unmappable", fmt.Sprintf(
			"No column identifies each row. Map a column to “%s”, the file’s own key for each row.", target),
		).WithParams(migration.RefusalValues(refusal{migration.ParamObject: object, migration.ParamTarget: target}))
	}
	return httperr.Validation("source_key", "unmappable", fmt.Sprintf(
		"No column identifies each row. Map a column to “%s”, or to “%s” for the file’s own key.", target, refTarget),
	).WithParams(migration.RefusalValues(refusal{
		migration.ParamObject: object, migration.ParamTarget: target, migration.ParamRefTarget: refTarget,
	}))
}
