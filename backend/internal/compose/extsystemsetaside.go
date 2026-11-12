// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The system read of whether a user has set a contact aside. A nightly job
// that files its own reminders has nobody signed in, so it cannot ask the
// user's own routes. The answer is the user's own choice and nothing else
// about the contact. The rule for when a choice still holds is the Worklist
// lane's own, asked through contacts.NudgeStatesOf.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

//nolint:ireturn // the published port, as Deals.
func (r *callRuntime) SetAside() extension.SetAside { return systemSetAside{rt: r} }

type systemSetAside struct{ rt *callRuntime }

func (s systemSetAside) Contact(ctx context.Context, user extension.UserID, contact string) (extension.SetAsideState, error) {
	got, err := s.Contacts(ctx, user, []string{contact})
	if err != nil {
		return extension.SetAsideState{}, err
	}
	return got[contact], nil
}

func (s systemSetAside) Contacts(ctx context.Context, user extension.UserID, contact []string) (map[string]extension.SetAsideState, error) {
	ctx, err := s.rt.systemRead(ctx, extension.SystemReadSetAside)
	if err != nil {
		return nil, err
	}
	if len(contact) > extension.MaxResponsible {
		return nil, fmt.Errorf("%w: %d contacts asked about, at most %d", extension.ErrInvalid, len(contact), extension.MaxResponsible)
	}
	reader, err := unitID(string(user), "user")
	if err != nil {
		return nil, err
	}
	asked := make([]ids.UUID, len(contact))
	for i, raw := range contact {
		if asked[i], err = unitID(raw, "contact"); err != nil {
			return nil, err
		}
	}
	connected := false
	err = database.WithWorkspaceTx(ctx, s.rt.deps.pool, func(tx pgx.Tx) error {
		var err error
		connected, err = memberConnectedTx(ctx, tx, s.rt.unit, reader)
		return err
	})
	if err != nil {
		return nil, s.rt.systemReadFault(ctx, err)
	}
	if !connected {
		return nil, fmt.Errorf("%w: a set-aside choice is answered only for a member who connected this unit", extension.ErrForbidden)
	}
	if err := s.rt.traceSystemRead(ctx, extension.SystemReadSetAside, "contact", fmt.Sprintf("%d contacts", len(contact))); err != nil {
		return nil, err
	}
	var states map[ids.UUID]contacts.NudgeState
	err = database.WithWorkspaceTx(ctx, s.rt.deps.pool, func(tx pgx.Tx) error {
		var err error
		states, err = contacts.NudgeStatesOf(ctx, tx, reader, asked, time.Now())
		return err
	})
	if err != nil {
		return nil, s.rt.systemReadFault(ctx, err)
	}
	out := make(map[string]extension.SetAsideState, len(contact))
	for i, raw := range contact {
		out[raw] = setAsideStateOf(states[asked[i]])
	}
	return out, nil
}

func setAsideStateOf(state contacts.NudgeState) extension.SetAsideState {
	switch {
	case state.Stopped:
		return extension.SetAsideState{Kind: extension.SetAsidePermanent}
	case state.Until != nil:
		return extension.SetAsideState{Kind: extension.SetAsideUntil, Until: state.Until}
	}
	return extension.SetAsideState{Kind: extension.SetAsideNone}
}
