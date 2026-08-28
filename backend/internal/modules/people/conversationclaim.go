// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// What was promised, asked and decided in captured conversations (ADR-0097 D1).
//
// ONE store behind four cards. Commitments, open questions, decisions and the
// what-matters rows share a lifecycle — extracted → cited → correctable →
// dismissible — and differ only by kind, so three stores would be three copies
// of the same correction machinery.
//
// GROUNDED OR ABSENT. A claim carries the activity it was read from and the
// verbatim snippet, and this writer refuses one that carries neither: an
// ungrounded candidate is dropped rather than stored, because a claim nobody
// can check against what was actually written is the thing the whole mechanism
// exists to prevent.
//
// The extraction task that will call this is still to come (issue #849), and
// the demo seed does not call it either — both arrive with #849. Until then
// the HTTP endpoint below is the only door, and no product flow feeds it.
// That is why the attention feed's commitments lane is unbound
// (compose/attentionseam.go): a lane no production writer can fill would show
// every real customer an empty promise list dressed as a feature, and
// rebinding it is a one-line change when #849 ships. The endpoint stays: it
// is the correction surface extracted claims will need, and it is how a test
// seeds through the real writer rather than SQL.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ClaimInput is one claim to record.
type ClaimInput struct {
	PersonID   ids.PersonID
	Kind       string
	Body       string
	ActivityID ids.UUID
	Quote      string
	DueAt      *time.Time
	Source     string
}

// RecordConversationClaim writes one claim through the audited write shape.
//
// It gates the PERSON and the ACTIVITY separately, because the claim names
// both: citing a message the caller cannot open would disclose that the
// message exists, which is what the activity read protects.
func (s *Store) RecordConversationClaim(ctx context.Context, in ClaimInput) (crmcontracts.ConversationClaim, error) {
	// The MALFORMED-REQUEST checks run first, before any authority question.
	// A caller who omitted a field made a mistake the server can name; making
	// them fail an auth check instead reports their own typo as a permission
	// problem, and nothing about the missing field discloses a record.
	if in.Body == "" {
		return crmcontracts.ConversationClaim{}, httperr.Validation("body", "required",
			"a claim says something; an empty one is not a claim")
	}
	// Grounded or absent, and the two halves are refused separately so the
	// caller is told which one is missing. An omitted id decodes to the zero
	// UUID with no error, so without this probe it would reach the visibility
	// check, match nothing, and answer not-found for a message nobody named.
	if err := httperr.RequireBodyID("source_activity_id", in.ActivityID); err != nil {
		return crmcontracts.ConversationClaim{}, err
	}
	if in.Quote == "" {
		return crmcontracts.ConversationClaim{}, httperr.Validation("source_quote", "required",
			"a claim carries the words it was read from — an ungrounded claim is dropped, never stored")
	}
	// Human-only in the STORE as well as the router table. The route is
	// declared human-only today, but a store that relies on the routing
	// declaration alone is one x-mcp-tool away from being agent-reachable
	// without anybody revisiting this function.
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.ConversationClaim{}, err
	}
	if err := auth.Require(ctx, "person", principal.ActionUpdate); err != nil {
		return crmcontracts.ConversationClaim{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.ConversationClaim{}, err
	}

	var out crmcontracts.ConversationClaim
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritableLive(ctx, tx, "person", in.PersonID.UUID); err != nil {
			return err
		}
		// Activities are reachability-scoped rather than row-scoped, so they
		// have their own probe. Live, not merely visible: a claim must not
		// quote a message that has since been archived.
		if err := auth.EnsureActivityContentVisibleLive(ctx, tx, in.ActivityID); err != nil {
			return err
		}
		var id ids.UUID
		err := tx.QueryRow(ctx, `
			INSERT INTO conversation_claim
				(person_id, kind, body, source_activity_id, source_quote,
				 due_at, evidence_fingerprint, source, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id`,
			in.PersonID, in.Kind, in.Body,
			in.ActivityID, in.Quote, in.DueAt,
			claimFingerprint(in), in.Source, by).Scan(&id)
		if err != nil {
			return fmt.Errorf("write the conversation claim: %w", err)
		}
		auditID, err := storekit.Audit(ctx, tx, "create", "person", in.PersonID.UUID, nil,
			map[string]any{"claim_kind": in.Kind, "claim_id": id.String()})
		if err != nil {
			return fmt.Errorf("audit the conversation claim: %w", err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, in.PersonID.UUID,
			crmcontracts.PublicEventConversationClaimCaptured{
				ClaimId: openapi_types.UUID(id),
				Kind:    in.Kind,
			}); err != nil {
			return fmt.Errorf("emit conversation_claim.captured: %w", err)
		}
		out = crmcontracts.ConversationClaim{
			Id:               openapi_types.UUID(id),
			Kind:             crmcontracts.ConversationClaimKind(in.Kind),
			Body:             in.Body,
			SourceActivityId: openapi_types.UUID(in.ActivityID),
			SourceQuote:      in.Quote,
			Status:           crmcontracts.ConversationClaimStatusOpen,
			DueAt:            in.DueAt,
			NeedsReview:      false,
		}
		return nil
	})
	return out, err
}

