// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package customfields

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// columnText is the storage type of every type held as plain text.
const columnText = "text"

// sqlType maps a validated field type to its storage type
// (CUSTOM-FIELDS-PARAM-4). A number is numeric and round-trips as a string,
// never a float. A currency is bigint minor units, and its ISO-4217 code lives
// in the catalog row, not the column.
func sqlType(fieldType string) (string, error) {
	switch fieldType {
	case TypeText:
		return columnText, nil
	case TypeNumber:
		return "numeric", nil
	case TypeDate:
		return "date", nil
	case TypeCurrency:
		return "bigint", nil
	case TypeMultiselect:
		return "text[]", nil
	case TypePicklist:
		return columnText, nil
	case TypeBoolean:
		return "boolean", nil
	case TypeRecord, TypeURL:
		return columnText, nil
	default:
		return "", fmt.Errorf("customfields: unsupported type %q", fieldType)
	}
}

// textBound is the character limit of a free text column, and false for a type
// whose values the write checks some other way.
func textBound(fieldType string) (int, bool) {
	switch fieldType {
	case TypeText:
		return values.TextDocumentCharacters, true
	case TypeURL:
		return values.WebAddressCharacters, true
	default:
		return 0, false
	}
}
