// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Enforced-SSO mode: an installation may switch the password path off so its
// members sign in only through their corporate directory. Admins keep the
// password path — the break-glass that stops a broken IdP from locking out the
// people who fix it. The policy is read at login; the gate lives in Login,
// after the credential check, so enforcement never answers a wrong password
// differently and becomes a password oracle.

import (
	"context"
	"errors"
)

// errSSORequired refuses a password login on an installation that has closed the
// password path. It is NOT ErrBadCredentials — the credentials were correct, and
// the caller is told to use single sign-on instead. Admins are exempt, so this
// only ever reaches an ordinary member.
var errSSORequired = errors.New("identity: single sign-on is required")

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
