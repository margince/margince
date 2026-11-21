// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/heldreason"
)

type reasonedRefusal struct{}

func (reasonedRefusal) Error() string      { return "refused" }
func (reasonedRefusal) HeldReason() string { return heldreason.SendRefused }

func TestHoldReasonForNamesTheRefusalsThatNeedARep(t *testing.T) {
	for name, tc := range map[string]struct {
		err    error
		want   string
		wanted bool
	}{
		"a mailbox that cannot send":    {&MailboxNotSendCapableError{}, HeldSenderInactive, true},
		"no mailbox connected":          {&MailboxNotConnectedError{}, HeldSenderInactive, true},
		"wrapped, no mailbox connected": {fmt.Errorf("staging: %w", &MailboxNotConnectedError{}), HeldSenderInactive, true},
		"an error that names its hold":  {fmt.Errorf("staging: %w", reasonedRefusal{}), HeldSendRefused, true},
		"a fault no hold can heal":      {errors.New("connection reset"), "", false},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := holdReasonFor(tc.err)
			if got != tc.want || ok != tc.wanted {
				t.Errorf("holdReasonFor = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.wanted)
			}
		})
	}
}
