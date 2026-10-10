// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ScopeRequiredError refuses an agent whose passport lacks the scope a call
// needs, and names the scope.
type ScopeRequiredError struct {
	// Tool names the tool that asked, empty where the refusal is for a route.
	Tool  string
	Scope principal.Scope
}

func (e *ScopeRequiredError) Error() string {
	if e.Tool == "" {
		return fmt.Sprintf("gate: this call needs scope %q: %s", e.Scope, apperrors.ErrScopeExceeded)
	}
	return fmt.Sprintf("gate: %s needs scope %q: %s", e.Tool, e.Scope, apperrors.ErrScopeExceeded)
}

// Unwrap keeps the refusal on the sentinel table's 403 row.
func (e *ScopeRequiredError) Unwrap() error { return apperrors.ErrScopeExceeded }

// MessageFault keeps the sentinel's code and names the missing permission.
func (e *ScopeRequiredError) MessageFault() (code, message string) {
	return "scope_exceeds_grantor", fmt.Sprintf(
		"this passport lacks the %q permission this call needs; ask the user to make a passport that includes it", e.Scope)
}
