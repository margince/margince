// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

const viewNameIndex = "saved_view_name_once"

type viewNameTaken struct{}

func (viewNameTaken) Error() string {
	return "another saved view already has this name; choose a different name"
}
func (viewNameTaken) Unwrap() error                    { return apperrors.ErrConflict }
func (e viewNameTaken) MessageFault() (string, string) { return "view_name_taken", e.Error() }

func savedViewNameError(err error) error {
	if index, ok := storekit.UniqueViolation(err); ok && index == viewNameIndex {
		return viewNameTaken{}
	}
	return err
}
