// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// rawCaptureLink correlates one raw_capture row, aliased `rc`, to the activity
// `a` read from it. The export, the retention stage and the shield all use it.
//
// The link column comes first. The channel poller keys raw_capture by the
// provider's redelivery id, so a natural-key join finds no activity there.
//
// Held by: TestThePrivacyRawCaptureCorrelationIsSpelledOnce
// (backend/gates/rawcapturelink_test.go).
const rawCaptureLink = `(a.raw_capture_id = rc.id
		         OR (a.raw_capture_id IS NULL
		             AND a.source_system = rc.source_system
		             AND a.source_id = rc.source_id))`
