// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "fmt"

// The parts of a deal's file membership that the deal's Files area and its deal
// room both apply. Each module adds its own conditions around them; these are
// the ones the two must not answer differently.

// InlineMailImageCeiling is the size under which an image part of a captured
// message is taken for a signature logo or an icon. Capture does not mark inline
// parts, so size is the proxy: a contract scan is never this small.
const InlineMailImageCeiling = 65536

// ListedMailFileSQL is true when file, an attachment alias, belongs on a
// Documents list. An inline image or a calendar payload stays on its message.
func ListedMailFileSQL(file string) string {
	return notInlineMailImageSQL(file) + " AND " + notCalendarPayloadSQL(file)
}

// notInlineMailImageSQL is true unless file is an inline image of a captured
// message. A part with no content type is not one; a missing size counts as 0.
func notInlineMailImageSQL(file string) string {
	return fmt.Sprintf(`NOT (COALESCE(%[1]s.content_type, '') LIKE 'image/%%' AND COALESCE(%[1]s.byte_size, 0) < %[2]d)`,
		file, InlineMailImageCeiling)
}

// notCalendarPayloadSQL is true unless file is the calendar part of an
// invitation mail. Its bytes sniff as text/plain, so text/calendar survives
// only in declared_type; a named part is caught by its .ics name.
func notCalendarPayloadSQL(file string) string {
	return fmt.Sprintf(`NOT (lower(COALESCE(%[1]s.declared_type, '')) LIKE 'text/calendar%%'
		OR lower(COALESCE(%[1]s.content_type, '')) LIKE 'text/calendar%%'
		OR lower(%[1]s.filename) LIKE '%%.ics')`, file)
}

// MessageOnDealSQL is true when the message whose id is message is linked to
// deal. Both are SQL expressions from compile-time literals.
func MessageOnDealSQL(message, deal string) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM activity_link deal_link
		 WHERE deal_link.activity_id = %s AND deal_link.entity_type = 'deal' AND deal_link.deal_id = %s)`, message, deal)
}
