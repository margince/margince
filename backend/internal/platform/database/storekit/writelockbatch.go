// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "github.com/jackc/pgx/v5"

// QueueWriteIdentityLock adds LockWriteIdentity's lock to a batch, so a caller
// can take it and read what it protects in one exchange. Each statement of a
// batch takes its own snapshot, so the read after the lock sees what a writer
// committed before releasing it.
//
// The statement is spelled again here, because editing LockWriteIdentity moves
// the stamp of every AI certification whose check reaches it.
// TestQueuedWriteIdentityLockIsTheSameLock holds the two to one key.
func QueueWriteIdentityLock(batch *pgx.Batch, entityType, identity string) {
	batch.Queue(`SELECT pg_advisory_xact_lock(hashtextextended(
		$1 || ':' || $2, 0))`, entityType+"_write", identity)
}
