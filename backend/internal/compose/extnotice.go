// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The core's half of extension.NoticeRepo: a unit tells a member about a record
// and the core decides who that is and how they are reached. The write is the
// notices store's own CreateTx on the unit's transaction. The notice, its
// audit row and its event commit with the unit's own rows or not at all.

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
	"github.com/margince/margince/backend/pkg/extension"
)

// extensionNoticesPerMinute is how many notices one unit may raise in a minute,
// across all its invocations. A role can reach many members, so the count is per
// call and not per recipient. The bound is on how often a unit may speak.
const extensionNoticesPerMinute = 60

// extensionNotices meters sends per unit. FailClosed: the bound exists to stop
// spam, and an outage of the shared counter must not lift it.
var extensionNotices = ratelimit.New("extension-notice", ratelimit.FailClosed, extensionNoticesPerMinute, time.Minute)

//nolint:ireturn // returning the published repo IS the seam: a unit holds extension.NoticeRepo, never a core type.
func (c extensionCore) Notices() extension.NoticeRepo { return extensionNoticeDoor{core: c} }

type extensionNoticeDoor struct{ core extensionCore }

// validateNotice refuses what the contract does not admit before any query runs.
func validateNotice(in extension.NoticeRequest) error {
	if (in.Member == "") == (in.Role == "") {
		return fmt.Errorf("%w: name exactly one of member and role", extension.ErrInvalid)
	}
	if in.Member != "" {
		if _, err := ids.Parse(in.Member); err != nil {
			return fmt.Errorf("%w: member is not an id", extension.ErrInvalid)
		}
	}
	if types := extension.NoticeRecordTypes(); !slices.Contains(types, in.Record.Type) {
		return fmt.Errorf("%w: record type must be %s", extension.ErrInvalid, strings.Join(types, ", "))
	}
	if _, err := ids.Parse(in.Record.ID); err != nil {
		return fmt.Errorf("%w: record id is not an id", extension.ErrInvalid)
	}
	if in.Subject == "" {
		return fmt.Errorf("%w: a notice needs a subject", extension.ErrInvalid)
	}
	return nil
}

// Send raises the notice for every live member the recipient names who can open
// the record. The caller must be able to open it too, so a unit cannot use the
// door to learn which records exist.
func (d extensionNoticeDoor) Send(ctx context.Context, in extension.NoticeRequest) (extension.NoticeSent, error) {
	if err := d.core.refuseUnattended(); err != nil {
		return extension.NoticeSent{}, err
	}
	ctx, err := d.core.invocationCtx(ctx)
	if err != nil {
		return extension.NoticeSent{}, err
	}
	if !extensionNotices.Allow(d.core.unit) {
		return extension.NoticeSent{}, extension.ErrRateLimited
	}
	if err := validateNotice(in); err != nil {
		return extension.NoticeSent{}, err
	}
	record, _ := ids.Parse(in.Record.ID)
	// A caller without read on the type is told ErrNotFound, like one who cannot
	// see the row. Either way the unit learns nothing about the record.
	seen, err := auth.VisibleTo(ctx, d.core.tx, in.Record.Type, record)
	if err != nil {
		return extension.NoticeSent{}, portRefusal(err)
	}
	if !seen {
		return extension.NoticeSent{}, extension.ErrNotFound
	}
	if err := auth.EnsureVisibleLive(ctx, d.core.tx, in.Record.Type, record); err != nil {
		return extension.NoticeSent{}, portRefusal(err)
	}
	sent, err := d.deliver(ctx, in, record)
	if err != nil {
		return extension.NoticeSent{}, portRefusal(err)
	}
	return extension.NoticeSent{Recipients: sent}, nil
}

func (d extensionNoticeDoor) deliver(ctx context.Context, in extension.NoticeRequest, record ids.UUID) (int, error) {
	candidates, err := noticeCandidates(ctx, d.core.tx, in)
	if err != nil {
		return 0, err
	}
	store := notices.NewStore(InstallationDB(d.core.deps.pool))
	sent := 0
	for _, user := range candidates {
		seat, err := identity.SeatActor(ctx, d.core.tx, user)
		if err != nil {
			return 0, err
		}
		if !seat.Permissions.Allows(in.Record.Type, principal.ActionRead) {
			continue
		}
		// The recipient's own row scope, judged now. A notice is a pointer, and
		// a pointer to a record its reader cannot open tells them it exists.
		if err := auth.EnsureVisibleLive(principal.WithActor(ctx, seat), d.core.tx, in.Record.Type, record); err != nil {
			continue
		}
		dedupe := ""
		if in.Key != "" {
			dedupe = "extension:" + d.core.unit + ":" + in.Key
		}
		_, written, err := store.CreateTxNew(ctx, d.core.tx, notices.NewNotice{
			Recipient: ids.From[ids.UserKind](user),
			Kind:      notices.KindExtension,
			Subject:   in.Subject,
			DedupeKey: dedupe,
			Target:    notices.Target{Type: in.Record.Type, ID: record},
		})
		if err != nil {
			return 0, err
		}
		// A retry under the same key writes nothing, so it tells nobody new.
		if written {
			sent++
		}
	}
	return sent, nil
}

// noticeCandidates are the live members a request names. A member who has left
// or was erased is not live, so they never come back from this query.
func noticeCandidates(ctx context.Context, tx pgx.Tx, in extension.NoticeRequest) ([]ids.UUID, error) {
	live := workforce.LiveMemberSQL("u")
	var rows pgx.Rows
	var err error
	if in.Member != "" {
		member, _ := ids.Parse(in.Member)
		rows, err = tx.Query(ctx, `SELECT u.id FROM app_user u WHERE u.id = $1 AND `+live, member)
	} else {
		rows, err = tx.Query(ctx, `
			SELECT DISTINCT u.id FROM app_user u
			  JOIN role_assignment ra ON ra.user_id = u.id
			  JOIN role r ON r.id = ra.role_id AND r.archived_at IS NULL
			 WHERE r.key = $1 AND `+live, in.Role)
	}
	if err != nil {
		return nil, fmt.Errorf("compose: resolving the recipients of a notice: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}
