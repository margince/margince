// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The basis of a delayed send is every obligation its words name, with the
// rows and versions they rest on. Every delayed send reads through
// readSendPayload and reserves through reserveSend. The reservation commits
// only when each row is unchanged and still authorised for the recipient.
// The message goes to the relay after that commit, and is never taken back.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// sendRefs names what a payload shows: the seat's own tasks, tasks shown as
// a colleague's on the seat's team, and decisions waiting on the seat.
// records are the targets of lines that report what already happened. They
// are no one's duty, but the reader must still be allowed to see them.
type sendRefs struct {
	ownTasks, teamTasks, approvals []ids.UUID
	records                        []notices.Target
}

func (r *sendRefs) canonical() {
	for _, set := range []*[]ids.UUID{&r.ownTasks, &r.teamTasks, &r.approvals} {
		slices.SortFunc(*set, compareUUID)
		*set = slices.Compact(*set)
	}
	slices.SortFunc(r.records, func(a, b notices.Target) int {
		if a.Type != b.Type {
			return strings.Compare(a.Type, b.Type)
		}
		return compareUUID(a.ID, b.ID)
	})
	r.records = slices.Compact(r.records)
}

func compareUUID(a, b ids.UUID) int {
	for i := range a {
		if a[i] != b[i] {
			return int(a[i]) - int(b[i])
		}
	}
	return 0
}

// basisRow is one row the payload rests on, at the version it was read. A
// commitment admission is keyed by its source and locator. Its version is a
// fingerprint of the row.
type basisRow struct {
	table   string
	id      ids.UUID
	key     string
	version int64
}

// commitmentAdmissionTable names the recorded decision a claimless
// commitment proposal rests on.
const commitmentAdmissionTable = "activity_commitment_admission"

// sendBasis is the payload's rows and the recipient they are authorised for.
// excluded names the shown items that no longer qualify; a payload is read
// again without them rather than sent with them.
type sendBasis struct {
	seat     ids.UUID
	team     *ids.UUID
	refs     sendRefs
	rows     []basisRow
	excluded map[ids.UUID]bool
}

// errSendBasisChanged ends a reservation whose basis moved since it was read.
var errSendBasisChanged = errors.New("send: the work the message names changed before its send was reserved")

// readSendBasis reads, per shown item, the rows it rests on. A task rests on
// its source; a commitment decision rests on its promises and source. A task
// qualifies while it is open and the seat's, or its team's when shown as a
// colleague's. A decision qualifies while it waits on the seat. Promises must
// be open and the source live and readable by the seat. An item that fails is
// excluded, and its rows are not part of the basis.
func readSendBasis(ctx context.Context, tx pgx.Tx, seat ids.UUID, team *ids.UUID, refs sendRefs, now time.Time) (sendBasis, error) {
	refs.canonical()
	basis := sendBasis{seat: seat, team: team, refs: refs, excluded: map[ids.UUID]bool{}}
	shown := map[ids.UUID]bool{}
	for _, set := range [][]ids.UUID{refs.ownTasks, refs.teamTasks, refs.approvals} {
		for _, id := range set {
			shown[id] = true
		}
	}
	ctx, live, err := basisAuthorityAndRecords(ctx, tx, seat, refs, shown, &basis)
	if err != nil || !live {
		return basis, err
	}
	read, err := readShownItems(ctx, tx, seat, team, refs, now)
	if err != nil {
		return basis, err
	}
	for _, it := range read {
		delete(shown, it.row.id)
		deps, ok, err := shownItemDependencies(ctx, tx, it)
		if err != nil {
			return basis, err
		}
		if !it.eligible || !ok {
			basis.excluded[it.row.id] = true
			continue
		}
		basis.rows = append(append(basis.rows, it.row), deps...)
	}
	for id := range shown {
		basis.excluded[id] = true
	}
	return basis, nil
}

// shownItem is one task or decision a payload shows, as the basis read found it.
type shownItem struct {
	row      basisRow
	eligible bool
	claims   []ids.UUID
	source   *ids.UUID
	locator  *string
	target   *string
	targetID *ids.UUID
}

