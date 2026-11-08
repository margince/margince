// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

// retryAfterFault is a referenced refusal that knows when the same call can
// succeed, such as an agent allowance that resets at a known moment. Classify
// renders it as the Retry-After header.
type retryAfterFault interface {
	RetryAfterSeconds() int
}
