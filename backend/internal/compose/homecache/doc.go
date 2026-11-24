// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package homecache serves one reader's last answer to a Home read while
// nothing that could change who may see it has happened.
//
// Home's day reads assemble the reader's whole day on the server. Each request
// paid that again, however little had changed. An answer here is served while:
//   - it is younger than Life, which is how far behind Home may be for changes
//     made by colleagues and by background work. A day answer is served for
//     the rest of its day too (StaleGrace). It is then flagged stale and read
//     again, so a reader back from a break sees their day at once;
//   - the access fence has not moved since its own read began. Every write that
//     can take a record out of somebody's reach advances home_read_fence through
//     a trigger, so no writer can forget it. A write that only widens reach
//     leaves it, because capture widens reach all day;
//   - no record grant it was assembled under has expired, and none of its
//     reader's snoozes has ended or lifted, since neither writes anything.
//
// The walk a first page started lives four hours, less than a day answer. Once
// it ends, the answer is served paging without it and read again behind.
//
// An answer served in its last minute of Life, or past it, is read again behind
// it. So a reader who keeps Home open does not wait on its expiry. A refresh
// that fails is not tried again for a minute. The reader's own writes end their
// answers (AfterWrite), which covers writes that emit no event and no audit row.
// A POST that only adds marks the day behind the write. The next day read serves
// it flagged stale and reads it again behind; a receipt is read fresh.
// A write on a row, a worklist action or a 409 drops the answers. So a settled
// row is never listed as open, and a retry never meets the old version.
// The fence still ends an answer, stale or not, on a permission, team or reach change.
//
// UNLIKE worklistsnap, an answer holds display text. That is safe only because
// the fence moves before a revocation commits a reader out of reach. A copy
// that survived one would disclose what the revocation withdrew.
//
// Tables owned: docs/reference/entity-model/compose.md. It owns
// `home_read_cache`, `home_read_fence`, `home_read_fence_pending` and
// `home_read_forgotten`, and writes no audit row and no event: per-reader
// derived state, like worklist_snapshot.
package homecache
