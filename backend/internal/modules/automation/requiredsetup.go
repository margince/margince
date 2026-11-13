// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import "strings"

func validateRequiredSetup(entry CatalogEntry, params map[string]any) error {
	required, ok := entry.ParamsSchema[schemaKeyRequired].([]string)
	if !ok {
		return nil
	}
	for _, key := range required {
		value := params[key]
		text, scalar := value.(string)
		if value == nil || scalar && strings.TrimSpace(text) == "" {
			return &ParamError{Field: "params." + key, Reason: "is required to finish setup"}
		}
	}
	return nil
}
