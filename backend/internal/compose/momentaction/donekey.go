// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DoneKey names the may-be-done question about one promise task. The contact
// page and the Worklist ask it under this key, so a Not yet given on one holds
// on the other.
func DoneKey(promise openapi_types.UUID) string {
	return "moment:may_be_done:" + promise.String()
}

// DoneFingerprint is what a Not yet to the question is tied to. It covers the
// promise, the mail that may have kept it, and who holds the task. A newer
// mail, or the task changing hands, asks again.
func DoneFingerprint(promise openapi_types.UUID, sent activities.PromiseFollowUp, holder string) string {
	at := sent.At
	return owedwork.Fingerprint([]owedwork.Mark{
		{Kind: "promise", ID: promise.String()},
		{Kind: "activity", ID: sent.EmailID.String(), At: &at},
	}) + holder
}

// HeldMarker says whether the task belongs to someone other than the reader.
// A Not yet given while a colleague held the task lifts once it is the
// reader's.
func HeldMarker(ctx context.Context, assignee *ids.UUID) string {
	if assignee == nil {
		return ":ours"
	}
	viewer, ok := principal.Actor(ctx)
	if auth.RequireHuman(ctx) != nil || !ok || viewer.UserID == (ids.UUID{}) || *assignee != viewer.UserID {
		return ":theirs"
	}
	return ":ours"
}

// DoneSetAside is one Not yet a reader gave: the contact it was given on, the
// question's key and the fingerprint it was given to.
type DoneSetAside struct {
	Contact     ids.UUID
	Key         string
	Fingerprint string
}

// DoneSetAsidesTx reads the Not yet answers the reader gave to the named
// questions. Only a human has answers: an agent carries its human's id and
// must not read their screen.
func DoneSetAsidesTx(ctx context.Context, tx pgx.Tx, keys []string) (map[DoneSetAside]bool, error) {
	out := map[DoneSetAside]bool{}
	viewer, ok := principal.Actor(ctx)
	if !ok || viewer.UserID == (ids.UUID{}) || len(keys) == 0 {
		return out, nil
	}
	if err := auth.RequireHuman(ctx); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return out, nil
		}
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT contact_id, claim_key, evidence_fingerprint FROM contact_moment_dismissal
		WHERE user_id = $1 AND claim_key = ANY($2)`, viewer.UserID, keys)
	if err != nil {
		return nil, fmt.Errorf("momentaction: reading Not yet answers: %w", err)
	}
	var answer DoneSetAside
	if _, err := pgx.ForEachRow(rows, []any{&answer.Contact, &answer.Key, &answer.Fingerprint}, func() error {
		out[answer] = true
		return nil
	}); err != nil {
		return nil, fmt.Errorf("momentaction: reading Not yet answers: %w", err)
	}
	return out, nil
}
