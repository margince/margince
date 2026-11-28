// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import "github.com/margince/margince/backend/internal/platform/database/storekit"

// IdentifierSpellings are the identifier columns this module owns.
func IdentifierSpellings() []storekit.IdentifierColumn {
	return []storekit.IdentifierColumn{
		{Table: "workspace_email_domain", Column: "domain", Object: captureSettingsObject, HashedDomainKey: true},
		{Table: "capture_freemail_domain", Column: "domain", Object: captureSettingsObject, Parent: "id"},
		{Table: "capture_pending_counterparty", Column: "email", Object: "capture_pending_counterparty", Parent: "id"},
		{Table: "capture_sender_override", Column: fieldAddress, Object: captureSettingsObject, Parent: "id"},
	}
}
