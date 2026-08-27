// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Reversing a LINK, dispatched from the same route a record row uses.
//
// ONE route and not two, because a second one would be two answers to "undo this
// history entry" reached from one button. The path already reads correctly for an
// edge — `{entity_type}/{id}` is the record whose history is open and
// `{audit_id}` is the entry — so what changes is that the entry's own entity_type
// decides the mechanism.
//
// No SQL here, and none is coming. `relationship` is the people module's table
// and that module already owns every semantic an edge write has, so this file
// decides WHICH inverse and the seam there performs it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"

	"github.com/jackc/pgx/v5"
)

// edgeEntityType is privacy's spelling of the audit spine's entity_type for an
// edge row. The read that projects those rows and the write that reverses them
// must agree on it, so there is one constant and this side imports it.
const edgeEntityType = privacy.EdgeEntityType

// edgeActionArchive is the audited unlink. Its inverse is an un-archive, which no
// write path here performs, so it is named where the refusal is decided.
const edgeActionArchive = "archive"

// reversibleEdgeAction reports whether an edge change has an inverse this path
// performs: making a link is undone by removing it, and changing one by restating
// what it held. Removing one is not here — see edgeActionArchive.
func reversibleEdgeAction(action string) bool {
	return action == "create" || action == "update"
}

// EdgeReverser is the people-module seam that performs an edge's inverse. It is a
// port because `relationship` is that module's table: a write of it from here
// would be a second, thinner copy of the rules its own store holds.
type EdgeReverser interface {
	ReverseEdge(ctx context.Context, in people.ReverseEdgeInput) error
}

// reverseEdge puts one audited edge change back and answers with the line that
// recorded it.
//
// TWO guards, and each answers a question the other cannot.
//
// The record's If-Match says "the history screen I decided from was current",
// which is what the route requires it for, and it is verified against the PATH
// record — the record whose history was open. An edge row sits on both records it
// joins, so reversing from the person's page checks the person's version and from
// the company's page the company's.
//
// It cannot be the write's guard as well: an edge write does not touch either
// record it joins, so neither record's version moves and neither would notice a
// second person reversing the same link from the other end. The EDGE's own
// version, read by the binding evaluation immediately before the write, is what
// serialises those two.
func (s RestoreSeam) reverseEdge(ctx context.Context, entityType string, id ids.UUID, row AuditRow, ifVersion int64) (privacy.RecordHistoryEntry, error) {
	if s.edges == nil {
		return privacy.RecordHistoryEntry{}, fmt.Errorf(
			"compose: no edge writer is wired, so entry %s cannot be put back", row.ID)
	}
	facts, answer, err := s.decideEdge(ctx, entityType, id, row, ifVersion)
	if err != nil {
		return privacy.RecordHistoryEntry{}, err
	}
	if !answer.Undoable {
		return privacy.RecordHistoryEntry{}, RefusedRestore{Reason: answer.Reason, Detail: answer.Detail}
	}
	before, err := edgeImage(row.Before)
	if err != nil {
		return privacy.RecordHistoryEntry{}, err
	}
	if s.afterEdgeDecision != nil {
		s.afterEdgeDecision()
	}
	if err := s.edges.ReverseEdge(ctx, people.ReverseEdgeInput{
		EdgeID:    row.EntityID,
		Action:    row.Action,
		Before:    before,
		IfVersion: facts.Version,
		Evidence:  map[string]any{undidAuditLogID: row.ID.String()},
	}); err != nil {
		return privacy.RecordHistoryEntry{}, edgeWriteRefusal(err)
	}
	return s.readRestoreEntry(ctx, entityType, id, row.ID)
}

// edgeWriteRefusal renders the one outcome the decision could not have seen.
//
// Two people reversing ONE link from opposite ends are serialised by the edge's
// own version: the loser either finds the version moved, which is a version skew,
// or finds the link already gone, which every write path here answers as an
// absent row. Returned unchanged, that second answer tells the caller the ENTRY
// does not exist — when the entry is on their screen and what happened is that
// somebody else got there first.
func edgeWriteRefusal(err error) error {
	if !errors.Is(err, apperrors.ErrNotFound) {
		return err
	}
	return RefusedRestore{
		Reason: ReasonRecordArchived,
		Detail: "the link was removed while this was being decided",
	}
}

