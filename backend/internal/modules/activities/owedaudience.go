// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/threadverdict"
)

// routinePrivateReasons are the holds that narrow mail by setting rather than
// by anybody's judgement of it. Every other reason protects the mail, and an
// unknown one counts as protecting it.
var routinePrivateReasons = []string{
	ReasonPosture, ReasonWorkspaceFloor, ReasonPendingVerdict,
	ReasonSharedMailbox, ReasonNoRecord, ReasonNoCounterparty,
}

// judgeableAudienceSQL is whether text of activity alias may go to a model.
// Workspace mail may. With includePrivate, mail private for a routine reason
// may too, once its thread's verdict has landed. A thread hold or a
// restriction refuses either. Every reader and writer that sends mail to a
// model asks this one predicate, so a row one offers is a row another accepts.
func judgeableAudienceSQL(alias string, includePrivate bool) string {
	audience := auth.WorkspaceAudienceSQL(alias)
	if includePrivate {
		audience = `(` + audience + ` OR ` + routinePrivateSQL(alias) + `)`
	}
	return `(` + audience + ` AND ` + alias + `.restricted_at IS NULL)`
}

// routinePrivateSQL is whether activity alias is private only by a routine
// setting and its thread's verdict has landed without a hold. A missing reason
// reads as false, never null, so a negated test still refuses the row.
func routinePrivateSQL(alias string) string {
	return `(` + alias + `.audience = '` + audienceParticipants + `'
   AND coalesce(` + alias + `.audience_reason, '') IN ('` + strings.Join(routinePrivateReasons, "', '") + `')
   AND ` + threadverdict.SettledUnheldSQL(alias) + `)`
}

// withheldColumns renders the waiting queue's model-free fallback columns for
// message a: judge_withheld, and thread_writers, the seats whose sent mail on
// a's conversation came before a. Only mail the reader may read names a
// writer, and only on a row no model will judge.
func withheldColumns(ctx context.Context, tx pgx.Tx, arg func(any) int) (string, error) {
	private, err := privateMailRequests(ctx, tx)
	if err != nil {
		return "", err
	}
	readable, err := auth.ActivityMessageContentClause(ctx, "writer_sent", arg)
	if err != nil {
		return "", err
	}
	withheld := `(a.owed_verdict IS NULL AND NOT ` + judgeableAudienceSQL("a", private) + `)`
	return withheld + ` AS judge_withheld,
	       CASE WHEN ` + withheld + ` THEN ARRAY(SELECT DISTINCT writer_from.user_id
	         FROM activity writer_sent
	         JOIN activity_participant writer_from ON writer_from.activity_id = writer_sent.id
	          AND writer_from.role = 'from' AND writer_from.user_id IS NOT NULL
	          AND ` + auth.ParticipantStandsSQL("writer_from", "writer_sent") + `
	        WHERE ` + ourOutboundInThisThread("writer_sent", "a") + `
	          AND (writer_sent.occurred_at, writer_sent.id) < (a.occurred_at, a.id)
	          AND writer_sent.restricted_at IS NULL AND (` + readable + `))
	       ELSE '{}'::uuid[] END AS thread_writers`, nil
}

// noWithheldColumns is withheldColumns for a read that draws only ids.
const noWithheldColumns = `false AS judge_withheld, '{}'::uuid[] AS thread_writers`

// JudgeableAudienceTx is judgeableAudienceSQL under this installation's
// setting, for a reader outside the module that sends alias's text to a model.
func JudgeableAudienceTx(ctx context.Context, tx pgx.Tx, alias string) (string, error) {
	private, err := privateMailRequests(ctx, tx)
	if err != nil {
		return "", err
	}
	return judgeableAudienceSQL(alias, private), nil
}

// JudgeableAudience is judgeableAudienceSQL for a reader outside the module
// that composes it into fixed SQL, with the setting read by
// ModelReadsPrivateMailTx in the same transaction.
func JudgeableAudience(alias string, includePrivate bool) string {
	return judgeableAudienceSQL(alias, includePrivate)
}

// ModelReadsPrivateMailTx reads whether mail private for a routine reason may
// go to a model in this installation.
func ModelReadsPrivateMailTx(ctx context.Context, tx pgx.Tx) (bool, error) {
	return privateMailRequests(ctx, tx)
}
