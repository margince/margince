// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package person360

// The sections read from the timeline and its neighbours: recent activity,
// open tasks, the two last-touch directions, who knows this contact, the
// consent guard, the enrichment evidence, and the visit delta.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/network"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// personReachesActivity is the reachability predicate every person-360 section
// uses: an activity this contact is ON.
//
// Two arms, because a person is on a message in two different ways. The LINK is
// how the message is filed — what the entity-scoped activity list walks, so the
// 360's recent rows and the full timeline agree about it. The PARTICIPANT row is
// who was actually in the conversation, and it is the only record of a contact
// who was CC'd or who attended a meeting: capture files a message under its
// counterparty, so a thread a contact was copied on is filed under somebody else
// and their own page never showed it.
//
// No deal arm, deliberately, although the ORG walk has one. A company's reach is
// the company's business and a deal belongs to it; a person's page answers a
// narrower question — what did I have with THIS human — and pulling in every
// message on their employer's deals would put colleagues' threads they were
// never on onto their record.
//
// The single %d bind is used by both arms, so every call site passes the person
// position exactly once.
const personReachesActivity = `(EXISTS (
	SELECT 1 FROM activity_link l
	WHERE l.activity_id = a.id AND l.person_id = $%[1]d)
 OR EXISTS (
	SELECT 1 FROM activity_participant ap
	WHERE ap.activity_id = a.id AND ap.person_id = $%[1]d))`

// activityScope renders the caller's activity CONTENT gate for the timeline
// rows this section hands back, defaulting to the permissive clause when the
// scope adds no predicate of its own.
func activityScope(ctx context.Context, arg func(any) int) (string, error) {
	return activityScopeUnder(ctx, arg, auth.ActivityContentClause)
}

// activityDiscoverScope is the DISCOVER gate for the sections that disclose a
// date and a direction and nothing else (last touch): a limited conversation
// still counts as a touch, its content stays withheld.
func activityDiscoverScope(ctx context.Context, arg func(any) int) (string, error) {
	return activityScopeUnder(ctx, arg, auth.ActivityDiscoverClause)
}

func activityScopeUnder(ctx context.Context, arg func(any) int,
	gate func(context.Context, string, func(any) int) (string, error)) (string, error) {
	clause, err := gate(ctx, "a", arg)
	if err != nil {
		return "", err
	}
	if clause == "" {
		return "true", nil
	}
	return clause, nil
}

// projectScope renders the body-of-work narrowing as one more WHERE term, or
// nothing when the page is unscoped. Every timeline section of this page goes
// through it, so the recent rows, the open tasks, the last-touch dates and the
// since-last-visit count cannot disagree about which project they describe.
func projectScope(opts AssembleOptions, arg func(any) int) string {
	if opts.ProjectID == nil {
		return ""
	}
	return " AND " + activities.ActivityWithinProject(arg(*opts.ProjectID))
}

// activitiesSection is the recent timeline — a summary, not a paging
// surface: page two comes from GET /activities with its own cursor.
func (s *Service) activitiesSection(ctx context.Context, tx pgx.Tx, personID ids.PersonID, opts AssembleOptions, out *crmcontracts.Person360) error {
	if err := requireRead(ctx, "activity"); err != nil {
		return err
	}
	rows, hasMore, err := s.readActivities(ctx, tx, personID, opts, "")
	if err != nil {
		return err
	}
	out.Activities = &struct {
		Data []crmcontracts.Activity `json:"data"`
		Page crmcontracts.PageInfo   `json:"page"`
	}{Data: rows, Page: sectionPage(rows, hasMore)}
	return nil
}

