// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Filing a captured message under a project (PROJ-FORM-1..3): a deterministic
// ladder, first match wins, no model call anywhere in it. It runs inside the
// capture transaction of a new activity (fileUnderProjectInCaptureTx), and
// after the commit when that did not finish it.
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
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
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

// ProjectAttribution is the ladder's parts, which are one seam because they are
// one obligation. It decides a message belongs to a project, writes that link
// and classifies the correspondence the decision qualifies (D5).
//
// They are one struct so a caller cannot supply the first without the others.
// Attributing nothing is an ordinary configuration. Attributing without
// classifying leaves a Handelsbrief the next erasure destroys, so that wiring
// cannot be built.
type ProjectAttribution struct {
	Keys  ProjectKeyMatcher
	Stamp StampProjectCorrespondence
	// File is the activities module's automatic project filing, the write the
	// deal and lead filings use too. Compose injects it.
	File FileOnProject
}

// FileOnProject writes the project link under the activity's lock only while it
// has none. It answers whether the link landed.
type FileOnProject func(ctx context.Context, tx pgx.Tx, id ids.ActivityID, project ids.UUID, evidence map[string]any) (bool, error)

func (a ProjectAttribution) complete() bool {
	return a.Keys != nil && a.Stamp != nil && a.File != nil
}

// WithProjectAttribution returns a copy that files captured activities under a
// project and classifies what it files. The zero value leaves capture exactly
// as it is without one: messages land, and none of them is attributed — which
// is also what every rung concluding nothing looks like, so no caller has to
// special-case the absence.
func (s *Sink) WithProjectAttribution(attribution ProjectAttribution) *Sink {
	c := *s
	c.projectAttribution = attribution
	return &c
}

// attributeProject runs the ladder after the commit, in its own transaction,
// for a capture whose transaction did not run it. A fault is recorded in
// system_log and never returned: the message stays on the timeline.
//
// Every replay of the activity runs it again, and that is the retry.
// alreadyFiledUnderAProject makes the re-run cheap, and linkActivityToProject
// makes it safe.
func (s *Sink) attributeProject(ctx context.Context, rec connector.NormalizedRecord, ref datasource.EntityRef) {
	if !s.attributesProject(ref) {
		return
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return s.fileUnderProjectTx(ctx, tx, rec, ref)
	})
	if err != nil {
		s.logProjectAttributionFault(ctx, rec, err)
	}
}

// ladderInCapture is how the ladder went inside the capture transaction.
// ran false leaves it to the run after the commit.
type ladderInCapture struct {
	ran   bool
	fault error
}

// fileUnderProjectInCaptureTx runs the ladder for a new activity inside the
// capture transaction. Mail from a deferred or suppressed sender is filed under
// nobody by the sender ensure, so its project link is its first. Written here,
// no other transaction sees the mail unlinked, and the link does not move the
// Home fence (home_read_fence_first_link).
//
// A savepoint holds the ladder, so a fault costs the filing and not the mail.
// A lock cycle is returned, so the whole capture runs again.
func (s *Sink) fileUnderProjectInCaptureTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, landed activityLanding) (ladderInCapture, error) {
	if !landed.created || landed.internal {
		return ladderInCapture{}, nil
	}
	if !s.attributesProject(landed.ref) {
		return ladderInCapture{ran: true}, nil
	}
	_, err := storekit.Savepoint(ctx, tx, func(sp pgx.Tx) (bool, error) {
		return true, s.fileUnderProjectTx(ctx, sp, rec, landed.ref)
	})
	if err != nil && storekit.IsLockCycle(err) {
		return ladderInCapture{}, err
	}
	return ladderInCapture{ran: true, fault: err}, nil
}

// settleProject finishes the ladder once the capture committed. A fault inside
// the capture is recorded, and the next replay retries it, as after any fault.
func (s *Sink) settleProject(ctx context.Context, rec connector.NormalizedRecord, ref datasource.EntityRef, ladder ladderInCapture) {
	switch {
	case !ladder.ran:
		s.attributeProject(ctx, rec, ref)
	case ladder.fault != nil:
		s.logProjectAttributionFault(ctx, rec, ladder.fault)
	}
}

// attributesProject says whether the ladder applies. A project is something an
// ACTIVITY is filed under, so any other record skips the transaction.
func (s *Sink) attributesProject(ref datasource.EntityRef) bool {
	return s.projectAttribution.complete() && ref.Type == datasource.EntityActivity
}

// fileUnderProjectTx is the ladder and its one link, on the caller's
// transaction. Both the capture transaction and the run after it call it.
func (s *Sink) fileUnderProjectTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, ref datasource.EntityRef) error {
	if !s.attributesProject(ref) {
		return nil
	}
	activityID := ids.From[ids.ActivityKind](ref.ID)
	filed, err := alreadyFiledUnderAProject(ctx, tx, activityID)
	if err != nil {
		return err
	}
	// A zero project is "nothing to propose": the activity is already filed, so
	// linkActivityToProject goes straight to the stamp repair.
	var projectID ids.UUID
	var rung string
	if !filed {
		if projectID, rung, err = s.decideProject(ctx, tx, rec, activityID); err != nil || projectID.IsZero() {
			return err
		}
	}
	return linkActivityToProject(ctx, tx, activityID, projectFiling{project: projectID, rung: rung}, s.projectAttribution)
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
// Only a project link counts. An activity filed under a contact or a deal has
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
func (s *Sink) decideProject(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, activityID ids.ActivityID) (ids.UUID, string, error) {
	fields, isActivity := rec.Fields.(ActivityFields)
	if !isActivity {
		return ids.Nil, "", nil
	}
	grant, err := readableProjectScope(ctx)
	if err != nil || grant.deniedOutright {
		// No project grant at all: this principal may not learn a project id,
		// let alone have one copied onto their timeline. Not a fault — a role
		// that never sees projects is a legitimate role.
		return ids.Nil, "", err
	}
	for _, rung := range []struct {
		name   string
		decide func() (ids.UUID, error)
	}{
		{provenance.ProjectRungThread, func() (ids.UUID, error) { return threadProject(ctx, tx, rec, fields, activityID) }},
		{provenance.ProjectRungDeal, func() (ids.UUID, error) { return dealProject(ctx, tx, activityID) }},
		{provenance.ProjectRungSubject, func() (ids.UUID, error) { return s.subjectProject(ctx, tx, fields) }},
	} {
		projectID, err := rung.decide()
		if err != nil || !projectID.IsZero() {
			return projectID, rung.name, err
		}
	}
	return ids.Nil, "", nil
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
	return s.projectAttribution.Keys.MatchProjectKey(ctx, tx, tokens)
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
// belongs to a project belongs to that project too. storekit.DealsProject
// decides it, as it does for a note or a send logged on the deal. Two deals
// naming two projects file the message under neither.
func dealProject(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) (ids.UUID, error) {
	return storekit.ActivityDealsProject(ctx, tx, activityID)
}
