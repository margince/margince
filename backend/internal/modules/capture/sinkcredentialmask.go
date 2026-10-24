// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// maskCapturedCredentials masks the credentials in the subject and body a
// record would store, whichever connector built it. It runs before any of it
// reaches a row or a trace. Raw keeps the original. The mail and calendar mappers already
// mask before their length cut, and masked text masks to itself.
func maskCapturedCredentials(rec connector.NormalizedRecord) connector.NormalizedRecord {
	fields, ok := rec.Fields.(ActivityFields)
	if !ok {
		return rec
	}
	fields.Subject = credentialmask.MaskText(fields.Subject)
	fields.Body = credentialmask.MaskText(fields.Body)
	rec.Fields = fields
	return rec
}
