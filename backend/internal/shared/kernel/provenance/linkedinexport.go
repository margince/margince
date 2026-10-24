// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

// LinkedInExportSource is the field-level source of a value a member took from
// their own LinkedIn export when they accepted a connection match. It covers
// the contact's title, and the employment edge and its role. The human who
// ticked the box is the captured_by, so the source says only where it came from.
const LinkedInExportSource = "linkedin_export"