// readShownItems reads each shown task and decision with whether it still
// qualifies and what it rests on.
func readShownItems(ctx context.Context, tx pgx.Tx, seat ids.UUID, team *ids.UUID, refs sendRefs, now time.Time) ([]shownItem, error) {
	args := []any{slices.Concat(refs.ownTasks, refs.teamTasks), seat, team, refs.teamTasks, refs.approvals, CommitmentTaskKind, now}
	arg := func(value any) int { args = append(args, value); return len(args) }
	identities, err := storekit.MailSeatIdentitiesSQL(ctx, tx, arg)
	if err != nil {
		return nil, err
	}
	currentWork := storekit.CurrentTaskClause("t", now, identities, arg)
	items, err := tx.Query(ctx, `SELECT t.id, 'activity', t.version, (`+sendTaskEligibleSQL+` AND `+currentWork+`), '{}'::uuid[], t.source_activity_id, NULL::text, NULL::text, NULL::uuid
		  FROM activity t WHERE t.kind = 'task' AND t.id = ANY($1)
		UNION ALL
		SELECT a.id, 'approval', a.version, `+sendApprovalEligibleSQL+`,
		       coalesce((SELECT array_agg(c::uuid) FROM jsonb_array_elements_text(coalesce(a.proposed_change->'claim_ids', '[]'::jsonb)) c), '{}')
		         || CASE WHEN a.proposed_change ? 'claim_id' THEN ARRAY[(a.proposed_change->>'claim_id')::uuid] ELSE '{}' END,
		       CASE WHEN a.kind = $6 THEN (a.proposed_change->>'source_activity_id')::uuid END,
		       CASE WHEN a.kind = $6 THEN a.proposed_change->>'locator' END,
		       CASE WHEN a.kind <> $6 THEN a.target_entity_type END, CASE WHEN a.kind <> $6 THEN a.target_entity_id END
		  FROM approval a WHERE a.id = ANY($5)
		ORDER BY 1`, args...)
	if err != nil {
		return nil, fmt.Errorf("send: reading the work a message names: %w", err)
	}
	var read []shownItem
	var item shownItem
	_, err = pgx.ForEachRow(items, []any{&item.row.id, &item.row.table, &item.row.version, &item.eligible, &item.claims, &item.source, &item.locator, &item.target, &item.targetID}, func() error {
		read = append(read, item)
		return nil
	})
	return read, err
}

// shownItemDependencies is everything one shown item rests on, and whether
// all of it still qualifies.
func shownItemDependencies(ctx context.Context, tx pgx.Tx, it shownItem) ([]basisRow, bool, error) {
	deps, ok, err := itemDependencies(ctx, tx, it.claims, it.source, it.locator)
	if err != nil || !ok || it.target == nil || it.targetID == nil {
		return deps, ok, err
	}
	target, ok, err := targetDependency(ctx, tx, *it.target, *it.targetID)
	if err != nil {
		return nil, false, err
	}
	return append(deps, target...), ok, nil
}

// basisAuthorityAndRecords binds the recipient's authority as this
// transaction reads it, and puts the reported records into the basis. A
// recipient who is no longer live excludes everything shown.
func basisAuthorityAndRecords(ctx context.Context, tx pgx.Tx, seat ids.UUID, refs sendRefs, shown map[ids.UUID]bool, basis *sendBasis) (context.Context, bool, error) {
	// The context a send started with carries the grants of that moment. Every
	// check below runs as the recipient's authority read in this transaction.
	ctx, live, err := seatAuthorityTx(ctx, tx, seat)
	if err != nil || !live {
		for id := range shown {
			basis.excluded[id] = true
		}
		for _, record := range refs.records {
			basis.excluded[record.ID] = true
		}
		return ctx, false, err
	}
	for _, record := range refs.records {
		rows, ok, err := recordDependency(ctx, tx, record)
		if err != nil {
			return ctx, false, err
		}
		if !ok {
			basis.excluded[record.ID] = true
			continue
		}
		basis.rows = append(basis.rows, rows...)
	}
	return ctx, true, nil
}

// sendTaskEligibleSQL is an open task the seat holds, or one a colleague on
// the seat's team holds when it was shown as theirs.
const sendTaskEligibleSQL = `(t.archived_at IS NULL AND NOT t.is_done AND (t.assignee_id = $2
   OR (t.id = ANY($4) AND EXISTS (SELECT 1 FROM team_membership m WHERE m.team_id = $3 AND m.user_id = t.assignee_id))))`

// sendApprovalEligibleSQL is a decision still waiting on the seat.
const sendApprovalEligibleSQL = `(a.status = 'pending' AND a.requested_decider_id = $2 AND (a.expires_at IS NULL OR a.expires_at > $7))`

