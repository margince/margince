// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build loadrig

package gmail

// PointGoogleCallsAt sends every Google call to the server at base, so several
// large history imports run locally against backend/tools/fakegmail. Only a
// binary built with the loadrig tag has it.
func PointGoogleCallsAt(base string) {
	googleEndpoints.auth = base + "/o/oauth2/v2/auth"
	googleEndpoints.token = base + "/token"
	googleEndpoints.api = base + "/gmail/v1/users/me"
}
