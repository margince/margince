// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// The second gate on a write whose record is no longer ours to write.
//
// Every 🟡 tool that stages already refuses a target held in an external system
// of record, at STAGING (agents.refuseStagingElsewhere). That is the right
// place to say it first — it tells an agent immediately, rather than letting a
// human approve something that will fail — and it is not enough on its own.
//
// THE WINDOW IS THE INBOX. Handle is entered after the approval is redeemed and
// re-reads nothing through the provider, and activating the overlay flips a
// workspace MODE rather than deleting the native rows — so the store's own
// existence-and-row-scope probe still answers yes. A staged proposal that waits
// overnight spans exactly the kind of admin action that moves the system of
// record, and lands the next morning against a record this installation no
// longer owns.
//
// WHY HERE AND NOT IN THE TOOL. Re-reading every link inside each verb would be
// N provider round-trips on the approved path, and it would put the
// system-of-record question in the tool rather than beside the write. This seam
// is the one place REST, the MCP tools and the automation executors all reach
// these writes through, and the mode it asks is one cached read for the whole
// call.

// externalSoR answers whether this workspace's system of record has moved
// outside the installation. It is the Dispatcher's own cached mode read, passed
// as a function so this seam has ONE definition of the question rather than a
// second query beside it.
type externalSoR func(context.Context) (bool, error)

// refuseIfHeldElsewhere refuses a write whose records this installation no
// longer holds the authority for.
//
// FAIL CLOSED ON A MISSING SEAM, unlike the optional collaborators beside it in
// commsAdapter. A nil timer means "this surface cannot defer" and a nil
// calendar means "this deployment reads no diary" — both are honest absences a
// caller can act on. A nil answer to "may I still write this record" is not an
// absence; it is the guard not running, and a guard that passes when it cannot
// ask is the shape of the defect it was added for.
//
// The two draft-only adapters that carry no seam (the workflow executors and
// the reconciler) construct with an empty SendPath and reach no send at all, so
// this refusal is unreachable from them — but it is the refusal they would get,
// which is the answer that stays right if one of them ever grows a send.
func (c commsAdapter) refuseIfHeldElsewhere(ctx context.Context, what string) error {
	if c.externalSoR == nil {
		return fmt.Errorf(
			"this surface cannot tell whether this workspace's records are still held here, so it "+
				"will not %s against one: %w", what, apperrors.ErrUnsupportedBySoR)
	}
	external, err := c.externalSoR(ctx)
	if err != nil {
		return fmt.Errorf("compose: reading this workspace's system of record: %w", err)
	}
	if !external {
		return nil
	}
	return fmt.Errorf(
		"this workspace's records are held in an external system of record, so this installation "+
			"cannot %s against one — the change belongs in the system that owns the record: %w",
		what, apperrors.ErrUnsupportedBySoR)
}