// itemDependencies reads the promises and source one item rests on, and
// whether all still qualify. A commitment decision that names no claim rests
// on the recorded admission of its promise instead, as its preview does.
func itemDependencies(ctx context.Context, tx pgx.Tx, claims []ids.UUID, source *ids.UUID, locator *string) ([]basisRow, bool, error) {
	var deps []basisRow
	slices.SortFunc(claims, compareUUID)
	claims = slices.Compact(claims)
	for _, claim := range claims {
		row, ok, err := currentRow(ctx, tx, "conversation_claim", claim, `status = 'open' AND archived_at IS NULL`)
		if err != nil || !ok {
			return nil, false, err
		}
		deps = append(deps, row)
	}
	if source == nil {
		return deps, true, nil
	}
	if len(claims) == 0 && locator != nil {
		fingerprint, open, err := activities.CommitmentAdmissionTx(ctx, tx, *source, *locator)
		if err != nil || !open {
			return nil, false, err
		}
		deps = append(deps, basisRow{table: commitmentAdmissionTable, id: *source, key: *locator, version: fingerprint})
	}
	row, ok, err := sourceDependency(ctx, tx, *source, sendSourceCurrentSQL(string(recordTypeActivity)))
	if err != nil || !ok {
		return nil, false, err
	}
	return append(deps, row), true, nil
}

// sourceDependency is an activity a message quotes or reports, current by
// live, and still readable by the recipient's own content scope.
func sourceDependency(ctx context.Context, tx pgx.Tx, source ids.UUID, live string) (basisRow, bool, error) {
	row, ok, err := currentRow(ctx, tx, string(recordTypeActivity), source, live)
	if err != nil || !ok {
		return row, false, err
	}
	// The object grant comes first: the row and audience check below asks only
	// which rows the reader's scope reaches, not whether they may read activity.
	if err := auth.Require(ctx, "activity", principal.ActionRead); errors.Is(err, apperrors.ErrPermissionDenied) {
		return row, false, nil
	} else if err != nil {
		return row, false, err
	}
	// An audience can narrow through a team without the row changing, so the
	// seat is asked whether it may still open the source. In a savepoint: a
	// refused read must not abort the transaction for the items still readable.
	if err := pgx.BeginFunc(ctx, tx, func(savepoint pgx.Tx) error {
		return auth.EnsureActivityContentVisibleLive(ctx, savepoint, source)
	}); err != nil {
		return row, false, nil //nolint:nilerr // A source the seat may no longer open excludes its item.
	}
	return row, true, nil
}

// reportedActivitySQL is when an activity a line only reports on is current:
// live and unrestricted. The line is history about the row, not work judged
// from its text, so the body's completeness does not apply.
func reportedActivitySQL(alias string) string {
	return alias + ".archived_at IS NULL AND " + alias + ".restricted_at IS NULL"
}

// recordDependency is the record a reported line names. An activity is held as
// a source is, through the recipient's content scope. A deal, company or
// contact is held as a decision's target is. Any other row-scoped record is
// checked for visibility only, the way the digest names it.
func recordDependency(ctx context.Context, tx pgx.Tx, record notices.Target) ([]basisRow, bool, error) {
	if record.Type == string(recordTypeActivity) {
		row, ok, err := sourceDependency(ctx, tx, record.ID, reportedActivitySQL(string(recordTypeActivity)))
		if err != nil || !ok {
			return nil, false, err
		}
		return []basisRow{row}, true, nil
	}
	if slices.Contains(decisionTargets, record.Type) {
		return targetDependency(ctx, tx, record.Type, record.ID)
	}
	if !auth.RowScoped(record.Type) {
		return nil, true, nil
	}
	visible, err := auth.VisibleSubset(ctx, tx, record.Type, []ids.UUID{record.ID})
	return nil, err == nil && visible[record.ID], err
}

// seatAuthorityTx binds the recipient's authority as this transaction reads
// it. A recipient who is no longer a live member answers false.
func seatAuthorityTx(ctx context.Context, tx pgx.Tx, seat ids.UUID) (context.Context, bool, error) {
	rbac, seatType, err := identity.EffectiveAuthorityTx(ctx, tx, seat)
	if errors.Is(err, apperrors.ErrNotFound) {
		return ctx, false, nil
	}
	if err != nil {
		return ctx, false, fmt.Errorf("send: reading the recipient's current authority: %w", err)
	}
	return boundToSeat(ctx, seat, rbac, seatType), true, nil
}

// decisionTargets are the records a decision other than a commitment names.
// The reader must still see the record, and it is held at the version read.
// Each is a table name spelled here, never taken from the row.
var decisionTargets = []string{string(recordTypeDeal), string(recordTypeCompany), string(recordTypeContact)}

// targetDependency is the record a decision is about, when it is one of
// decisionTargets: whether the reader still sees it, and its version.
func targetDependency(ctx context.Context, tx pgx.Tx, targetType string, id ids.UUID) ([]basisRow, bool, error) {
	at := slices.Index(decisionTargets, targetType)
	if at < 0 {
		return nil, true, nil
	}
	table := decisionTargets[at]
	visible, err := auth.VisibleSubset(ctx, tx, table, []ids.UUID{id})
	if err != nil || !visible[id] {
		return nil, false, err
	}
	row, ok, err := currentRow(ctx, tx, table, id, `archived_at IS NULL`)
	if err != nil || !ok {
		return nil, false, err
	}
	return []basisRow{row}, true, nil
}

