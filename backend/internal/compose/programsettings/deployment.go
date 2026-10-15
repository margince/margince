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
	CapturePayloads = config.Define(config.Spec[bool]{
		Key: "ai.capture_payloads", Env: "MARGINCE_AI_CAPTURE_PAYLOADS", Type: config.Bool, Default: config.Some(false), Roles: both,
		Doc: "Keep each AI call's request and answer in ai_call_payload, aged by its retention policy; set it on the api and the worker together.",
	})
)

// FileSettings lists the margince.yaml switches above.
func FileSettings() []config.Any {
	return []config.Any{PasswordLogin, CapturePayloads}
}
