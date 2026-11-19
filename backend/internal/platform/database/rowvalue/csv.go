// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package rowvalue

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/margince/margince/backend/internal/shared/kernel/formulacell"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CSVCell renders a value a Reader decoded as one CSV field. Numbers,
// intervals and bit strings keep the text Postgres prints; jsonb objects,
// arrays and SQL arrays are compact JSON, guarded like any text.
//
//craft:ignore naked-any a driver row value spans every SQL type a table carries; the switch narrows each
func CSVCell(v any) (string, error) {
	switch t := v.(type) {
	case pgtype.Numeric:
		return valuerText(t)
	case pgtype.Interval:
		return valuerText(t)
	case pgtype.Bits:
		return valuerText(t)
	case []any, map[string]any:
		raw, err := CompactJSON(t)
		if err != nil {
			return "", err
		}
		return formulacell.Guard(raw), nil
	default:
		return CSVScalar(v), nil
	}
}

// valuerText is a pgtype value's Postgres text, or an empty cell for NULL.
func valuerText(v driver.Valuer) (string, error) {
	value, err := v.Value()
	if err != nil || value == nil {
		return "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("rowvalue: %T rendered as %T, want text", v, value)
	}
	return text, nil
}

// CSVScalar renders a scalar as a single CSV field. Numbers keep their sign
// without a quote. Every other value is guarded, so a new type whose text a
// user typed cannot reach the cell as a formula.
//
//craft:ignore naked-any a driver row value spans every SQL type a table carries; the switch narrows each
func CSVScalar(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case [16]byte:
		return ids.UUID(t).String()
	case []byte:
		return ByteaText(t)
	case string:
		return formulacell.Guard(t)
	case time.Time:
		return t.UTC().Format(time.RFC3339Nano)
	case bool:
		return strconv.FormatBool(t)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 32)
	case json.Number:
		return t.String()
	default:
		return formulacell.Guard(fmt.Sprint(t))
	}
}
