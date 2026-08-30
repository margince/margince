// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Undoing one audited change to an edge.
//
// `relationship` is this module's table, so the write lives here and not in the
// history surface that offers the button. That is not only the ownership rule: the
// update and the archive above already hold every semantic an edge write has —
// the person lock, the one-current-primary rule, the anchor's authority, the
// project_company exclusion, the version clause — and a reversal that wrote the
// table directly would be a second, thinner set of them.
//
// So the inverse of a create is this module's own archive and the inverse of a
// patch is this module's own patch, each carrying the entry it reverses as
// evidence. Two verbs and no third, because the third inverse — putting an
// unlinked edge back — is an un-archive no write path here performs, and the
// caller refuses it by name before reaching this file.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// EdgeFacts is what deciding a reverse needs to know about the edge itself: what
// the audit row cannot say because it was written before whatever happened next.
//
// Anchor and AnchorID travel with it because an edge's write authority is its
// ANCHOR's — an employment anchors the PERSON, whichever end the history page
// was read from — and relationshipAnchor is the one place that decides which.
type EdgeFacts struct {
	Kind     string
	Version  int64
	Archived bool
	Anchor   string
	AnchorID ids.UUID
}

// EdgeFactsForReverse reads the edge under the caller's endpoint scope, in the
// caller's transaction: the version it returns is pinned by the write, so the two
// must be one reading or the pin guards a state nobody observed.
//
// Archived rather than filtered. Reversing the line that MADE a link somebody has
// since unlinked is a refusal the reader can act on, where an absent row would
// say the entry does not exist.
func (s *Store) EdgeFactsForReverse(ctx context.Context, tx pgx.Tx, id ids.UUID) (EdgeFacts, error) {
	if err := auth.Require(ctx, "relationship", principal.ActionRead); err != nil {
		return EdgeFacts{}, err
	}
	row, err := s.visibleRelationship(ctx, tx, id)
	if err != nil {
		return EdgeFacts{}, err
	}
	anchor, anchorID, err := anchorIDOf(row)
	if err != nil {
		return EdgeFacts{}, err
	}
	return EdgeFacts{
		Kind:     row.Kind,
		Version:  row.Version,
		Archived: row.ArchivedAt != nil,
		Anchor:   anchor,
		AnchorID: anchorID,
	}, nil
}

// RefuseEdgeWrite answers the OBJECT half of the authority the inverse of one
// audited edge change asks for, and writes nothing — so the button is honest
// before anyone presses it rather than a 403 waiting to happen. The row half is
// the anchor's own writability, which the caller asks through the gate every
// record read takes.
//
// The verb follows the ENTRY and is not update for both: reversing a `create`
// archives the link, and the archive above asks delete. Asking update for both
// lights the button for every seat holding the seeded rep grid, which carries
// create, read and update on `relationship` and no delete.
//
// Both grants, because an edge annotates its anchor: without the anchor's write
// grant an edge write is an RBAC side door onto it, and that is the rule the
// store's own entry points ask at their own entry.
func (s *Store) RefuseEdgeWrite(ctx context.Context, kind, entryAction string) error {
	action, err := edgeReversalGrant(entryAction)
	if err != nil {
		return err
	}
	if err := auth.Require(ctx, "relationship", action); err != nil {
		return err
	}
	anchor, _ := relationshipAnchor(kind)
	return auth.Require(ctx, anchor, principal.ActionUpdate)
}

// edgeReversalGrant names the grant the inverse of one audited edge change is
// performed under. It reads the same pair ReverseEdge dispatches on, so the
// authority asked in advance and the write that follows cannot name different
// verbs for one entry.
func edgeReversalGrant(entryAction string) (principal.Action, error) {
	switch entryAction {
	case edgeEntryCreate:
		return principal.ActionDelete, nil
	case edgeEntryUpdate:
		return principal.ActionUpdate, nil
	default:
		return "", unreversibleEdgeAction(entryAction)
	}
}

// ReverseEdgeInput is one audited edge change and the state its inverse must be
// conditioned on.
type ReverseEdgeInput struct {
	EdgeID ids.UUID
	// Action is what the reversed entry DID: `create`, whose inverse archives the
	// edge, or `update`, whose inverse replays the image below onto it.
	Action string
	// Before is the reversed entry's own before-image, as relationshipFieldImage
	// wrote it. Empty for a create, which had no prior row.
	Before map[string]any
	// IfVersion is the EDGE's version as the decision read it. An edge write does
	// not touch either record it joins, so neither record's version moves and
	// neither can serialise two people reversing one link from opposite ends.
	IfVersion int64
	// Evidence names the entry being put back. Nil is a write that reverses
	// nothing, which this path never is.
	Evidence map[string]any
}