// claimFingerprint pins the evidence a correction is made against. A later
// extraction run that reads the same words from the same message produces the
// same digest, which is what lets a human's correction hold against it.
func claimFingerprint(in ClaimInput) string {
	sum := sha256.Sum256([]byte(in.ActivityID.String() + "\x00" + in.Kind + "\x00" + in.Quote))
	return hex.EncodeToString(sum[:])
}

// claimSourceWithinProject keeps the claims whose source message is filed
// under ONE project, plus the ones whose source is filed under none: a claim
// is evidence from a conversation, and a conversation on another engagement
// is the wrong evidence for this one, while most correspondence on an account
// carries no project at all and dropping it would empty the card.
//
// activities.ActivityWithinProject and search's projectScope.clause are the
// same predicate. This is a deliberate copy, not an oversight: a module never
// imports a sibling (ADR-0054), and the rule is about subject matter rather
// than authority, so platform/auth is the wrong home for it. Change one,
// change all three.
func claimSourceWithinProject(projectPos int) string {
	return fmt.Sprintf(`(
			EXISTS (
			    SELECT 1 FROM activity_link scoped
			    WHERE scoped.activity_id = a.id AND scoped.project_id = $%[1]d)
			OR NOT EXISTS (
			    SELECT 1 FROM activity_link filed
			    WHERE filed.activity_id = a.id AND filed.project_id IS NOT NULL))`,
		projectPos)
}

