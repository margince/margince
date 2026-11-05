// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// How every delayed send reads its payload and reserves it against the basis
// in sendbasis.go, and what a payload names.

import (
	"context"
	"errors"
	"maps"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sendReadings bounds how often one payload is read before its send waits
// for its next attempt.
const sendReadings = 3

// sendReading is what one delayed send reads. It names the recipient, renders
// the payload without the excluded items, and says when two renderings match.
// render reports false when there is nothing worth sending.
type sendReading[P any] struct {
	seat   ids.UUID
	team   *ids.UUID
	clock  func() time.Time
	render func(excluded map[ids.UUID]bool) (P, sendRefs, bool, error)
	same   func(a, b P) bool
}

// readSendPayload renders the payload, reads the basis of what it names, and
// renders it again. It returns the payload only when both renderings agree and
// nothing is excluded. The versions the reservation holds are then the ones the
// words were read at. An item whose basis fails is excluded, and the payload is
// rendered without it.
func readSendPayload[P any](ctx context.Context, pool *pgxpool.Pool, read sendReading[P]) (P, sendBasis, bool, error) {
	var none P
	excluded := map[ids.UUID]bool{}
	for range sendReadings {
		first, refs, wanted, err := read.render(excluded)
		if err != nil || !wanted {
			return none, sendBasis{}, false, err
		}
		var basis sendBasis
		if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			var err error
			basis, err = readSendBasis(ctx, tx, read.seat, read.team, refs, read.clock())
			return err
		}); err != nil {
			return none, sendBasis{}, false, err
		}
		if len(basis.excluded) > 0 {
			maps.Copy(excluded, basis.excluded)
			continue
		}
		again, _, wanted, err := read.render(excluded)
		if err != nil || !wanted {
			return none, sendBasis{}, false, err
		}
		if read.same(first, again) {
			return first, basis, true, nil
		}
	}
	return none, sendBasis{}, false, errSendBasisChanged
}

// reserveSend reads the payload and runs reserve with it and the hold of its
// basis. reserve reports whether it won the send. A basis that moved between
// the read and the hold is read once more. A second change waits for the next
// attempt.
func reserveSend[P any](ctx context.Context, pool *pgxpool.Pool, read sendReading[P], reserve func(payload P, hold func(pgx.Tx) error) (bool, error)) (P, bool, error) {
	var none P
	for range 2 {
		payload, basis, wanted, err := readSendPayload(ctx, pool, read)
		if err != nil || !wanted {
			return none, false, err
		}
		won, err := reserve(payload, func(tx pgx.Tx) error { return holdSendBasisTx(ctx, tx, basis, read.clock) })
		if errors.Is(err, errSendBasisChanged) {
			continue
		}
		return payload, won && err == nil, err
	}
	return none, false, errSendBasisChanged
}

// reserveInOwnTx is the reservation of a send whose claim committed earlier:
// the hold alone, in its own transaction.
func reserveInOwnTx[P any](ctx context.Context, pool *pgxpool.Pool) func(P, func(pgx.Tx) error) (bool, error) {
	return func(_ P, hold func(pgx.Tx) error) (bool, error) {
		return true, database.WithWorkspaceTx(ctx, pool, hold)
	}
}

// morningItem is one work item a morning body shows.
type morningItem struct {
	source string
	id     string
	team   bool
}

// morningRefs is what a morning payload names: its body's tasks and
// decisions, and the decisions its digest quotes. An approval in both is one
// row of the basis.
func morningRefs(items []morningItem, digest digestLines) sendRefs {
	refs := digestRefs(digest)
	for _, item := range items {
		id, err := ids.Parse(item.id)
		if err != nil {
			continue
		}
		switch {
		case item.source == string(crmcontracts.WorklistItemSourceTask) && item.team:
			refs.teamTasks = append(refs.teamTasks, id)
		case item.source == string(crmcontracts.WorklistItemSourceTask):
			refs.ownTasks = append(refs.ownTasks, id)
		case item.source == string(crmcontracts.WorklistItemSourceApproval):
			refs.approvals = append(refs.approvals, id)
		}
	}
	return refs
}

// digestRefs is what a digest names: the decisions it quotes, and the
// records its other lines report. Those lines stay history; the reader must
// still be allowed to see what they name.
func digestRefs(digest digestLines) sendRefs {
	var refs sendRefs
	for _, line := range digest.quotable {
		switch {
		case line.decision && line.target.Type == notices.TargetApproval:
			refs.approvals = append(refs.approvals, line.target.ID)
		case line.target.Named():
			refs.records = append(refs.records, line.target)
		}
	}
	return refs
}
