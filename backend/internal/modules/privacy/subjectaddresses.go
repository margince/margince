// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import "github.com/margince/margince/backend/internal/platform/database/storekit"

func lowercased(values []string) []string {
	return storekit.EmailLookups(values...)
}

func recordedRequestAddressScope(emails string) string {
	return "recorded_for_addresses && " + emails + "::text[]"
}
