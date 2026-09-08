// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Filing a captured message under a project (PROJ-FORM-1..3): a deterministic
// ladder, run AFTER the capture transaction committed, first match wins, no
// model call anywhere in it.
//
// Most mail belongs to no project and that is the correct answer. The ladder is
// therefore allowed to conclude nothing — it never guesses, and it never fails
// a capture. Concluding nothing is silent: no link, and no question raised for
// a human either. A message whose sender named no project named no project.
//
// The first two rungs read the timeline rows capture itself writes — the
// thread's siblings and the activity's own links — and the third touches no
// project SQL at all: which project a subject's tokens name is asked through
// the ProjectKeyMatcher seam, which compose implements from the module that
// owns the project table. A module never imports a sibling.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// ProjectKeyMatcher resolves the subject-line rung of the ladder: which LIVE
// project one of these tokens is the key of. It is a seam because `project`
// belongs to another module and capture must not import a sibling; compose
// injects the implementation.
//
// Ambiguity is the matcher's to report, not the caller's to reconstruct: two
// distinct projects named in one subject means the message says nothing
// reliable, so the matcher answers with no project rather than picking one.
//
// It takes the caller's TRANSACTION, the way StampProjectCorrespondence below
// does and for a sharper reason. An implementation opening its own connection
// does so while this ladder's transaction is still held, so each caller holds
// one connection and waits for another: with a small pool, or whenever
// concurrent captures occupy every connection, they deadlock until the context
// is cancelled — and a path documented as never failing the capture stalls the
// sync instead (#2107). The two rungs beside this one already read on the
// caller's tx; this is the one that did not.
type ProjectKeyMatcher interface {
	MatchProjectKey(ctx context.Context, tx pgx.Tx, tokens []string) (ids.UUID, error)
}

// StampProjectCorrespondence marks the activity just filed under a project as
// commercial correspondence, inside the transaction that wrote the link (D5).
// `activity` belongs to another module, so this is a seam for the same reason
// ProjectKeyMatcher is one.
type StampProjectCorrespondence func(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, projectID ids.UUID) error

// ProjectAttribution is the ladder's two halves, which are one seam because
// they are one obligation: deciding a message belongs to a project and
// classifying the correspondence that decision qualifies (D5).
//
// They are ONE struct rather than two parameters so a caller cannot supply the
// first without the second. The two failure modes are not comparable —
// attributing nothing is an ordinary configuration, attributing without
// classifying leaves a Handelsbrief the next erasure destroys — so the wiring
// that could get it wrong does not exist rather than being checked for.
type ProjectAttribution struct {
	Keys  ProjectKeyMatcher
	Stamp StampProjectCorrespondence
}

// WithProjectAttribution returns a copy that files captured activities under a
// project and classifies what it files. The zero value leaves capture exactly
// as it is without one: messages land, and none of them is attributed — which
// is also what every rung concluding nothing looks like, so no caller has to
// special-case the absence.
func (s *Sink) WithProjectAttribution(attribution ProjectAttribution) *Sink {
	c := *s
	c.projectKeys = attribution.Keys
	c.stampProject = attribution.Stamp
	return &c
}

// attributeProject runs the ladder for one freshly captured activity and writes
// at most one project link.
//
// Post-commit, in its own transaction, for the reason ensureCounterparty is:
// the timeline row must never be lost to an attribution fault, and the capture
// budget must not wait on this. A fault is recorded in system_log and never
// returned — the message is already on the timeline and stays there.
//
// Nothing re-runs the ladder over it afterwards. link_reconcile repairs the
// PERSON links a captured message is owed; a project attribution it never made
// is a filing decision nobody has re-asked, and saying otherwise here would
// describe a repair that does not exist.
func (s *Sink) attributeProject(ctx context.Context, rec connector.NormalizedRecord, ref datasource.EntityRef) {
	if s.projectKeys == nil || s.stampProject == nil {
		return
	}
	// A project is something an ACTIVITY is filed under. decideProject refuses
	// any other record anyway, but it refuses from inside the transaction this
	// function opens — so a lead capture paid for a transaction and an indexed
	// read to be told what its record type already says.
	if ref.Type != datasource.EntityActivity {
		return
	}
	activityID := ids.From[ids.ActivityKind](ref.ID)
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		filed, err := alreadyFiledUnderAProject(ctx, tx, activityID)
		if err != nil {
			return err
		}
		// A zero project is "nothing to propose": the activity is already
		// filed, so the ladder has nothing left to decide and
		// linkActivityToProject goes straight to the repair below.
		var projectID ids.UUID
		if !filed {
			if projectID, err = s.decideProject(ctx, tx, rec, activityID); err != nil || projectID.IsZero() {
				return err
			}
		}
		return linkActivityToProject(ctx, tx, activityID, projectID, s.stampProject)
	})
	if err != nil {
		s.logProjectAttributionFault(ctx, rec, err)
	}
}

