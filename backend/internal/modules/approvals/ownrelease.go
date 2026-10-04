// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// UndoableRelease reports whether releasing a staged proposal of this kind,
// aimed at this target type, changes only records a human can change back. A
// kind whose release reaches outside the workspace — a message sent, a page
// fetched, a webhook registered — is not undoable: what left cannot be recalled.
//
// change is the staged call itself, because a tool whose tier is dynamic is
// undoable or not by where the call RESOLVES, which only its arguments say.
type UndoableRelease func(kind, targetType string, change json.RawMessage) bool

// WithUndoableRelease installs the classification. This module cannot see which
// staged kinds egress, because the tool specs that declare it live in another
// module; compose derives it from them.
func (s *Service) WithUndoableRelease(undoable UndoableRelease) *Service {
	s.undoable = undoable
	return s
}

// ownRelease says whether a credential may approve a proposal its own human
// staged, its own included. The zero value refuses, so a service built without
// the classification keeps the stricter rule.
type ownRelease bool

const (
	ownReleaseRefused ownRelease = false
	ownReleaseAllowed ownRelease = true
)

// ownReleaseFor answers it for one proposal. A credential acts for its human
// with that human's permissions, so what the human could release in the CRM it
// may release too — but only an undoable change, because a human who sees it
// afterwards can still put it back, and only on an attended call: a
// scheduled run has nobody behind it to have asked.
func (s *Service) ownReleaseFor(ctx context.Context, a row) ownRelease {
	if _, unattended := principal.AgentRunID(ctx); unattended || s.undoable == nil {
		return ownReleaseRefused
	}
	target := ""
	if a.TargetType != nil {
		target = *a.TargetType
	}
	if !s.undoable(a.Kind, target, a.ProposedChange) {
		return ownReleaseRefused
	}
	return ownReleaseAllowed
}

// ReleasableByCaller says whether the calling credential could approve the
// proposal it is about to stage, so the answer it is handed names a move that
// works rather than one decideApproval will refuse.
func (s *Service) ReleasableByCaller(ctx context.Context, kind, targetType string, change json.RawMessage) bool {
	return s.ownReleaseFor(ctx, row{Kind: kind, TargetType: &targetType, ProposedChange: change}) == ownReleaseAllowed
}
