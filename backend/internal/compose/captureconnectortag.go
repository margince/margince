// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Filing what a connector captures under the connector's own word, so
// "which records came in from this source" has an answer.
//
// The import side of this already existed (csvcontexttag.go): a person picks an
// existing tag at the start of a run and every record the run creates is filed
// under it. Capture's equivalent is the CONNECTOR's word — the operator chose
// it once, on a thing that exists, rather than a machine minting one per sync
// window into a shared human-curated vocabulary.
//
// The other half of the question the ticket asked — "in this period" — is the
// record's own created_at and needs no word at all.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// connectorTagFiler applies a connector's chosen word to what capture creates.
type connectorTagFiler struct{ tags *collections.Store }

func newConnectorTagFiler(pool *pgxpool.Pool) *connectorTagFiler {
	return &connectorTagFiler{tags: collections.NewStore(InstallationDB(pool))}
}

// fileUnderConnectorTag files one newly created person under the word the
// connector that captured them was set to.
//
// Inside the create's own transaction: a record and the tag that files it land
// together or neither lands, because a crash between the two leaves a row in
// the estate that the batch it belongs to cannot find — and nothing afterwards
// knows to look for it. The same argument the import's filing makes.
//
// A nil filer files nothing. A verdict engine composed without one is a real
// shape in this tree, and it must create the person either way: the word is a
// finding aid, and refusing to create a contact because a tag could not be
// applied would trade the product's actual job for its index.
func (f *connectorTagFiler) fileUnderConnectorTag(
	ctx context.Context, tx pgx.Tx, in counterpartyCreation, personID ids.PersonID,
) error {
	if f == nil {
		return nil
	}
	tagID, err := connectorContextTag(ctx, tx, in.ActivityID, in.OwnerID)
	if err != nil {
		return err
	}
	if tagID.IsZero() {
		return nil
	}
	_, err = f.tags.ApplyTagTx(ctx, tx, tagID, "person", personID.UUID)
	if errors.Is(err, apperrors.ErrConflict) {
		// The person already carries the word — a replayed effect, or a second
		// verdict for an address the first already answered. Not a failure: the
		// record is filed exactly as asked.
		return nil
	}
	if err != nil {
		return fmt.Errorf("capture: filing %s under its connector's word: %w", personID, err)
	}
	return nil
}

// connectorContextTag answers the LIVE word the connector that captured this
// activity files under, and the zero id for none.
//
// The connection is found from the activity's own provenance rather than from a
// column on the ledger: `captured_by` carries `connector:<name>` in the
// contract's declared grammar, and a connection is one seat's mailbox for one
// provider — so the pair names exactly one row. Resolving it here rather than
// freezing it at capture time is what lets an operator's choice reach the
// records a still-open question is about to create.
//
// The join to `tag` is what makes an ARCHIVED word file nothing: a retired word
// is one the workspace took out of its vocabulary, and applying it anyway would
// keep it alive from a setting nobody looks at. It is not an error — the
// connection reports `context_tag.archived` so an operator can see why the
// filing stopped, and a capture must never fail over a word somebody archived
// after it was chosen.
func connectorContextTag(ctx context.Context, tx pgx.Tx, activityID, ownerID ids.UUID) (ids.TagID, error) {
	var tagID ids.TagID
	err := tx.QueryRow(ctx, `
		SELECT c.context_tag_id
		  FROM capture_connection c
		  JOIN tag t ON t.id = c.context_tag_id AND t.archived_at IS NULL
		  JOIN activity a ON a.id = $1
		 WHERE c.user_id = $2
		   AND c.archived_at IS NULL
		   AND a.captured_by = 'connector:' || c.provider`,
		activityID, ownerID).Scan(&tagID)
	if errors.Is(err, pgx.ErrNoRows) {
		// No connection, no word, or a word since archived — all three mean this
		// record is filed under nothing, which is the honest default.
		return ids.TagID{}, nil
	}
	if err != nil {
		return ids.TagID{}, fmt.Errorf("capture: reading the connector's word: %w", err)
	}
	return tagID, nil
}
