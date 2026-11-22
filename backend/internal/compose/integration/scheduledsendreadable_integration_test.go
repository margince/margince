// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The list and the detail of scheduled messages answer one read rule: a
// message is its scheduling seat's alone.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// scheduledSeen answers whether the list holds the message, and whether the
// detail read finds it.
func scheduledSeen(ctx context.Context, t *testing.T, store *activities.Store, id ids.UUID) (listed, found bool) {
	t.Helper()
	list, err := store.ListScheduledSends(ctx, "")
	if err != nil {
		t.Fatalf("listing scheduled messages: %v", err)
	}
	for _, row := range list {
		listed = listed || row.ID == id
	}
	_, err = store.GetScheduledSend(ctx, id)
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reading the scheduled message: %v", err)
	}
	return listed, err == nil
}

func TestTheListAndTheDetailOfScheduledMailAnswerTheSame(t *testing.T) {
	p := setupPreflight(t)
	id := p.scheduleForTheUser(t)
	store := activities.NewStore(p.DB())
	user := uuidOf(t, p.user)
	passportless := principal.WithActor(p.scheduledReader(t, user, nil), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + user.String(), UserID: user, Permissions: AdminPerms,
	})
	for name, view := range map[string]struct {
		ctx  context.Context
		sees bool
	}{
		"the author":                      {p.scheduledReader(t, user, nil), true},
		"the author's agent with no pass": {passportless, true},
		"a second seat":                   {p.scheduledReader(t, ids.NewV7(), nil), false},
	} {
		listed, found := scheduledSeen(view.ctx, t, store, id)
		if listed != view.sees || found != view.sees {
			t.Errorf("%s: listed %v, found %v; want both %v", name, listed, found, view.sees)
		}
	}
}