// alreadyFiledUnderAProject is what makes the ladder safe to run on a REPLAY, and
// cheap to.
//
// The ladder used to run only when the capture had just created the activity. A
// transient fault — a SQL error, a matcher failure — logged for a reconcile that
// has no caller, and every later replay of that mailbox found the activity
// already present and skipped the ladder. The message stayed unfiled forever,
// and the breadcrumb was the whole record that the question went unanswered.
//
// So the ladder runs on every capture of the activity, not only the first.
// linkActivityToProject was already idempotent — ON CONFLICT DO NOTHING, and it
// stamps whether or not it inserted — so a second run was always SAFE; what it
// was not is free, and this is what makes it so.
//
// It answers yes or no, and its caller still goes through
// linkActivityToProject with the answer. Skipping straight past would skip the
// STAMP too, and that is a repair the conflict path exists to make: the
// migration's own backfill exists because pre-stamp project links do, and a
// missed stamp leaves a Handelsbrief an erasure destroys. What is skipped here
// is the three-rung ladder, which has nothing left to decide — not the write
// that makes the answer durable.
//
// Only a project link counts. An activity filed under a person or a deal has
// not been asked this question yet, and treating any link as an answer would
// make the retry it exists for unreachable for exactly the messages that carry
// other links — which is most of them.
func alreadyFiledUnderAProject(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) (bool, error) {
	var filed bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM activity_link
			 WHERE activity_id = $1 AND entity_type = 'project'
		)`, activityID).Scan(&filed)
	if err != nil {
		return false, fmt.Errorf("capture: reading whether the activity is already filed: %w", err)
	}
	return filed, nil
}

// logProjectAttributionFault records a failed attribution in system_log, on its
// own transaction — the one this fault came out of is already rolled back. The
// activity stands, filed under nothing. There is no partial state to repair,
// because a failed transaction wrote no link — and the next capture of that
// activity runs the ladder again, so this records a question that went
// unanswered THIS time rather than one nothing will ask again.
//
// It logs rather than returns for the reason every post-commit step does: the
// message is on the timeline and nothing here may take it off.
func (s *Sink) logProjectAttributionFault(ctx context.Context, rec connector.NormalizedRecord, cause error) {
	detail := map[string]any{
		fieldReason:       "project_attribution_failed",
		fieldSourceSystem: rec.NaturalKey.SourceSystem,
		fieldError:        cause.Error(),
	}
	// The natural key of a channel message embeds the customer's account id,
	// and this fault can land after an erasure committed — logEnsureFault
	// withholds it for that reason and so does this.
	if rec.Counterparty.ChannelIdentity.Provider == "" {
		detail[fieldSourceID] = rec.NaturalKey.SourceID
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, logErr := storekit.LogSystem(ctx, tx, "capture_project_attribution_fault", detail)
		return logErr
	})
	if err != nil {
		slog.ErrorContext(ctx, "capture: recording the project-attribution fault", "err", err, "cause", cause)
	}
}

// decideProject is the ladder itself: thread stickiness, then the deal the
// activity is filed under, then the subject's tokens. Zero means no rung
// matched, which is the ordinary answer for most mail.
//
// A rung that finds a project the caller cannot use — archived, or outside
// their object grant and row scope — counts as NO MATCH and the ladder carries
// on to the next rung. Stopping there instead would let an unusable T0 answer
// silently suppress a perfectly good T1 subject key, and the member would see
// a message filed under nothing with no way to tell why.
//
// Nothing here checks for an existing project link, and nothing needs to. This
// runs only for an activity the capture transaction just INSERTED, so there is
// no earlier link to find, and the link it writes is the only one there will
// ever be: uq_activity_link_project admits one per activity, and
// linkActivityToProject writes ON CONFLICT DO NOTHING. Replacing a filing is a
// human's relink alone.
//
// The ladder ends at these three, and its silence is the whole answer. Below
// them once sat a rung that PROPOSED a project — the sole live one on the
// account, or the nearest by embedding — and asked a human to confirm it. It is
// gone by founder decision: a project key in the subject is what files a
// message, and a system that guesses when no key was written asks a question
// nobody can answer better than the sender could have by typing `[ERP-27]`.
// Every rung here follows a link a human already made; none invents one.
func (s *Sink) decideProject(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, activityID ids.ActivityID) (ids.UUID, error) {
	fields, isActivity := rec.Fields.(ActivityFields)
	if !isActivity {
		return ids.Nil, nil
	}
	grant, err := readableProjectScope(ctx)
	if err != nil || grant.deniedOutright {
		// No project grant at all: this principal may not learn a project id,
		// let alone have one copied onto their timeline. Not a fault — a role
		// that never sees projects is a legitimate role.
		return ids.Nil, err
	}
	for _, rung := range []func() (ids.UUID, error){
		func() (ids.UUID, error) { return threadProject(ctx, tx, rec, fields, activityID) },
		func() (ids.UUID, error) { return dealProject(ctx, tx, activityID) },
		func() (ids.UUID, error) { return s.subjectProject(ctx, tx, fields) },
	} {
		projectID, err := rung()
		if err != nil || !projectID.IsZero() {
			return projectID, err
		}
	}
	return ids.Nil, nil
}

// subjectProject asks the key matcher about the subject's bracketed keys. A
// subject carrying none never reaches the seam: the bracket rule rules out
// almost every message, and asking anyway would be one query per captured
// message to learn what the tokenizer already knows.
func (s *Sink) subjectProject(ctx context.Context, tx pgx.Tx, fields ActivityFields) (ids.UUID, error) {
	tokens := projectKeyCandidates(fields.Subject)
	if len(tokens) == 0 {
		return ids.Nil, nil
	}
	return s.projectKeys.MatchProjectKey(ctx, tx, tokens)
}

// threadProject is the stickiness rung: a conversation is about one body of
// work, so a sibling message already filed under a project settles where this
// one belongs.
//
// Matched within ONE medium, and that is a security control rather than a
// convenience — the same one sinkreply.go's reply detection applies, for the
// same reason and with the same predicate. thread_key is a single flat
// namespace holding both a mail thread root and a channel's
// `<provider>:<bot>:<chat>` key, and the mail half is attacker-supplied: it is
// the message's own References root, chosen verbatim by the sender. Without the
// medium match, a forged References header naming a Telegram conversation — a
// bot id being public and a private chat's id being the user's own — files the
// forger's mail onto the project of a conversation they were never in.
//
// kind alone stopped discriminating at ADR-0107/A158, when every channel row
// took kind='message' whatever carried it, so the match is on the PAIR:
// IS NOT DISTINCT FROM lets one statement serve both, mail comparing NULL to
// NULL and a channel comparing provider to provider.
//
// Siblings CAN disagree, because a human may relink any one of them, so the
// most recent one wins: the latest filing is the freshest statement about where
// the conversation stands. id breaks a timestamp tie so two siblings stamped
// the same second cannot return either project run to run.
func threadProject(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields, activityID ids.ActivityID) (ids.UUID, error) {
	if rec.ThreadKey == "" {
		return ids.Nil, nil
	}
	args := []any{rec.ThreadKey, activityID, fields.Kind, fields.ChannelProvider}
	arg := func(v any) int { args = append(args, v); return len(args) }
	scoped, err := projectScopeClause(ctx, "p", arg)
	if err != nil {
		return ids.Nil, err
	}
	var projectID ids.UUID
	// A held or archived sibling settles nothing. A legal hold takes a message
	// out of every read that answers a question about the business, and this is
	// one of them: letting a restricted message decide where later mail is
	// filed would put its content back into circulation by proxy.
	err = tx.QueryRow(ctx, `
		SELECT p.id
		  FROM activity a
		  JOIN activity_link al ON al.activity_id = a.id AND al.entity_type = 'project'
		  JOIN project p ON p.id = al.project_id
		 WHERE a.thread_key = $1 AND a.id <> $2
		   AND a.kind = $3
		   AND a.channel_provider IS NOT DISTINCT FROM NULLIF($4, '')
		   AND a.restricted_at IS NULL AND a.archived_at IS NULL
		   AND p.archived_at IS NULL`+scoped+`
		 ORDER BY a.occurred_at DESC, a.id DESC
		 LIMIT 1`, args...).Scan(&projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, nil
	}
	if err != nil {
		return ids.Nil, fmt.Errorf("capture: reading the thread's project: %w", err)
	}
	return projectID, nil
}

// dealProject is the inheritance rung: a message filed under a deal that
// belongs to a project belongs to that project too. The deal's own rollup is
// the claim; this rung only follows it.
//
// An activity can carry several deal links, so this reads the DISTINCT projects
// they roll up to rather than picking one: two deals naming two different
// projects is ambiguity, and the ladder answers ambiguity the same way here as
// it does for a subject naming two keys — with no project at all. LIMIT 2 is
// what makes that a bounded question; the ORDER BY makes the one-match case
// reproducible rather than dependent on scan order.
func dealProject(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) (ids.UUID, error) {
	args := []any{activityID}
	arg := func(v any) int { args = append(args, v); return len(args) }
	scoped, err := projectScopeClause(ctx, "p", arg)
	if err != nil {
		return ids.Nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT p.id
		  FROM activity_link al
		  JOIN deal d ON d.id = al.deal_id
		  JOIN project p ON p.id = d.project_id
		 WHERE al.activity_id = $1 AND al.entity_type = 'deal'
		   AND d.archived_at IS NULL
		   AND p.archived_at IS NULL`+scoped+`
		 ORDER BY p.id
		 LIMIT 2`, args...)
	if err != nil {
		return ids.Nil, fmt.Errorf("capture: reading the deal's project: %w", err)
	}
	matched, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return ids.Nil, fmt.Errorf("capture: reading the deal's project: %w", err)
	}
	if len(matched) != 1 {
		return ids.Nil, nil
	}
	return matched[0], nil
}
