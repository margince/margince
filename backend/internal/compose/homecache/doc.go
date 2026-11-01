// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package homecache serves one reader's last answer to a Home read while
// nothing that could change who may see it has happened.
//
// Home's day reads assemble the reader's whole day on the server. Each request
// paid that again, however little had changed. An answer here is served while:
//   - it is younger than Life, which is how far behind Home may be for changes
//     made by other people and by background work;
//   - the access fence has not moved since its own read began. Every write that
//     can take a record out of somebody's reach advances home_read_fence through
//     a trigger, so no writer can forget it (migration 1793559778);
//   - no record grant it was assembled under has expired, since expiry writes
//     nothing;
//   - the walk its first page started can still be resumed;
//   - for the receipt, no machine has written an audit row since.
//
// The reader's own writes drop their answers (Forget), so what they do shows at
// once. That covers the writes that emit no event and no audit row.
//
// UNLIKE worklistsnap, an answer holds display text. That is safe only because
// the fence moves before a revocation commits a reader out of reach. A copy
// that survived one would disclose what the revocation withdrew.
//
// Tables owned: docs/reference/entity-model/compose.md. It owns
// `home_read_cache`, `home_read_fence` and `home_read_forgotten`, and writes no
// audit row and no event: per-reader derived state, like worklist_snapshot.
package homecache
