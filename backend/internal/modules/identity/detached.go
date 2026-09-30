// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The budget this module's one detached goroutine carries, kept the way
// platform/keyvault keeps CleanupTimeout: beside the rule it answers rather
// than among the token TTLs it would otherwise be mistaken for.

import "time"

// resetSendTimeout bounds the detached send goroutine on this UNAUTHENTICATED
// endpoint, where a stuck relay would otherwise pin a goroutine and a
// connection per request that reached it. Sized for a lookup, a token mint and
// one SMTP round trip, long enough that a merely slow relay still delivers: a
// reset mail that never arrives is a locked-out user.
//
// Released inside the goroutine rather than by the handler that started it:
// the handler returns the moment the goroutine does, so a deferred release up
// there would cancel this budget before any of the work it bounds had run.
const resetSendTimeout = 2 * time.Minute
