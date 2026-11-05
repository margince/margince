// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package captureorigin tells a contact capture made from mail or a calendar
// from one a human, an import or an agent made. Contacts and capture both ask
// it, and neither may import the other, so it sits where both can.
package captureorigin

import "github.com/margince/margince/backend/internal/platform/approvalsubject"

// The machine principals that mint a contact through the capture door after a
// sender has been judged, and the source the vCard import stamps.
const (
	VerdictActor = "agent:capture_counterparty_verdict"
	AcceptActor  = "agent:" + approvalsubject.KindCounterparty
	VCardSource  = "vcard_import"
)

// ContactSQL is the test that capture created the contact aliased p, read from
// captured_by, which is stamped once at create. The arguments are the
// placeholders holding VerdictActor, AcceptActor and VCardSource.
//
// Capture sinks run as `connector:` principals, the verdict and accept paths as
// their two actors. Two connector doors are not capture: the vCard ingest and an
// import, which names its system in source_system.
func ContactSQL(verdict, accept, vcard string) string {
	return `(p.captured_by LIKE 'connector:%' OR p.captured_by IN (` + verdict + `, ` + accept + `))
		   AND p.source IS DISTINCT FROM ` + vcard + `
		   AND p.source_system IS NULL`
}
