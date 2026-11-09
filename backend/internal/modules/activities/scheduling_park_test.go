// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A refusal reaches the delivery from any provider call, bare or wrapped, or as
// the policy's own "connect a calendar" refusal. Any other failure is retried.
func TestEveryProviderRefusalIsAnAuthFailure(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want bool
	}{
		"provider":       {connector.ErrAuthRejected, true},
		"wrapped":        {fmt.Errorf("calendar: save: %w", connector.ErrAuthRejected), true},
		"policy":         {errCalendarNotConnected(), true},
		"unreachable":    {connector.ErrUnreachable, false},
		"other argument": {&SchedulingArgumentError{Field: "start", Code: "outside_availability"}, false},
		"ours":           {errors.New("keyvault: key resolution failed"), false},
	} {
		if got := calendarAuthFailure(tc.err); got != tc.want {
			t.Errorf("%s: calendarAuthFailure = %t, want %t", name, got, tc.want)
		}
	}
}
