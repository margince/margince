// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"bytes"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
)

// textMaskedAuditTypes are the record types whose audit images carry text a
// person wrote or received. audit_log is append-only, so a credential written
// into it in clear could never be taken out again.
var textMaskedAuditTypes = map[string]bool{"activity": true}

// AuditedText is text in the form the audit trail of entityType stores it. A
// reader that compares a live column with an image compares this form.
func AuditedText(entityType, text string) string {
	if !textMaskedAuditTypes[entityType] {
		return text
	}
	return credentialmask.MaskText(text)
}

// maskAuditImage masks every string in one encoded image. Numbers decode as
// json.Number so an int64 keeps every digit, and an image with nothing to mask
// is stored exactly as it was encoded.
func maskAuditImage(entityType string, image []byte) ([]byte, error) {
	if image == nil || !textMaskedAuditTypes[entityType] {
		return image, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(image))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	masked, changed := maskImageStrings(entityType, document)
	if !changed {
		return image, nil
	}
	return json.Marshal(masked)
}

//craft:ignore naked-any one node of a decoded audit image, whichever JSON type it holds
func maskImageStrings(entityType string, node any) (any, bool) {
	switch node := node.(type) {
	case string:
		masked := AuditedText(entityType, node)
		return masked, masked != node
	case map[string]any:
		changed := false
		for key, value := range node {
			masked, maskedOne := maskImageStrings(entityType, value)
			node[key], changed = masked, changed || maskedOne
		}
		return node, changed
	case []any:
		changed := false
		for i, value := range node {
			masked, maskedOne := maskImageStrings(entityType, value)
			node[i], changed = masked, changed || maskedOne
		}
		return node, changed
	default:
		return node, false
	}
}