// nextStepsSection is the open work filed against this person: tasks not
// yet done. A task with no due date still counts — it is owed either way.
func (s *Service) nextStepsSection(ctx context.Context, tx pgx.Tx, personID ids.PersonID, opts AssembleOptions, out *crmcontracts.Person360) error {
	if err := requireRead(ctx, "activity"); err != nil {
		return err
	}
	rows, hasMore, err := s.readActivities(ctx, tx, personID, opts,
		`AND a.kind = 'task' AND coalesce(a.is_done, false) = false`)
	if err != nil {
		return err
	}
	out.NextSteps = &struct {
		Data []crmcontracts.Activity `json:"data"`
		Page crmcontracts.PageInfo   `json:"page"`
	}{Data: rows, Page: sectionPage(rows, hasMore)}
	return nil
}

// sectionPage is the section's edge in the activities list's own cursor
// vocabulary: the same (occurred_at, id) keyset GET /activities orders by, so
// the record page continues from this page's last row rather than fetching
// page one again and showing every row twice.
func sectionPage(rows []crmcontracts.Activity, hasMore bool) crmcontracts.PageInfo {
	info := crmcontracts.PageInfo{HasMore: hasMore}
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		cursor := storekit.EncodeCursor(last.OccurredAt, ids.UUID(last.Id))
		info.NextCursor = &cursor
	}
	return info
}

// readActivities is the shared body of the timeline and next-step reads.
//
// It selects channel_provider, and that is not decoration: since ADR-0107/A158
// the kind says only that an interaction was a message, so a row without the
// provider renders as the bare word "message" and a Telegram thread becomes
// indistinguishable from a unit's. It also selects version, for the same
// reason and by the same mistake a second time: AudienceAction sends the
// row's version as If-Match and refuses to write blind without one, so a row
// missing it cannot have its audience narrowed from this page at all — the
// request never leaves the browser, and the error names no cause because
// there was no request to have one (margince#3249). This SELECT is a
// hand-written sibling of activities.activityColumns, which is exactly how
// it came to be missing a column for a whole slice, twice.
// TestThePerson360TimelineNamesTheTransportThatCarriedAMessage and
// TestThePerson360TimelineCarriesTheVersionAWriteNeeds are the guards that
// say so out loud.
func (s *Service) readActivities(ctx context.Context, tx pgx.Tx, personID ids.PersonID, opts AssembleOptions, extra string) ([]crmcontracts.Activity, bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	personPos := arg(personID)
	// DISCOVER-gated, like the activities list: a limited conversation on
	// this contact is still a row on the timeline — date, direction, kind —
	// with its content withheld (content_state), not a gap the reader
	// cannot tell from silence.
	scope, err := activityDiscoverScope(ctx, arg)
	if err != nil {
		return nil, false, err
	}
	contentArm, err := auth.ActivityAudienceArm(ctx, "a", arg)
	if err != nil {
		return nil, false, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT a.id, a.kind, a.channel_provider, a.subject, a.body, a.direction,
		       a.occurred_at, a.due_at, a.is_done, a.source, a.captured_by, a.created_at,
		       a.thread_key, a.bulk_mail_attested, a.audience, a.audience_reason,
		       a.version, (%s) AS content_available,
		       EXISTS (SELECT 1 FROM activity_link fl
		                WHERE fl.activity_id = a.id AND fl.person_id = $%d) AS filed_here
		FROM activity a
		WHERE a.archived_at IS NULL AND %s AND (%s)%s %s
		ORDER BY a.occurred_at DESC, a.id DESC
		LIMIT %d`,
		contentArm, personPos, fmt.Sprintf(personReachesActivity, personPos), scope, projectScope(opts, arg), extra, sectionCap+1), args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make([]crmcontracts.Activity, 0, sectionCap)
	for rows.Next() {
		var a crmcontracts.Activity
		var id ids.UUID
		var audience string
		var version int64
		var contentAvailable, bulkMailAttested, filedHere bool
		var threadKey, audienceReason *string
		if err := rows.Scan(&id, &a.Kind, &a.ChannelProvider, &a.Subject, &a.Body,
			&a.Direction, &a.OccurredAt, &a.DueAt, &a.IsDone, &a.Source, &a.CapturedBy,
			&a.CreatedAt, &threadKey, &bulkMailAttested, &audience, &audienceReason,
			&version, &contentAvailable, &filedHere); err != nil {
			return nil, false, err
		}
		a.Id = openapi_types.UUID(id)
		aud := crmcontracts.ActivityAudience(audience)
		a.Audience = &aud
		a.Version = &version
		// The thread key and the bulk attestation are what lets the record
		// page fold this page into conversations the way the list's page
		// folds; the key identifies the message at the provider, so it is
		// withheld with the content, exactly as the list's scan withholds it.
		a.BulkMailAttested = &bulkMailAttested
		a.ThreadKey = threadKey
		// Why the row is held travels with the row. The record page seeds its
		// timeline from this read, so a reason dropped here is a reason the
		// timeline never has — and the timeline is where an owner decides
		// whether to share the thread.
		a.AudienceReason = audienceReason
		state := crmcontracts.ActivityContentStateAvailable
		if !contentAvailable {
			state = crmcontracts.ActivityContentStateWithheld
			// The reason describes what the message is about, so it is
			// withheld with the content: a colleague who may not read a held
			// message does not learn why it is held either.
			a.Subject, a.Body, a.ThreadKey, a.AudienceReason = nil, nil, nil, nil
		}
		a.ContentState = &state
		// Links say how the message is FILED, and a row can reach this page
		// without being filed here: a contact who was CC'd or who attended is on
		// the message through their participant row, while the filing belongs to
		// whoever capture named as its counterparty. Asserting a link for those
		// would describe a row activity_link does not hold — and a client acting
		// on it, to unfile the message, would act on nothing.
		if filedHere {
			a.Links = &[]crmcontracts.ActivityLink{{
				EntityType: crmcontracts.ActivityLinkEntityTypePerson,
				EntityId:   openapi_types.UUID(personID.UUID),
			}}
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(out) > sectionCap
	if hasMore {
		out = out[:sectionCap]
	}
	return out, hasMore, nil
}

// lastTouchSection reads the two directions separately. Folding them into
// one "last touch" hides the only distinction a reader acts on: a contact
// we mailed a fortnight ago with no reply and one who wrote to us this
// morning have the same last-touch date and opposite meanings.
func (s *Service) lastTouchSection(ctx context.Context, tx pgx.Tx, personID ids.PersonID, opts AssembleOptions, out *crmcontracts.Person360) error {
	if err := requireRead(ctx, "activity"); err != nil {
		return err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	personPos := arg(personID)
	scope, err := activityDiscoverScope(ctx, arg)
	if err != nil {
		return err
	}
	return tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT max(a.occurred_at) FILTER (WHERE a.direction = 'inbound'),
		       max(a.occurred_at) FILTER (WHERE a.direction = 'outbound')
		FROM activity a
		WHERE a.archived_at IS NULL AND %s AND (%s)%s`,
		fmt.Sprintf(personReachesActivity, personPos), scope, projectScope(opts, arg)), args...).
		Scan(&out.LastInboundAt, &out.LastOutboundAt)
}

