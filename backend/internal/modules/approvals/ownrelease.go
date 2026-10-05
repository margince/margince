// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Queryer is the read handle a classification may use: the decision's own
// transaction, so a state check never opens a second connection while the
// approval row is locked.
type Queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// StagedCall is the proposal a classification is asked about: its kind, the
// record it targets (a single relink names its activity here, not in its body)
// and the staged arguments.
type StagedCall struct {
	Kind       string
	TargetType string
	TargetID   ids.UUID
	Change     json.RawMessage
}

// UndoableRelease reports whether releasing a staged proposal changes only
// records a human can change back. A kind whose release reaches outside the
// workspace — a message sent, a page fetched, a webhook registered — is not
// undoable: what left cannot be recalled.
//
// The call carries the staged arguments because a tool whose tier is dynamic is
// undoable or not by where it RESOLVES, and the Queryer lets the answer read the
// state of the records it would touch.
type UndoableRelease func(ctx context.Context, q Queryer, call StagedCall) bool

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
func (s *Service) ownReleaseFor(ctx context.Context, q Queryer, a row) ownRelease {
	if _, unattended := principal.AgentRunID(ctx); unattended || s.undoable == nil {
		return ownReleaseRefused
	}
	call := StagedCall{Kind: a.Kind, Change: a.ProposedChange}
	if a.TargetType != nil {
		call.TargetType = *a.TargetType
	}
	if a.TargetID != nil {
		call.TargetID = *a.TargetID
	}
	if !s.undoable(ctx, q, call) {
		return ownReleaseRefused
	}
	return ownReleaseAllowed
}

// ReleasableByCaller says whether the calling credential could approve the
// proposal it is about to stage. It builds the row the stager would write and
// asks the questions decide does — the decision grants and the target's own
// visibility (decidable), then the credential rules and the classification — so
// the answer an agent is handed can never offer a release the decision would
// refuse. They run in one short transaction of their own, because staging holds
// none.
func (s *Service) ReleasableByCaller(ctx context.Context, kind, targetType string, targetID ids.UUID, change json.RawMessage) bool {
	p, ok := principal.Actor(ctx)
	if !ok {
		return false
	}
	staged := row{Kind: kind, TargetType: &targetType, ProposedChange: change}
	if targetID != ids.Nil {
		staged.TargetID = &targetID
	}
	if onBehalfOf := nullUUID(p.OnBehalfOf); onBehalfOf != nil {
		user := ids.From[ids.UserKind](*onBehalfOf)
		staged.OnBehalfOf = &user
	}
	if passport := nullUUID(p.PassportID); passport != nil {
		id := ids.From[ids.PassportKind](*passport)
		staged.PassportID = &id
	}
	var releasable bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		decides, err := decidable(ctx, tx, p, staged)
		if err != nil || !decides {
			return err
		}
		releasable = agentMayDecide(p, staged, true, s.ownReleaseFor(ctx, tx, staged)) == nil
		return nil
	})
	return err == nil && releasable
}
