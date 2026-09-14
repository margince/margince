// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// EmploymentImportError names an input the caller can correct without exposing
// a storage constraint or a provider's raw payload.
type EmploymentImportError struct{ Field, Message string }

func (e *EmploymentImportError) Error() string { return e.Message }

// FieldFault exposes a field-addressable validation problem.
func (e *EmploymentImportError) FieldFault() (string, string, string) {
	return e.Field, employmentInvalidValue, e.Message
}
