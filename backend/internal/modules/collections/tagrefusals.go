// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// codeRequired is the code of a refusal for an input the caller left out.
const codeRequired = "required"

// refuseUnknownColor names a colour outside the palette the contract lists,
// before the CHECK refuses it with no field to name.
func refuseUnknownColor(color *string) error {
	if color == nil || crmcontracts.TagColor(*color).Valid() {
		return nil
	}
	return &BadInputError{Field: "color", Code: "invalid_enum", Reason: fmt.Sprintf("%q is not a tag colour; choose one from the palette", *color)}
}

// TagNameTakenError refuses a name a live tag holds. It answers 409 with the
// holder's id, so a caller can apply that tag instead.
type TagNameTakenError struct{ ExistingID ids.UUID }

func (*TagNameTakenError) Error() string {
	return "a tag with this name already exists; apply that tag instead"
}

func (*TagNameTakenError) Unwrap() error { return apperrors.ErrConflict }

// FaultReference names the tag that holds the name, once it is known.
func (e *TagNameTakenError) FaultReference() map[string]any {
	if e.ExistingID.IsZero() {
		return map[string]any{}
	}
	return map[string]any{"existing_id": e.ExistingID.String()}
}

// Unreferenced is the conflict the status and the code come from.
func (*TagNameTakenError) Unreferenced() error { return apperrors.ErrConflict }