// The two audited edge verbs this path reverses. Named because the dispatch that
// performs an inverse and the grant that inverse is asked for must read the same
// pair.
const (
	edgeEntryCreate = "create"
	edgeEntryUpdate = "update"
)

// unreversibleEdgeAction is the fault both dispatches answer with, so a verb this
// path does not reverse is described one way rather than two.
func unreversibleEdgeAction(action string) error {
	return fmt.Errorf("people: %q is not an edge change this path reverses", action)
}

// ReverseEdge performs the inverse of one audited edge change.
func (s *Store) ReverseEdge(ctx context.Context, in ReverseEdgeInput) error {
	switch in.Action {
	case edgeEntryCreate:
		_, err := s.archiveRelationshipWithEvidence(ctx, in.EdgeID, &in.IfVersion, in.Evidence)
		return err
	case edgeEntryUpdate:
		patch, err := relationshipPatchFromImage(in.Before)
		if err != nil {
			return err
		}
		patch.IfVersion, patch.Evidence = &in.IfVersion, in.Evidence
		_, err = s.UpdateRelationship(ctx, in.EdgeID, patch)
		return err
	default:
		return unreversibleEdgeAction(in.Action)
	}
}

// relationshipPatchFromImage reads an audited edge image back as the patch that
// restates it. The keys are relationshipFieldImage's, decoded here because that
// writer lives in this module: a caller decoding them would be a second reader
// of one image, and the two would part company the first time a column moved.
//
// A key the image does not carry is left unsupplied, which the patch reads as
// "leave it". The audited pair is already narrowed to what MOVED, so an image
// missing a key is an assertion that this entry did not change it.
func relationshipPatchFromImage(image map[string]any) (UpdateRelationshipInput, error) {
	var out UpdateRelationshipInput
	var err error
	if out.Role, err = imageString(image, relationshipRoleField); err != nil {
		return UpdateRelationshipInput{}, err
	}
	if out.IsCurrentPrimary, err = imageBool(image, "is_current_primary"); err != nil {
		return UpdateRelationshipInput{}, err
	}
	if out.StartedAt, err = imageTime(image, "started_at"); err != nil {
		return UpdateRelationshipInput{}, err
	}
	if out.EndedAt, err = imageTime(image, "ended_at"); err != nil {
		return UpdateRelationshipInput{}, err
	}
	return out, nil
}

// The three decodes an edge image needs. An image arrives as jsonb, so a date is
// a string and a flag is a bool; a value of the wrong shape is a fault rather
// than a skipped field, because a silently dropped key restores a state the
// entry never held.
func imageString(image map[string]any, key string) (*string, error) {
	raw, present := image[key]
	if !present || raw == nil {
		return nil, nil //nolint:nilnil // an absent key is a field the entry did not hold, which is what a nil *string means on every update input in this tree
	}
	text, isString := raw.(string)
	if !isString {
		return nil, fmt.Errorf("people: edge image %q is %T, want a string", key, raw)
	}
	return &text, nil
}

func imageBool(image map[string]any, key string) (*bool, error) {
	raw, present := image[key]
	if !present || raw == nil {
		return nil, nil //nolint:nilnil // same as imageString: absent means the entry did not hold this field
	}
	flag, isBool := raw.(bool)
	if !isBool {
		return nil, fmt.Errorf("people: edge image %q is %T, want a bool", key, raw)
	}
	return &flag, nil
}

// imageTime reads a link's date out of an image, in EITHER spelling it may
// carry.
//
// The columns are `date`, and today's writer records them as "2024-05-06" so the
// undo path's JSON comparison against the live row agrees with itself. Images
// written before that carry "2024-05-06T00:00:00Z", and they are already in every
// deployed database — a decision a human made does not stop being reversible
// because the spelling changed under it.
func imageTime(image map[string]any, key string) (*time.Time, error) {
	text, err := imageString(image, key)
	if err != nil || text == nil {
		return nil, err
	}
	for _, layout := range []string{time.DateOnly, time.RFC3339Nano} {
		if parsed, err := time.Parse(layout, *text); err == nil {
			return &parsed, nil
		}
	}
	return nil, fmt.Errorf("people: edge image %q is not a date: %q", key, *text)
}