// decideEdge runs the binding evaluation and hands back the edge state it read.
// One reading, because the version the write pins and the state the decision was
// taken on have to be the same one: pinning a version read separately guards a
// state nobody judged.
//
// The path record's If-Match is verified in this SAME transaction as the
// decision, so the two are one answer about one moment rather than two readings
// a client cannot tell apart. The write is the people store's own transaction
// and cannot be joined to this one without that module restating a guard it does
// not own; what a caller decided on the screen is therefore bound here and what
// happens to the LINK between here and the write is bound by the edge version
// this returns. The record's version cannot move in that interval on an edge
// write at all — TestEndToEnd_anEdgeReverseDoesNotMoveEitherRecordsVersion is
// what keeps that true.
func (s RestoreSeam) decideEdge(ctx context.Context, entityType string, id ids.UUID, row AuditRow, ifVersion int64) (people.EdgeFacts, Undoability, error) {
	if s.evaluator.EdgeFacts == nil {
		return people.EdgeFacts{}, Undoability{},
			fmt.Errorf("compose: no edge reader is wired, so entry %s cannot be judged", row.ID)
	}
	var facts people.EdgeFacts
	var answer Undoability
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := recordVersionUnmoved(ctx, tx, entityType, id, ifVersion); err != nil {
			return err
		}
		var err error
		facts, err = s.evaluator.EdgeFacts(ctx, tx, row.EntityID)
		if err != nil {
			if refusal, refused := edgeScopeRefusal(err); refused {
				answer = refusal
				return nil
			}
			return err
		}
		// The evaluation is bound to THIS reading rather than taking its own.
		// Two statements in one READ COMMITTED transaction see two snapshots, so
		// a version read separately from the decision pins a state nobody judged.
		pinned := s.evaluator
		pinned.EdgeFacts = func(context.Context, pgx.Tx, ids.UUID) (people.EdgeFacts, error) {
			return facts, nil
		}
		answer, err = pinned.Evaluate(ctx, tx, row, Binding)
		return err
	})
	if errors.Is(err, apperrors.ErrVersionSkew) {
		// Unwrapped: the sentinel's own sentence is what the 409 carries, and a
		// caller reading `code: version_skew` is told to re-read and decide again
		// rather than shown this path's internal narration.
		return people.EdgeFacts{}, Undoability{}, err
	}
	if err != nil {
		return people.EdgeFacts{}, Undoability{},
			fmt.Errorf("compose: decide whether the link can be put back: %w", err)
	}
	return facts, answer, nil
}

// edgeImage decodes an audited edge image. The keys are the people module's, and
// so is the decode of each value: what travels from here is the image, unread.
func edgeImage(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var image map[string]any
	if err := json.Unmarshal(raw, &image); err != nil {
		return nil, fmt.Errorf("compose: edge image is not a JSON object: %w", err)
	}
	return image, nil
}

// edgeFieldsNoPatchCanClear names the fields whose reversal would have to write a
// NULL, which an edge patch cannot do: every field on the request is an optional
// pointer coalesced against the column, so a null reads as "not supplied". The
// write would report success and leave the field standing, which is worse than a
// refusal — the person reads the confirmation and stops looking.
//
// Judged from the audited PAIR rather than from the live row: the pair is already
// narrowed to what moved, so a key stated on both sides with a null before and a
// value after is exactly a field this entry filled in.
func edgeFieldsNoPatchCanClear(before, after json.RawMessage) ([]string, error) {
	beforeImage, err := edgeImage(before)
	if err != nil {
		return nil, err
	}
	afterImage, err := edgeImage(after)
	if err != nil {
		return nil, err
	}
	var unclearable []string
	for key, was := range beforeImage {
		if was != nil {
			continue
		}
		if now, stated := afterImage[key]; stated && now != nil {
			unclearable = append(unclearable, key)
		}
	}
	sort.Strings(unclearable)
	return unclearable, nil
}