// sendSourceCurrentSQL is the one test a delayed send asks of the source its
// words quote. It runs at the read and again under the hold. The source must be
// live, unrestricted, and a mail read in full (provenance.MailBodyCompleteSQL).
// A completeness change that moves no version is still caught by that re-read.
func sendSourceCurrentSQL(alias string) string {
	return reportedActivitySQL(alias) + " AND " + provenance.MailBodyCompleteSQL(alias)
}

func currentRow(ctx context.Context, tx pgx.Tx, table string, id ids.UUID, live string) (basisRow, bool, error) {
	row := basisRow{table: table, id: id}
	var current bool
	err := tx.QueryRow(ctx, `SELECT version, `+live+` FROM `+table+` WHERE id = $1`, id).Scan(&row.version, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, false, nil
	}
	if err != nil {
		return row, false, fmt.Errorf("send: reading what a message rests on: %w", err)
	}
	return row, current, nil
}

// holdSendBasisTx locks every basis row and refuses the reservation when one
// changed, stopped qualifying, or the recipient left. It also locks the
// authority the basis was read under: the seat, the team memberships it reads
// and the audience rows of every source. A change to any of them waits for the
// reservation. The clock is read after the locks, so a decision that expired
// during a wait is refused.
func holdSendBasisTx(ctx context.Context, tx pgx.Tx, basis sendBasis, clock func() time.Time) error {
	if err := holdBasisRows(ctx, tx, basis.rows); err != nil {
		return err
	}
	var sources []ids.UUID
	for _, row := range basis.rows {
		if row.table == string(recordTypeActivity) {
			sources = append(sources, row.id)
		}
	}
	var live int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM app_user WHERE id = $1 AND `+workforce.LiveMemberSQL("")+` FOR SHARE) seat`,
		basis.seat).Scan(&live); err != nil {
		return fmt.Errorf("send: holding the recipient: %w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM team_membership m WHERE m.user_id = $1
		    OR (m.team_id = $2 AND m.user_id IN (SELECT assignee_id FROM activity WHERE id = ANY($3) AND archived_at IS NULL))
		   FOR SHARE OF m`, basis.seat, basis.team, basis.refs.teamTasks); err != nil {
		return fmt.Errorf("send: holding the recipient's teams: %w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM activity_audience_member am WHERE am.activity_id = ANY($1) FOR SHARE OF am`, sources); err != nil {
		return fmt.Errorf("send: holding who may read the sources: %w", err)
	}
	if live == 0 {
		return errSendBasisChanged
	}
	// The versions held, so the basis is read again under the authority this
	// transaction reads, with the clock read after the locks.
	again, err := readSendBasis(ctx, tx, basis.seat, basis.team, basis.refs, clock())
	if err != nil {
		return err
	}
	if !slices.Equal(again.rows, basis.rows) || len(again.excluded) != len(basis.excluded) {
		return errSendBasisChanged
	}
	return nil
}

// holdBasisRows takes a share lock on each basis row at the version read, and
// refuses when one is gone or changed.
func holdBasisRows(ctx context.Context, tx pgx.Tx, basis []basisRow) error {
	for _, table := range slices.Concat([]string{string(recordTypeActivity), "approval", "conversation_claim"}, decisionTargets) {
		var rows []ids.UUID
		var versions []int64
		for _, row := range basis {
			if row.table == table {
				rows, versions = append(rows, row.id), append(versions, row.version)
			}
		}
		if len(rows) == 0 {
			continue
		}
		var held int
		if err := tx.QueryRow(ctx, `WITH locked AS (SELECT held.id FROM `+table+` held
		       JOIN unnest($1::uuid[], $2::bigint[]) AS basis(id, version) ON basis.id = held.id AND basis.version = held.version
		       FOR SHARE OF held)
		     SELECT count(*) FROM locked`, rows, versions).Scan(&held); err != nil {
			return fmt.Errorf("send: holding what a message rests on: %w", err)
		}
		if held != len(rows) {
			return errSendBasisChanged
		}
	}
	for _, row := range basis {
		if row.table != commitmentAdmissionTable {
			continue
		}
		fingerprint, open, err := activities.HoldCommitmentAdmissionTx(ctx, tx, row.id, row.key)
		if err != nil {
			return err
		}
		if !open || fingerprint != row.version {
			return errSendBasisChanged
		}
	}
	return nil
}
