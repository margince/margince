// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// RoleVerdict is how one call answered one role in a role sweep.
type RoleVerdict int

const (
	// VerdictOpen is an answer no gate decided, which a sweep fails on.
	VerdictOpen RoleVerdict = iota
	// VerdictAllowed is a call that ran.
	VerdictAllowed
	// VerdictRefused is a call a recorded gate stopped.
	VerdictRefused
)

// VerdictOf sorts one answer. A refusal counts only where the recorder saw a
// gate make it and the call stopped on it. That is a 403, a hidden row, or the
// 409 or 422 a gate answers in its contract's words.
func VerdictOf(got MCPResult, refusedBy []string) RoleVerdict {
	switch {
	case !got.IsError:
		return VerdictAllowed
	case len(refusedBy) > 0 && (AnswerRefused(got.Text) || AnswerNotFound(got.Text)):
		return VerdictRefused
	case answerRejected(got.Text) && slices.ContainsFunc(refusedBy, auth.RefusedInItsOwnWords):
		return VerdictRefused
	}
	return VerdictOpen
}

// answerRejected reports whether a tool answer is a 409 or a 422.
func answerRejected(text string) bool {
	return strings.Contains(text, "(conflict") || strings.Contains(text, "validation_error")
}

// AnswerRefused reports whether a tool answer is a 403.
func AnswerRefused(text string) bool { return strings.Contains(text, "permission denied") }

// AnswerNotFound reports whether a tool answer is a 404.
func AnswerNotFound(text string) bool { return strings.Contains(text, "No such record") }
