// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package assurance checks the inputs a forecast is built from, before anybody
// is asked to trust the number.
//
// A forecast is only as good as what it was computed over, and the failures are
// mundane: a close date that went by, an amount that disagrees with the offer
// that was sent, a deal nobody has heard from in ninety days. None of those is
// a bug in the arithmetic. Every one of them makes the total wrong.
//
// So a nightly pass asks the same questions a careful manager would, records
// what it found as EXCEPTIONS, and says how much of the pipeline it was
// actually able to check. That last part is the honest half: a run that could
// not read the mailbox reports which sources it reached, and readiness never
// says Ready with a required source missing. Refusing to run at all would
// produce no record in exactly the case the pass exists to report.
//
// An exception has a stable identity — a logical key over the workspace, the
// exception type and the record it is about, never over the value observed. The
// same finding on two nights is one exception seen twice, not two exceptions,
// which is what lets somebody resolve it once.
//
// A resolved exception is not reopened by re-detection. Somebody who answers
// "the value is correct" has said the condition will keep being true; reopening
// it tonight would ask them the same question every morning until they stopped
// reading.
//
// Tables owned: assurance_run, assurance_source_coverage, assurance_exception,
// assurance_resolution
package assurance
