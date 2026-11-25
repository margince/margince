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

// ContactSQL is the test that capture created the contact aliased p. The
// arguments hold VerdictActor, AcceptActor and the import channel. The vCard
// ingest and an import also run as connectors, and their channel is import.
// The database maps a source the correction task has not reached yet.
func ContactSQL(verdict, accept, imported string) string {
	return `(p.captured_by LIKE 'connector:%' OR p.captured_by IN (` + verdict + `, ` + accept + `))
		   AND party_source_channel(p.source, p.source_system) <> ` + imported + `
		   AND ` + NoOtherSystemSQL("p.source_system")
}

// NoOtherSystemSQL tests that column names no system but the connector or
// writer the source mapping fills. Each such name maps to itself, and an
// import's or an integration's name maps to NULL. Never NULL itself, so a
// caller may scan it. column is a compile-time expression.
func NoOtherSystemSQL(column string) string {
	return "(party_source_producer(" + column + ") IS NOT DISTINCT FROM " + column + ")"
}
