// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// codeNameTaken is the wire code of every settings list's duplicate-name refusal.
const codeNameTaken = "name_taken"

// NameTakenError refuses a name another entry of the same list holds. It
// unwraps to ErrConflict, so it answers 409 on every surface.
type NameTakenError struct {
	// Entry is the noun with its article, such as "a loss reason".
	Entry string
	Name  string
}

func (e *NameTakenError) Error() string {
	return fmt.Sprintf("%s called %q already exists; choose a different name", e.Entry, e.Name)
}

func (*NameTakenError) Unwrap() error { return apperrors.ErrConflict }

// MessageFault names the clash, so a screen can tell it from other conflicts.
func (e *NameTakenError) MessageFault() (code, message string) {
	return codeNameTaken, e.Error()
}

// NameTaken answers a 23505 on one of a list's name indexes as NameTakenError;
// any other error passes through.
func NameTaken(err error, entry, name string, indexes ...string) error {
	if constraint, ok := UniqueViolation(err); ok && slices.Contains(indexes, constraint) {
		return &NameTakenError{Entry: entry, Name: name}
	}
	return err
}

// NameAfter is the name a sparse patch leaves: the trimmed one the caller sent,
// or the held one. NameTaken then quotes what the caller asked for.
func NameAfter(held string, sent *string) string {
	if sent == nil {
		return held
	}
	return strings.TrimSpace(*sent)
}
