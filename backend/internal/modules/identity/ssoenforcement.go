// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Enforced-SSO mode: an installation may switch the password path off so its
// members sign in only through their corporate directory. Admins keep the
// password path — the break-glass that stops a broken IdP from locking out the
// admins who fix it. The policy is read at login; the gate lives in Login,
// after the credential check, and it refuses with the SAME ErrBadCredentials a
// wrong password earns: a distinct answer only ever reaches a caller who just
// proved a password correct, which would let anyone verify guessed passwords
// for any member the moment the mode is on. The login screen's SSO guidance
// comes from the anonymous capabilities probe instead, so nobody needs the
// refusal to differ.

import "context"

// WithRequireSSO injects the enforced-SSO policy reader, read fresh per login so
// an admin turning the mode on or off takes effect without a restart. Unset
// leaves the password path open, the same as a policy that is off.
func (s *Service) WithRequireSSO(fn func(ctx context.Context) (bool, error)) *Service {
	s.requireSSO = fn
	return s
}

// enforcedSSO reports whether this installation has closed the password path. An
// unwired reader is "not enforced"; a read that fails is neither open nor closed
// and propagates, because answering a policy outage as "off" would re-open a
// sign-in method an admin deliberately closed.
func (s *Service) enforcedSSO(ctx context.Context) (bool, error) {
	if s.requireSSO == nil {
		return false, nil
	}
	return s.requireSSO(ctx)
}
