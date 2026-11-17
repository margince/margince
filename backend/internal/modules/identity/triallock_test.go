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

func lockedBy(err error) Handlers {
	return Handlers{}.WithInstallationLock(func(context.Context) error { return err })
}

// A locked installation refuses every change with the lock's own reason. The
// few left open let a seat sign in, end the lock, or answer for the data it holds.
func TestALockedInstallationRefusesEveryChangeButTheOnesThatEndOrSurviveIt(t *testing.T) {
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
	for _, reason := range []error{apperrors.ErrTrialEnded, apperrors.ErrLicenseLapsed} {
		h := lockedBy(reason)
		for _, c := range cases {
			err := h.refuseWhenLocked(context.Background(), httptest.NewRequest(c.method, c.path, nil))
			if refused := errors.Is(err, reason); refused != c.refused {
				t.Errorf("%v: %s %s: refused = %v (err %v), want %v", reason, c.method, c.path, refused, err, c.refused)
			}
		}
	}
}

func TestAnUnlockedInstallationRefusesNothing(t *testing.T) {
	for name, h := range map[string]Handlers{"no lock wired": {}, "not locked": lockedBy(nil)} {
		if err := h.refuseWhenLocked(context.Background(), httptest.NewRequest(http.MethodPost, "/v1/contacts", nil)); err != nil {
			t.Errorf("%s: refuseWhenLocked = %v, want nil", name, err)
		}
	}
}

// A lock that cannot be read is a server fault, not a pass. The mutation is
// refused with the read's own error, never admitted on a guess.
func TestALockThatCannotBeReadRefusesTheChangeWithThatError(t *testing.T) {
	broken := errors.New("database unreachable")
	err := lockedBy(broken).refuseWhenLocked(context.Background(), httptest.NewRequest(http.MethodPost, "/v1/contacts", nil))
	if !errors.Is(err, broken) {
		t.Errorf("refuseWhenLocked = %v, want %v", err, broken)
	}
}

// /me tells every seat which lock holds, and says nothing while there is none.
func TestMeReportsTheLockByItsReason(t *testing.T) {
	trial, lapsed := lockedBy(apperrors.ErrTrialEnded).lockForMe(context.Background())
	if trial == nil || !*trial || lapsed != nil {
		t.Errorf("ended trial: lockForMe = (%v, %v), want (true, absent)", trial, lapsed)
	}
	trial, lapsed = lockedBy(apperrors.ErrLicenseLapsed).lockForMe(context.Background())
	if lapsed == nil || !*lapsed || trial != nil {
		t.Errorf("lapsed license: lockForMe = (%v, %v), want (absent, true)", trial, lapsed)
	}
	for name, h := range map[string]Handlers{
		"no lock wired":   {},
		"not locked":      lockedBy(nil),
		"unreadable lock": lockedBy(errors.New("database unreachable")),
	} {
		if trial, lapsed := h.lockForMe(context.Background()); trial != nil || lapsed != nil {
			t.Errorf("%s: lockForMe = (%v, %v), want both absent", name, trial, lapsed)
		}
	}
}
