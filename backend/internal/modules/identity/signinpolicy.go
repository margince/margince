// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The sign-in policy: which providers an installation offers and whether it
// enforces SSO or MFA. Split from installationsettings.go, which owns the
// broadly-readable aggregate — this projection answers to authentication_policy
// instead, so who may sign in and how never rides on the read every role makes.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// signInPolicyReadActor names the entry read this projection performs after it
// has already admitted the caller. A SYSTEM actor for the same reason the login
// screen's read uses one: the question is what this INSTALLATION offers, not
// what this reader may see, and the reader's own authority was settled one line
// above.
const signInPolicyReadActor = "system:sign_in_policy_read"

// SignInPolicyView is the sign-in policy as its own read sees it: which providers
// the installation offers, and the two enforcement switches.
type SignInPolicyView struct {
	Providers  []string
	RequireSSO bool
	RequireMFA bool
}

// SignInPolicy answers which providers the installation offers and whether it
// enforces SSO or MFA, gated on `authentication_policy` rather than on the
// settings aggregate around it.
//
// THE GATE HERE IS THE WHOLE SECURITY OF THIS READ. The entries are defined on
// installation_settings — moving them would make every read of the aggregate
// demand this grant and take the name, timezone and currency with it, which
// every role is meant to read — so this checks the caller first and then reads
// as the installation. A system principal bypasses object RBAC entirely, so
// removing or weakening the Require below does not merely widen this endpoint,
// it removes its only gate.
func (s *InstallationSettingsStore) SignInPolicy(ctx context.Context) (SignInPolicyView, error) {
	if err := auth.Require(ctx, authenticationPolicyObject, principal.ActionRead); err != nil {
		return SignInPolicyView{}, err
	}
	// Only after the caller is admitted, and read as the installation.
	readCtx := s.asInstallation(ctx)
	chosen, err := settings.Get(readCtx, s.settings, EnabledOidcProviders)
	if err != nil {
		return SignInPolicyView{}, fmt.Errorf("identity: reading the sign-in policy: %w", err)
	}
	requireSSO, err := settings.Get(readCtx, s.settings, RequireSSO)
	if err != nil {
		return SignInPolicyView{}, fmt.Errorf("identity: reading the sign-in policy: %w", err)
	}
	requireMFA, err := settings.Get(readCtx, s.settings, RequireMFA)
	if err != nil {
		return SignInPolicyView{}, fmt.Errorf("identity: reading the sign-in policy: %w", err)
	}
	return SignInPolicyView{Providers: chosen, RequireSSO: requireSSO, RequireMFA: requireMFA}, nil
}

// SSOEnforced reports whether the installation has closed the password path — for
// a login-path gate rather than a user-facing read, so it carries no
// auth.Require: it runs where there is no principal to gate on and decides
// whether a request may proceed. Read as the installation, the same system actor
// SignInPolicy uses once its own gate has passed.
func (s *InstallationSettingsStore) SSOEnforced(ctx context.Context) (bool, error) {
	enforced, err := settings.Get(s.asInstallation(ctx), s.settings, RequireSSO)
	if err != nil {
		return false, fmt.Errorf("identity: reading the enforced-SSO policy: %w", err)
	}
	return enforced, nil
}

// MFARequired reports whether the installation makes a second factor mandatory,
// the sibling of SSOEnforced and read on the same terms — anonymous of any
// caller, so admission can decide whether a member with no factor is confined to
// the enrolment routes.
func (s *InstallationSettingsStore) MFARequired(ctx context.Context) (bool, error) {
	required, err := settings.Get(s.asInstallation(ctx), s.settings, RequireMFA)
	if err != nil {
		return false, fmt.Errorf("identity: reading the require-MFA policy: %w", err)
	}
	return required, nil
}

// asInstallation stamps the read as the installation's own system principal —
// the workspace and correlation id ride from the caller, so the read stays
// attributable to the trace that asked.
func (s *InstallationSettingsStore) asInstallation(ctx context.Context) context.Context {
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem,
		ID:   signInPolicyReadActor,
	})
}
