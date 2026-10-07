// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import "github.com/margince/margince/backend/internal/shared/apperrors"

type intervalError string

func (e intervalError) Error() string { return string(e) }
func (e intervalError) Unwrap() error { return apperrors.ErrInvalidArgument }
func (e intervalError) MessageFault() (code, message string) {
	return "reporting_interval_invalid", string(e)
}
