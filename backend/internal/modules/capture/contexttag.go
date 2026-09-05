// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The word a connector files what it captures under.
//
// An import already does this: a person picks an existing tag at the start of a
// run and every record the run creates is filed under it, so a batch stays
// findable as a batch. A record capture creates carried no equivalent, so
// "which records came in from this source" had no answer.
//
// The CONNECTOR is the batch. A mailbox polls forever, so a word per sync
// window would mint one nobody chose, filling a shared human-curated vocabulary
// with machine timestamps and making it useless for the browsing it exists for.
// The other half of the question — "in this period" — is the record's own
// created_at and needs no word at all.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// auditKeyContextTag names the word in a configuration audit's before/after
// image.
const auditKeyContextTag = "context_tag_id"

// ContextTag is a connection's chosen word as a reader needs it: the id it was
// set to, the name the vocabulary spells today, and whether that word has since
// been archived.
type ContextTag struct {
	ID   ids.UUID
	Name string
	// Archived says the workspace retired this word. The connector then files
	// NOTHING — applying a retired word would keep a vocabulary alive from a
	// setting nobody looks at — and this field is why an operator can see that
	// rather than wondering where the filing went.
	Archived bool
}

// UnknownContextTagError refuses a word the vocabulary does not carry, or one
// it has already retired.
//
// Refused HERE, at the moment it is set, and never when a record lands: a
// capture must not fail over a word somebody archived afterwards. A mailbox
// going unread is a far worse answer to a vocabulary edit than a connector
// going quiet.
type UnknownContextTagError struct{ TagID ids.UUID }

func (e *UnknownContextTagError) Error() string {
	return fmt.Sprintf("capture: %s is not a tag this workspace carries, or has been archived", e.TagID)
}

// Unwrap makes it an invalid-argument failure on the wire (422), which is what
// a caller naming a word that is not there is owed.
func (e *UnknownContextTagError) Unwrap() error { return apperrors.ErrInvalidArgument }

// contextTagOf assembles the view's optional word from the LEFT JOIN's three
// columns. All three are null together — the join found no row — which is a
// connection that chose no word rather than one whose word could not be read.
func contextTagOf(id *ids.UUID, name *string, archived *bool) *ContextTag {
	if id == nil || name == nil || archived == nil {
		return nil
	}
	return &ContextTag{ID: *id, Name: *name, Archived: *archived}
}

// SetContextTag chooses the word this human's own connection files under, or
// clears it when tagID is nil.
//
// Only their own mailbox: the write matches on the authenticated user, so there
// is no id here that reaches a colleague's connection — the same scoping
// SetMailPosture takes, and for the same reason.
//
// It governs what the connector creates AFTERWARDS. Records already captured
// keep the filing they have, because refiling a year of history under a word
// chosen today would claim they arrived with it.
func (r *Registry) SetContextTag(ctx context.Context, name string, tagID *ids.UUID) (ConnectionView, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return ConnectionView{}, fmt.Errorf("capture: only a human files a mailbox under a word: %w", apperrors.ErrPermissionDenied)
	}
	if _, ok := r.connectors[name]; !ok {
		return ConnectionView{}, ErrNoConnection
	}

	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		if tagID != nil {
			if err := requireLiveTag(ctx, tx, *tagID); err != nil {
				return err
			}
		}
		// The prior answer comes out of the same statement that replaces it, so
		// the audit's before-image is the row as it actually stood rather than a
		// value read separately that something else could have moved in between.
		var before *ids.UUID
		row := tx.QueryRow(ctx, `
			UPDATE capture_connection
			   SET context_tag_id = $3
			 WHERE user_id = $1 AND provider = $2 AND archived_at IS NULL
			RETURNING (SELECT c.context_tag_id
			             FROM capture_connection c WHERE c.id = capture_connection.id)`,
			actor.UserID, name, tagID)
		if err := row.Scan(&before); err != nil {
			return err
		}
		// Audit-only, like every other capture-configuration change beside it:
		// the closed public-event catalog carries no type for this, and
		// inventing one would put a mailbox's own setting on the outbound bus.
		_, err := storekit.Audit(ctx, tx, "update", captureSettingsObject, storekit.MustWorkspace(ctx),
			contextTagImage(name, before), contextTagImage(name, tagID))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ConnectionView{}, ErrNoConnection
	}
	if err != nil {
		return ConnectionView{}, fmt.Errorf("capture: filing this mailbox under a word: %w", err)
	}
	return r.connectionFor(ctx, name)
}

// requireLiveTag refuses a word the vocabulary does not carry or has retired.
//
// One statement for both refusals, and deliberately so: to the caller they are
// the same answer — the word they named is not one this connector can file
// under — and distinguishing them would tell an unauthorized reader which
// archived words exist.
func requireLiveTag(ctx context.Context, tx pgx.Tx, tagID ids.UUID) error {
	var live bool
	err := tx.QueryRow(ctx,
		`SELECT true FROM tag WHERE id = $1 AND archived_at IS NULL`, tagID).Scan(&live)
	if errors.Is(err, pgx.ErrNoRows) {
		return &UnknownContextTagError{TagID: tagID}
	}
	if err != nil {
		return fmt.Errorf("capture: reading the word this connector would file under: %w", err)
	}
	return nil
}

// contextTagImage renders one side of the configuration audit.
func contextTagImage(provider string, tagID *ids.UUID) map[string]any {
	image := map[string]any{auditKeyProvider: provider}
	if tagID == nil {
		// Absent rather than a zero uuid: "chose no word" and "chose the nil
		// word" are different facts, and only the first one happens.
		image[auditKeyContextTag] = nil
		return image
	}
	image[auditKeyContextTag] = tagID.String()
	return image
}
