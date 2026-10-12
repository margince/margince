// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// notADuplicate is the disposition the web screen's "not a duplicate" posts.
const notADuplicate = "not_a_duplicate"

// duplicateQueue binds the two queue verbs to the contacts store the web screen
// uses, so a pair dismissed from a chat card and one dismissed in the queue pass
// the same write-authority probe over both records and write the same audit row.
type duplicateQueue struct{ store *contacts.Store }

//nolint:ireturn // the seam is an interface by design: agents may not import the contacts module
func duplicateQueueSeam(db *database.DB) agents.DuplicateQueue {
	return duplicateQueue{store: contacts.NewStore(db)}
}

func (q duplicateQueue) Dismiss(ctx context.Context, candidate ids.UUID) (agents.DuplicateVerdict, error) {
	row, err := q.store.DisposeDedupeCandidate(ctx, candidate, notADuplicate, nil)
	return duplicateVerdictOf(row, err)
}

func (q duplicateQueue) Reopen(ctx context.Context, candidate ids.UUID) (agents.DuplicateVerdict, error) {
	row, err := q.store.UndoDedupeDisposition(ctx, candidate)
	return duplicateVerdictOf(row, err)
}

func duplicateVerdictOf(row contacts.DedupeCandidateRow, err error) (agents.DuplicateVerdict, error) {
	if err != nil {
		return agents.DuplicateVerdict{}, err
	}
	return agents.DuplicateVerdict{CandidateID: row.ID.String(), Disposition: row.Disposition}, nil
}
