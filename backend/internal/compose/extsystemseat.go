// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The one door of the system reads that answer for a seat. A job tick has no
// caller, so these reads name the member they run as. That member is the unit's
// service member and nobody else, so no read is bound to a colleague who may leave.
// The read then runs under that member's own grants, row scope and field masks.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
	"github.com/margince/margince/backend/pkg/extension"
)

// systemSeat is the member a seat read runs as, and the context its trace is
// written under.
type systemSeat struct {
	rt     *callRuntime
	read   extension.SystemRead
	member ids.UUID
	sys    context.Context
}

// seatRead is the only door of a read that takes actAs. It admits a job tick or
// a bus delivery that declared the read. It then requires actAs to be the
// unit's live service member, one that connected the unit's secret. It answers
// the context carrying that member's own principal. A refusal gives its class
// only, so a unit cannot tell a colleague from a service member of another unit.
func (r *callRuntime) seatRead(ctx context.Context, read extension.SystemRead, actAs extension.UserID) (context.Context, systemSeat, error) {
	sys, err := r.systemRead(ctx, read)
	if err != nil {
		return nil, systemSeat{}, err
	}
	if actAs == "" {
		return nil, systemSeat{}, fmt.Errorf("%w: actAs is required, and it is the unit's service member", extension.ErrInvalid)
	}
	member, err := unitID(string(actAs), "actAs")
	if err != nil {
		return nil, systemSeat{}, err
	}
	var admitted bool
	err = database.WithWorkspaceTx(sys, r.deps.pool, func(tx pgx.Tx) error {
		var err error
		admitted, err = serviceMemberConnectedTx(sys, tx, r.deps.composed, r.unit, member)
		return err
	})
	if err != nil {
		return nil, systemSeat{}, r.systemReadFault(sys, err)
	}
	refused := fmt.Errorf("%w: a system read runs as the service member of this unit, which has connected it", extension.ErrForbidden)
	if !admitted {
		return nil, systemSeat{}, refused
	}
	seatCtx, _, bound, err := bindSeat(sys, r.deps.pool, member)
	if err != nil {
		return nil, systemSeat{}, r.systemReadFault(sys, err)
	}
	if !bound {
		return nil, systemSeat{}, refused
	}
	return seatCtx, systemSeat{rt: r, read: read, member: member, sys: sys}, nil
}

// bindSeat binds a member's principal. It is a variable so the object-gate census
// does not count every method named Get or List as gated by this one call.
var bindSeat = workerSeat

// serviceMemberConnectedTx reports whether member is the live service member
// declared for unit and holds one of the unit's secrets.
func serviceMemberConnectedTx(ctx context.Context, tx pgx.Tx, composed *Extensions, unit string, member ids.UUID) (bool, error) {
	var service bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM app_user u WHERE u.id = $1 AND u.service_extension = $2 AND `+
		workforce.LiveMemberSQL("u")+`)`, member, unit).Scan(&service)
	if err != nil || !service {
		return false, err
	}
	return memberConnectedTx(ctx, tx, composed, unit, member)
}

// trace writes the system_log row of the read: unit, read, record and the seat
// it ran as. requested lists the keys a read was asked for, when it has any.
func (s systemSeat) trace(recordType, recordID string, requested []string) error {
	detail := map[string]any{
		traceUnitKey: s.rt.unit, traceReadKey: string(s.read), traceTypeKey: recordType, traceRecordKey: recordID,
		"act_as": s.member.String(),
	}
	if requested != nil {
		detail["requested"] = requested
	}
	err := database.WithWorkspaceTx(s.sys, s.rt.deps.pool, func(tx pgx.Tx) error {
		return logSystemRead(s.sys, tx, detail)
	})
	if err != nil {
		return s.rt.systemReadFault(s.sys, err)
	}
	return nil
}

// seatFault answers a store's refusal as a published error class, and any
// other fault as the core's own failure.
func (s systemSeat) seatFault(err error) error {
	if mapped := portRefusal(err); !errors.Is(mapped, errCoreRefused) {
		return mapped
	}
	return s.rt.systemReadFault(s.sys, err)
}
