// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

var textBoundField = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

func textBoundFault(err error) (Fault, bool) {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok || pgErr.Code != "22001" || pgErr.ConstraintName != "text_character_bound" || !textBoundField.MatchString(pgErr.ColumnName) {
		return Fault{}, false
	}
	limit, parseErr := strconv.Atoi(pgErr.Detail)
	if parseErr != nil {
		return Fault{}, false
	}
	switch limit {
	case values.TextLabelCharacters, values.TextLineCharacters, values.TextNoteCharacters, values.TextDocumentCharacters:
	default:
		return Fault{}, false
	}
	refusal := withMax(Validation(pgErr.ColumnName, "too_long",
		fmt.Sprintf("%s holds at most %d characters", pgErr.ColumnName, limit)), limit)
	fault, classified := Classify(refusal)
	fault.InfraCause = err
	return fault, classified
}