// networkSection answers "who here knows them", warmest first — the
// ordering IS the answer, so it over-fetches and ranks before capping,
// exactly as GET /people/{id}/network does.
func (s *Service) networkSection(ctx context.Context, tx pgx.Tx, personID ids.PersonID, now time.Time, out *crmcontracts.Person360) error {
	edges, err := search.EdgesForPerson(ctx, tx, personID.UUID, networkFetch)
	if err != nil {
		return err
	}
	search.SortByStrength(edges, now)
	if len(edges) > networkCap {
		edges = edges[:networkCap]
	}
	names, err := network.UserNames(ctx, tx, network.EdgeUsers(edges))
	if err != nil {
		return err
	}
	colleagues := make([]crmcontracts.PersonNetworkColleague, 0, len(edges))
	for _, e := range edges {
		colleagues = append(colleagues, network.WireColleague(e, names[e.UserID], now))
	}
	out.Network = &struct {
		Colleagues []crmcontracts.PersonNetworkColleague `json:"colleagues"`
	}{Colleagues: colleagues}
	return nil
}

// networkCap and networkFetch mirror the standalone network endpoint: the
// record page must not name a different strongest colleague than the card.
const (
	networkCap   = 10
	networkFetch = 100
)

// consentSection is the outbound guard, not the ledger: per-purpose state
// only. The append-only proof log stays at GET /people/{id}/consent.
func (s *Service) consentSection(ctx context.Context, tx pgx.Tx, personID ids.PersonID, out *crmcontracts.Person360) error {
	states, _, err := s.consent.PersonConsentTx(ctx, tx, personID)
	if err != nil {
		return err
	}
	wire := make([]crmcontracts.PersonConsentState, 0, len(states))
	for _, st := range states {
		s := crmcontracts.PersonConsentState{
			PurposeId:              openapi_types.UUID(st.PurposeID.UUID),
			State:                  crmcontracts.PersonConsentStateState(st.State),
			LawfulBasis:            st.LawfulBasis,
			DoubleOptInConfirmedAt: st.DoubleOptInConfirmedAt,
			UpdatedAt:              st.UpdatedAt,
		}
		if st.PurposeKey != "" {
			key := st.PurposeKey
			s.PurposeKey = &key
		}
		wire = append(wire, s)
	}
	out.Consent = &struct {
		State []crmcontracts.PersonConsentState `json:"state"`
	}{State: wire}
	return nil
}

