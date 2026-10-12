// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func purgeActor(kind principal.PrincipalType, objects map[string]principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: kind, ID: "x:" + ids.NewV7().String(), UserID: ids.NewV7(),
		SeatType:    principal.SeatFull,
		Permissions: principal.Permissions{Objects: objects, RowScope: principal.RowScopeAll},
	})
}

// A purge asks the mail grant always, and the contact grant when it will
// anonymise contacts, before it counts or destroys anything.
func TestCheckPurgeAuthorityAsksWhatThePurgeWillAsk(t *testing.T) {
	mail := principal.ObjectGrant{Read: true, Delete: true}
	readOnly := principal.ObjectGrant{Read: true}
	for name, c := range map[string]struct {
		ctx        context.Context
		anonymise  bool
		wantDenied bool
	}{
		"no delete on activity": {
			ctx:        purgeActor(principal.PrincipalHuman, map[string]principal.ObjectGrant{"activity": readOnly, "contact": mail}),
			wantDenied: true,
		},
		"activity delete, no contact delete, anonymising": {
			ctx:        purgeActor(principal.PrincipalHuman, map[string]principal.ObjectGrant{"activity": mail, "contact": readOnly}),
			anonymise:  true,
			wantDenied: true,
		},
		"activity delete, no contact delete, nothing to anonymise": {
			ctx: purgeActor(principal.PrincipalHuman, map[string]principal.ObjectGrant{"activity": mail, "contact": readOnly}),
		},
		"an agent": {
			ctx:        purgeActor(principal.PrincipalAgent, map[string]principal.ObjectGrant{"activity": mail, "contact": mail}),
			wantDenied: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := (&RetentionService{}).CheckPurgeAuthority(c.ctx, c.anonymise)
			if denied := errors.Is(err, apperrors.ErrPermissionDenied); denied != c.wantDenied {
				t.Errorf("answered %v, want denied=%v", err, c.wantDenied)
			}
		})
	}
}
