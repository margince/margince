// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

// MaxExactInteger is the largest integer a JSON number holds without loss, and
// so the largest amount a wire figure may carry.
const MaxExactInteger = 1<<53 - 1
