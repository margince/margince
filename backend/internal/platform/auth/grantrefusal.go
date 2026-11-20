// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// grantRefusal is a gate's 403. Its text is a sentence for whoever reads the
// refusal. missing_grant names the check in the tool table's notation, so an
// agent can ask an admin for that grant without parsing English.
type grantRefusal struct{ check Check }

func refuseGrant(c Check) error { return grantRefusal{check: c} }

func (e grantRefusal) Error() string { return e.check.refusalSentence() }

func (e grantRefusal) Unwrap() error { return apperrors.ErrPermissionDenied }

func (e grantRefusal) FaultReference() map[string]any {
	return map[string]any{"missing_grant": e.check.String()}
}

func (e grantRefusal) Unreferenced() error {
	return fmt.Errorf("%s: %w", e.Error(), apperrors.ErrPermissionDenied)
}

func (c Check) refusalSentence() string {
	switch {
	case len(c.OneOf) > 0:
		grants := make([]string, len(c.OneOf))
		for i, alternative := range c.OneOf {
			grants[i] = alternative.grantPhrase()
		}
		return "your role grants none of what this needs: " + strings.Join(grants, ", or ")
	case c.Admin:
		return "only an admin may do this"
	case c.NarrowerThan != "":
		return fmt.Sprintf("this is only for a role that sees less than %s records", c.NarrowerThan)
	case c.RowScope != "":
		return fmt.Sprintf("this needs a role that sees %s records or more, and yours sees fewer", c.RowScope)
	}
	return "your role does not grant " + c.grantPhrase()
}

// grantPhrase words one check, such as "update on deal" or "admin".
func (c Check) grantPhrase() string {
	switch {
	case c.Admin:
		return "admin"
	case c.NarrowerThan != "":
		return fmt.Sprintf("a row scope narrower than %s", c.NarrowerThan)
	case c.RowScope != "":
		return fmt.Sprintf("a row scope of %s or wider", c.RowScope)
	}
	actions := make([]string, len(c.Actions))
	for i, a := range c.Actions {
		actions[i] = string(a)
	}
	object := strings.ReplaceAll(c.Object, "_", " ")
	if c.Object == anyObject {
		object = "any record type"
	}
	return strings.Join(actions, " or ") + " on " + object
}
