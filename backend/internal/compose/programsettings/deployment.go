// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package programsettings

import "github.com/margince/margince/backend/internal/platform/config"

// The margince.yaml switches a program reads at boot. They carry no flag: a
// deployment states them in the file it reviews, or in the environment.
var (
	PasswordLogin = config.Define(config.Spec[bool]{
		Key: "auth.password.enabled", Env: "MARGINCE_AUTH_PASSWORD_ENABLED", Type: config.Bool, Default: config.Some(true), Roles: apiOnly,
		Doc: "Email and password sign-in; an installation behind its own identity provider turns it off.",
	})
)

// FileSettings lists the margince.yaml switches above.
func FileSettings() []config.Any {
	return []config.Any{PasswordLogin}
}