// ClaimsForPerson reads this person's live claims, newest first, optionally
// narrowed to the claims whose source is filed under one project or under
// none (claimSourceWithinProject). The caller that narrows owes the project's
// own read gate first; this read filters, it does not authorize the filter.
//
// The activity join is what keeps a claim from outliving its evidence: a claim
// whose source the caller may not read is not returned, because the claim
// would otherwise quote a message the reader has no grant for.
func (s *Store) ClaimsForPerson(ctx context.Context, tx pgx.Tx, personID ids.PersonID, within *ids.ProjectID, limit int) ([]crmcontracts.ConversationClaim, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	personPos := arg(personID)
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = sqlAlwaysVisible
	}
	filed := sqlAlwaysVisible
	if within != nil {
		filed = claimSourceWithinProject(arg(*within))
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT c.id, c.kind, c.body, c.source_activity_id, c.source_quote,
		       a.subject, a.occurred_at, c.status, c.due_at, c.needs_review,
		       c.corrected_at, c.task_activity_id
		FROM conversation_claim c
		JOIN activity a ON a.id = c.source_activity_id AND a.archived_at IS NULL
		WHERE c.person_id = $%d AND c.archived_at IS NULL AND (%s) AND %s
		ORDER BY c.created_at DESC
		LIMIT %d`, personPos, scope, filed, limit), args...)
	if err != nil {
		return nil, fmt.Errorf("read the conversation claims: %w", err)
	}
	defer rows.Close()

	out := make([]crmcontracts.ConversationClaim, 0, limit)
	for rows.Next() {
		var claim crmcontracts.ConversationClaim
		var id, activityID ids.UUID
		var taskID *ids.UUID
		var subject *string
		var occurredAt time.Time
		if err := rows.Scan(&id, &claim.Kind, &claim.Body, &activityID, &claim.SourceQuote,
			&subject, &occurredAt, &claim.Status, &claim.DueAt, &claim.NeedsReview,
			&claim.CorrectedAt, &taskID); err != nil {
			return nil, fmt.Errorf("scan a conversation claim: %w", err)
		}
		claim.Id = openapi_types.UUID(id)
		claim.SourceActivityId = openapi_types.UUID(activityID)
		claim.SourceLabel = subject
		claim.OccurredAt = &occurredAt
		if taskID != nil {
			task := openapi_types.UUID(*taskID)
			claim.TaskActivityId = &task
		}
		out = append(out, claim)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the conversation claims: %w", err)
	}
	return out, nil
}

// RecordConversationClaim implements POST /people/{id}/claims.
func (h Handlers) RecordConversationClaim(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.RecordConversationClaimRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	claim, err := h.store.RecordConversationClaim(r.Context(), ClaimInput{
		PersonID:   ids.From[ids.PersonKind](ids.UUID(id)),
		Kind:       string(req.Kind),
		Body:       req.Body,
		ActivityID: ids.UUID(req.SourceActivityId),
		Quote:      req.SourceQuote,
		DueAt:      req.DueAt,
		Source:     "extraction",
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, claim)
}

// ProjectCommitment is one thing THEY said they would do, tied to the project
// the conversation was filed under.
type ProjectCommitment struct {
	ProjectID ids.ProjectID
	// Body is the claim verbatim. A caller quotes it; it is never a noun
	// phrase to build a sentence around — the extractor writes free text, and
	// "they owe us we'll revisit after legal" is what paraphrasing produces.
	Body       string
	Who        string
	DueAt      *time.Time
	ActivityID ids.UUID
}

// CommitmentsTheirsForProjects reads, for each of the named projects, the ONE
// open commitment they made to us that most wants a person: the overdue one,
// else the newest. One query for the whole set, because a record page asks
// this of every project it lists and a per-row read would grow with the
// account.
//
// The same two gates ClaimsForPerson keeps, for the same reasons. The activity
// join and auth.ActivityContentClause keep a claim from outliving or
// outreaching its evidence. The person row scope is new here and load-bearing:
// ClaimsForPerson is called with a person the caller has already been admitted
// to, while this read chooses the person itself, so an invisible person's
// claim must be skipped and the next candidate win rather than the row
// naming somebody this caller may not know of.
//
// status = 'open' AND NOT needs_review: a done or dismissed claim is settled,
// and needs_review means the extractor found contradicting evidence — the
// claim contract calls newest-wins no resolution, so presenting a disputed
// claim as "they owe us this" would state a contested thing as a fact.
//
// The bool is whether the caller could see EVERY person a candidate claim was
// made by. False means at least one was outside their scope and its claim was
// dropped, which a caller must report rather than let a project with no
// commitment read as a project with nothing outstanding.
func (s *Store) CommitmentsTheirsForProjects(ctx context.Context, tx pgx.Tx, projectIDs []ids.ProjectID, now time.Time) (map[ids.ProjectID]ProjectCommitment, bool, error) {
	out := map[ids.ProjectID]ProjectCommitment{}
	if len(projectIDs) == 0 {
		return out, true, nil
	}
	// The claim names a PERSON, and the row it returns carries their name and
	// what they said. Row scope alone does not admit the caller to people at
	// all — auth.ScopeClauseFor narrows a set the object grant has already
	// opened, and returns no predicate whatsoever for an unbounded actor. A
	// reader holding project and activity but not person would otherwise be
	// handed both.
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return nil, false, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	projectsPos := arg(projectIDs)
	nowPos := arg(now)
	activityScope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, false, err
	}
	if activityScope == "" {
		activityScope = sqlAlwaysVisible
	}
	personScope, err := auth.ScopeClauseFor(ctx, "person", "pr", arg)
	if err != nil {
		return nil, false, err
	}
	if personScope == "" {
		personScope = sqlAlwaysVisible
	}
	// Person scope is SELECTED, not filtered on. A WHERE clause would drop an
	// invisible person's claim and hand back a project that looks like it has
	// nothing outstanding — the silent answer this read exists to avoid. The
	// row still wins its project, and the caller is told it was withheld.
	//
	// DISTINCT ON needs its key first in ORDER BY, and the id breaks the tie
	// so two equally urgent commitments do not swap places between two reads
	// of the same page.
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT DISTINCT ON (l.project_id)
		       l.project_id, c.body, coalesce(pr.full_name, ''), c.due_at,
		       c.source_activity_id, (%[4]s) AS person_visible
		  FROM conversation_claim c
		  JOIN activity a ON a.id = c.source_activity_id AND a.archived_at IS NULL
		  JOIN activity_link l ON l.activity_id = a.id AND l.project_id = ANY($%[1]d)
		  JOIN person pr ON pr.id = c.person_id AND pr.archived_at IS NULL
		 WHERE c.kind = 'commitment_theirs' AND c.status = 'open' AND NOT c.needs_review
		   AND c.archived_at IS NULL
		   AND (%[3]s)
		 ORDER BY l.project_id,
		          (c.due_at IS NOT NULL AND c.due_at < $%[2]d) DESC,
		          c.created_at DESC, c.id`,
		projectsPos, nowPos, activityScope, personScope), args...)
	if err != nil {
		return nil, false, fmt.Errorf("read the account's open commitments: %w", err)
	}
	defer rows.Close()

	complete := true
	for rows.Next() {
		var projectID ids.ProjectID
		var commitment ProjectCommitment
		var personVisible bool
		if err := rows.Scan(&projectID, &commitment.Body, &commitment.Who,
			&commitment.DueAt, &commitment.ActivityID, &personVisible); err != nil {
			return nil, false, fmt.Errorf("scan an open commitment: %w", err)
		}
		if !personVisible {
			complete = false
			continue
		}
		commitment.ProjectID = projectID
		out[projectID] = commitment
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("read the account's open commitments: %w", err)
	}
	return out, complete, nil
}
