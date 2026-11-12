// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func endedTrial(ended bool, err error) Handlers {
	return Handlers{}.WithTrialLock(func(context.Context) (bool, error) { return ended, err })
}

// Once the trial has ended every change is refused, except the few that keep a
// seat able to sign in, end the lock, or answer for the data it holds.
func TestAnEndedTrialRefusesEveryChangeButTheOnesThatEndOrSurviveIt(t *testing.T) {
	h := endedTrial(true, nil)
	cases := []struct {
		method, path string
		refused      bool
	}{
		{http.MethodPost, "/v1/contacts", true},
		{http.MethodPatch, "/v1/deals/0190a000-0000-7000-8000-000000000001", true},
		{http.MethodDelete, "/v1/companies/0190a000-0000-7000-8000-000000000001", true},
		{http.MethodPost, "/v1/public/bookings", true},
		{http.MethodGet, "/v1/contacts", false},
		{http.MethodGet, "/v1/me", false},
		{http.MethodPost, changePasswordPath, false},
		{http.MethodPost, "/v1/installation/license/check", false},
		{http.MethodPost, "/v1/exports", false},
		{http.MethodPatch, "/v1/data-subject-requests/0190a000-0000-7000-8000-000000000001", false},
	}
	for _, c := range cases {
		err := h.refuseAfterTrial(context.Background(), httptest.NewRequest(c.method, c.path, nil))
		if refused := errors.Is(err, apperrors.ErrTrialEnded); refused != c.refused {
			t.Errorf("%s %s: refused = %v (err %v), want %v", c.method, c.path, refused, err, c.refused)
		}
	}
}

func TestATrialStillRunningRefusesNothing(t *testing.T) {
	for name, h := range map[string]Handlers{"no lock wired": {}, "trial running": endedTrial(false, nil)} {
		if err := h.refuseAfterTrial(context.Background(), httptest.NewRequest(http.MethodPost, "/v1/contacts", nil)); err != nil {
			t.Errorf("%s: refuseAfterTrial = %v, want nil", name, err)
		}
	}
}

// A trial that cannot be read is a server fault, not a pass: the mutation is
// refused with the read's own error rather than admitted on a guess.
func TestATrialThatCannotBeReadRefusesTheChangeWithThatError(t *testing.T) {
	broken := errors.New("database unreachable")
	err := endedTrial(false, broken).refuseAfterTrial(context.Background(), httptest.NewRequest(http.MethodPost, "/v1/contacts", nil))
	if !errors.Is(err, broken) {
		t.Errorf("refuseAfterTrial = %v, want %v", err, broken)
	}
}

// /me tells every seat about the lock, and says nothing while there is none.
func TestMeReportsTheEndedTrialOnlyOnceItHasEnded(t *testing.T) {
	if got := endedTrial(true, nil).trialEndedForMe(context.Background()); got == nil || !*got {
		t.Errorf("trialEndedForMe after the trial = %v, want true", got)
	}
	for name, h := range map[string]Handlers{
		"no lock wired":   {},
		"trial running":   endedTrial(false, nil),
		"unreadable lock": endedTrial(true, errors.New("database unreachable")),
	} {
		if got := h.trialEndedForMe(context.Background()); got != nil {
			t.Errorf("%s: trialEndedForMe = %v, want absent", name, *got)
		}
	}
}
