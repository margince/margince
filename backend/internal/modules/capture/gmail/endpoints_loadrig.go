// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build loadrig

package gmail

import "os"

// A load-test build sends every Google call to the fake Gmail server named
// here, so several large history imports run locally against demo mail.
func init() {
	base := os.Getenv("MARGINCE_LOADRIG_GOOGLE_BASE")
	if base == "" {
		return
	}
	googleEndpoints.auth = base + "/o/oauth2/v2/auth"
	googleEndpoints.token = base + "/token"
	googleEndpoints.api = base + "/gmail/v1/users/me"
}