// sinceLastVisitSection counts what arrived since the caller's own
// baseline. READ-ONLY: nothing here advances the mark — only view-ack does,
// because a GET that moved it would destroy the answer the caller opened
// the page to read.
func (s *Service) sinceLastVisitSection(ctx context.Context, tx pgx.Tx, personID ids.PersonID, opts AssembleOptions, out *crmcontracts.Person360) error {
	if err := requireRead(ctx, "activity"); err != nil {
		return err
	}
	var view crmcontracts.Person360SinceLastVisit
	since, visited, err := s.baselineFor(ctx, tx, personID)
	if err != nil {
		return err
	}
	if visited {
		view.BaselineAt = &since
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	personPos, sincePos := arg(personID), arg(since)
	scope, err := activityScope(ctx, arg)
	if err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT count(*)
		FROM activity a
		WHERE a.archived_at IS NULL AND a.created_at > $%d AND %s AND (%s)%s`,
		sincePos, fmt.Sprintf(personReachesActivity, personPos), scope, projectScope(opts, arg)), args...).
		Scan(&view.NewActivities); err != nil {
		return fmt.Errorf("count new activities: %w", err)
	}
	out.SinceLastVisit = &view
	return nil
}

// actingUser resolves the user a baseline belongs to. It answers for agents
// too — an agent's UserID is the granting human's — so it is a lookup, not
// a gate: Acknowledge's auth.RequireHuman is what keeps an agent from
// writing that human's mark.
func actingUser(ctx context.Context) (ids.UserID, error) {
	p, ok := principal.Actor(ctx)
	if !ok || p.UserID == (ids.UUID{}) {
		return ids.UserID{}, fmt.Errorf(
			"the visit baseline is per-user and this call carries no user: %w",
			apperrors.ErrPermissionDenied)
	}
	return ids.From[ids.UserKind](p.UserID), nil
}

// baselineFor reads the caller's own mark. The user_id predicate is the
// whole scope and has to be written out: without it one rep would read
// another rep's reading history. It is also sufficient — core 0225
// collapsed user_record_view's unique key to (user_id, entity_type,
// entity_id).
func (s *Service) baselineFor(ctx context.Context, tx pgx.Tx, personID ids.PersonID) (at time.Time, visited bool, err error) {
	userID, err := actingUser(ctx)
	if err != nil {
		return time.Time{}, false, err
	}
	err = tx.QueryRow(ctx, `
		SELECT last_viewed_at FROM user_record_view
		WHERE user_id = $1 AND entity_type = $2 AND entity_id = $3`,
		userID, entityTypePerson, personID).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return at, true, nil
}

func ptr[T any](v T) *T { return &v }
