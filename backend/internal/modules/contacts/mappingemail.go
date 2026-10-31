// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import openapi_types "github.com/oapi-codegen/runtime/types"

// emailArg is a contract address as the store's plain string, nil when absent.
func emailArg(e *openapi_types.Email) *string {
	if e == nil {
		return nil
	}
	email := string(*e)
	return &email
}
