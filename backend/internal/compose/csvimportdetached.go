// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The budget an import commit carries once it has left its request.
//
// Its own file, the way platform/keyvault keeps CleanupTimeout in detached.go:
// a detach budget is a decision about what happens after the caller is gone,
// and it reads as one beside the rule it answers rather than among the page
// sizes and blob prefixes csvimport.go is otherwise made of.

import "time"

// importCommitTimeout bounds a detached commit or undo. The work scales with
// the upload, whose ceiling is CAP-BODY and set by the deployment, so this sits
// an order of magnitude above any cap rather than tracking one: too short turns
// a slow-but-succeeding import into the half-written run the detach exists to
// prevent.
const importCommitTimeout = 30 * time.Minute
