// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailprobe

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type answer struct {
	provider string
	err      error
}

func (a answer) SendCapable(context.Context, string) (bool, error) { return false, nil }

func (a answer) SendableMailbox(context.Context, ids.UUID, ids.ActivityID) (string, ids.UUID, error) {
	return a.provider, ids.Nil, a.err
}

func TestForAnswersWhetherAMailboxCanSend(t *testing.T) {
	fault := errors.New("grant table unreachable")
	for name, tc := range map[string]struct {
		authority activities.SendAuthority
		want      bool
		wantErr   error
	}{
		"no authority runs no pre-flight": {nil, true, nil},
		"a mailbox that can send":         {answer{provider: "gmail"}, true, nil},
		"a mailbox that cannot send":      {answer{}, false, nil},
		"no mailbox connected":            {answer{err: &activities.MailboxNotConnectedError{}}, false, nil},
		"a lookup fault stays a fault":    {answer{err: fault}, false, fault},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := For(tc.authority)(t.Context())
			if got != tc.want || !errors.Is(err, tc.wantErr) {
				t.Errorf("probe = (%v, %v), want (%v, %v)", got, err, tc.want, tc.wantErr)
			}
		})
	}
}
