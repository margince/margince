// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/threadverdict"
)

// privateReason is what one reason a message is private for allows.
type privateReason struct {
	// carried: the hold lives on the row, and no import row records it.
	carried bool
	// read: a model may read the mail for its own reader, while
	// PrivateMailRequests is on.
	read bool
	// share: what the mail says may reach the whole workspace, while
	// SharePrivateQuoteRequests is on too.
	share bool
}

// privateReasons is the one classification of why a message is private. A
// reason missing here allows nothing. Only a mailbox's own setting, or a
// verdict still to land, is shared. The workspace floor, a held shared mailbox
// and mail filed under no record say colleagues do not read it. The owner's
// model may read such mail, and nothing of it is shared.
var privateReasons = map[string]privateReason{
	ReasonPosture:             {read: true, share: true},
	ReasonPendingVerdict:      {read: true, share: true},
	ReasonNoCounterparty:      {carried: true, read: true, share: true},
	ReasonSharedMailbox:       {read: true},
	ReasonWorkspaceFloor:      {carried: true, read: true},
	ReasonNoRecord:            {carried: true, read: true},
	ReasonCounterparty:        {carried: true},
	ReasonConfidentialMarker:  {carried: true},
	ReasonMembersOnly:         {carried: true},
	ReasonInternalOperational: {carried: true},
}

// reasonsWhere lists, sorted, the reasons whose rule passes keep.
func reasonsWhere(keep func(privateReason) bool) []string {
	var out []string
	for _, reason := range slices.Sorted(maps.Keys(privateReasons)) {
		if keep(privateReasons[reason]) {
			out = append(out, reason)
		}
	}
	return out
}

// privateForSQL is whether activity alias is private only for one of reasons
// and its thread's verdict has landed without a hold. A missing reason reads
// as false, never null, so a negated test still refuses the row.
func privateForSQL(alias string, reasons []string) string {
	return `(` + alias + `.audience = '` + audienceParticipants + `'
   AND coalesce(` + alias + `.audience_reason, '') IN ('` + strings.Join(reasons, "', '") + `')
   AND ` + threadverdict.SettledUnheldSQL(alias) + `)`
}

// ShareableAudience is whether what activity alias says may reach the whole
// workspace. It is workspace mail, or with on, mail private only by a mailbox
// setting on a thread no seat holds. A restriction refuses either.
func ShareableAudience(alias string, on bool) string {
	audience := auth.WorkspaceAudienceSQL(alias)
	if on {
		audience = `(` + audience + ` OR (` + privateForSQL(alias, reasonsWhere(func(r privateReason) bool { return r.share })) +
			` AND NOT ` + threadverdict.AnySeatHoldsSQL(alias) + `))`
	}
	return `(` + audience + ` AND ` + alias + `.restricted_at IS NULL)`
}

// SharesPrivateRequestsTx reads whether a request in routine-private mail may
// be read by a model and shared with the workspace: both switches on.
func SharesPrivateRequestsTx(ctx context.Context, tx pgx.Tx) (bool, error) {
	read, err := privateMailRequests(ctx, tx)
	if err != nil || !read {
		return false, err
	}
	return sharePrivateQuoteRequests(ctx, tx)
}

// ShareableAudienceTx is ShareableAudience under this installation's switches.
func ShareableAudienceTx(ctx context.Context, tx pgx.Tx, alias string) (string, error) {
	on, err := SharesPrivateRequestsTx(ctx, tx)
	if err != nil {
		return "", err
	}
	return ShareableAudience(alias, on), nil
}
