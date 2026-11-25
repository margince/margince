// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package scopesubjects

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// The population kinds, spelled as the wire spells them.
const (
	KindWorkspace = "workspace"
	KindTeam      = "team"
	KindOwner     = "owner"
)

// ShapeError refuses a requested scope whose id does not fit its kind, or a
// kind that is no population. Part is "kind" or "id", so each door can name the
// field in its own wire spelling.
type ShapeError struct{ Part, Sentence string }

func (e *ShapeError) Error() string { return e.Sentence }
func (e *ShapeError) Unwrap() error { return apperrors.ErrInvalidArgument }

// ShapeFault answers the refusal for a scope of kind that does or does not name
// a subject, or nil for a scope that fits.
func ShapeFault(kind string, namesSubject bool) *ShapeError {
	switch {
	case kind == KindWorkspace && namesSubject:
		return &ShapeError{Part: "id", Sentence: "the workspace scope names no subject"}
	case kind == KindTeam && !namesSubject:
		return &ShapeError{Part: "id", Sentence: "a team scope names which team"}
	case kind == KindOwner && !namesSubject:
		return &ShapeError{Part: "id", Sentence: "an owner scope names which owner"}
	case kind != KindWorkspace && kind != KindTeam && kind != KindOwner:
		return &ShapeError{Part: "kind", Sentence: fmt.Sprintf("%q is not a population; choose owner, team or workspace", kind)}
	}
	return nil
}
