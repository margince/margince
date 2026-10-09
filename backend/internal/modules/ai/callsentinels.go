// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The sentinels of an attempt a model answered. The health read keys on them to
// tell a tier that responds from one that does not, so they are spelled once.
// Held by: TestTheAnsweredErrorsClassifyToExactlyTheAnsweredSentinels (backend/internal/modules/ai/callstore_test.go)
const (
	sentinelMeteringFailed  = "metering_failed"
	sentinelOutputWithheld  = "output_withheld"
	sentinelRequestRejected = "request_rejected"
)

// sentinelOutputRejected is a terminal attempt the task's validator refused
// after the whole retry ladder: a model answered, and the caller got nothing.
const sentinelOutputRejected = "output_rejected"

// sentinelTimeout is an attempt its deadline stopped: the model was asked and
// did not answer in time. A failure like provider_error, named apart so an
// admin can tell a slow host from a broken one and set the deadline by it.
const sentinelTimeout = "timeout"

// answeredSentinels are the sentinels the health read does not count as a
// failure. metering_failed is an answer whose usage write failed. The rest are
// outcomes: the model was reached and decided, or answered and was refused.
var answeredSentinels = []string{
	sentinelMeteringFailed, sentinelOutputWithheld, sentinelRequestRejected, sentinelOutputRejected,
}

// servedSentinels are the sentinels of an attempt whose answer still reached the
// caller. The task flow counts by these rather than answeredSentinels: a model
// that answered and was refused responded, but the caller was served nothing.
var servedSentinels = []string{sentinelMeteringFailed}
