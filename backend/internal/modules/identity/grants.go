// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Manual per-record sharing (A52/ADR-0039): identity owns the grant
// rows because a grant IS access administration — platform/auth's
// visibility predicates read the table by SQL, never by import. A
// grant widens own/team base scope for exactly one record; revocation
// binds on the next query because the predicate evaluates live.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var shareableRecordTypes = map[string]bool{
	"contact": true, "company": true, "deal": true, "lead": true, "project": true,
}

// grantSubjectTeam is the subject type naming a team, and the table it lives in.
const grantSubjectTeam = "team"

const grantColumns = `id, record_type, record_id, subject_type, subject_id, access, granted_by, reason, expires_at, created_at, version`

type grantRow struct {
	ID          ids.UUID
	RecordType  string
	RecordID    ids.UUID
	SubjectType string
	SubjectID   ids.UUID
	Access      string
	GrantedBy   ids.UUID
	Reason      *string
	ExpiresAt   *time.Time
	CreatedAt   time.Time
	Version     int64
}

func scanGrant(r pgx.Row) (grantRow, error) {
	var g grantRow
	err := r.Scan(&g.ID, &g.RecordType, &g.RecordID, &g.SubjectType, &g.SubjectID,
		&g.Access, &g.GrantedBy, &g.Reason, &g.ExpiresAt, &g.CreatedAt, &g.Version)
	return g, err
}

type ListGrantsInput struct {
	RecordType  *string
	RecordID    *ids.UUID
	SubjectType *string
	SubjectID   *ids.UUID
	Cursor      *string
	Limit       *int
}

type CreateGrantInput struct {
	RecordType  string
	RecordID    ids.UUID
	SubjectType string
	SubjectID   ids.UUID
	Access      string
	Reason      *string
	ExpiresAt   *time.Time
}

// refuseAnExpiryAlreadyPast refuses a grant that would be dead on arrival.
//
// Both predicates that read this column test `expires_at IS NULL OR expires_at
// > now()`, so a back-dated grant is inert from the instant it is written. Inert
// is not harmless, because the ROW IS THERE: the share list names somebody who
// in fact has no access, and a human scanning who can see a record is told the
// opposite of the truth. Nothing sweeps the table either — expiry is evaluated
// live and no writer deletes an expired row — so the mistake accumulates.
//
// And it looked like success. No refusal, no field error, nothing to tell the
// caller their date was wrong, which is what makes this a validation gap rather
// than a quirk (#2139).
//
// The same refusal covers the RE-ASSERT, which shares this function: shortening
// a live grant's expiry to a past instant is a revoke wearing an expiry's
// clothes, and the product has an explicit revoke for that. A caller who means
// to end access now says so, and the audit records what they meant.
//
// Equal to now is refused with the past. The predicates are strict — `>` — so
// an expiry landing exactly on the instant it is written is already spent.
func refuseAnExpiryAlreadyPast(expiresAt *time.Time, now time.Time) error {
	if expiresAt == nil || expiresAt.After(now) {
		return nil
	}
	return httperr.Validation("expires_at", "not_in_future",
		"expires_at must be in the future: a grant that has already expired is stored and "+
			"immediately inert, and it lists as an active share while granting nothing. "+
			"To end an existing share now, revoke it")
}

// refuseUnknownGrantVocabulary names the enum field the caller got wrong. The
// table's CHECK constraints would refuse the same values, but only after the
// row-scope and subject reads, and with no field named.
func refuseUnknownGrantVocabulary(in CreateGrantInput) error {
	if !shareableRecordTypes[in.RecordType] {
		return httperr.Validation("record_type", "invalid",
			"record_type must be one of "+strings.Join(slices.Sorted(maps.Keys(shareableRecordTypes)), ", "))
	}
	if !crmcontracts.CreateRecordGrantRequestSubjectType(in.SubjectType).Valid() {
		return httperr.Validation("subject_type", "invalid", "subject_type must be user or team")
	}
	if !crmcontracts.CreateRecordGrantRequestAccess(in.Access).Valid() {
		return httperr.Validation("access", "invalid", "access must be read or write")
	}
	return nil
}

func (s *Service) CreateRecordGrant(ctx context.Context, in CreateGrantInput) (grantRow, error) {
	if err := s.refuseMalformedGrant(ctx, in); err != nil {
		return grantRow{}, err
	}
	var out grantRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = createRecordGrantInTx(ctx, tx, in)
		return err
	})
	return out, err
}

// CreateRecordGrantTx is CreateRecordGrant on a transaction the caller owns,
// for a share that must commit with the decision releasing it. The checks are
// the same, in the same order.
func (s *Service) CreateRecordGrantTx(ctx context.Context, tx pgx.Tx, in CreateGrantInput) error {
	if err := s.refuseMalformedGrant(ctx, in); err != nil {
		return err
	}
	_, err := createRecordGrantInTx(ctx, tx, in)
	return err
}

// refuseMalformedGrant is what a share settles before any transaction opens.
func (s *Service) refuseMalformedGrant(ctx context.Context, in CreateGrantInput) error {
	// Both ids, and both before anything else: a grant is a triple of record,
	// subject and access, and each id is required by the contract — a claim only
	// this check makes true. Unguarded, a zero record_id reaches the row-scope
	// probe and a zero subject_id reaches the subject lookup, and each answers
	// not-found for something the caller never named. The record refusal comes
	// first because it is what the grant is ABOUT.
	if err := httperr.RequireBodyID("record_id", in.RecordID); err != nil {
		return err
	}
	if err := httperr.RequireBodyID("subject_id", in.SubjectID); err != nil {
		return err
	}
	if err := refuseUnknownGrantVocabulary(in); err != nil {
		return err
	}
	if err := refuseAnExpiryAlreadyPast(in.ExpiresAt, s.now()); err != nil {
		return err
	}
	// Sharing widens who sees the record — the grantor needs the
	// record's own write grant (the spec's manage_sharing permission,
	// ADR-0039, is not yet in this build's policy vocabulary).
	return auth.Require(ctx, in.RecordType, principal.ActionUpdate)
}

// createRecordGrantInTx is the share itself: who may grant it, who may hold
// it, and the audited upsert.
func createRecordGrantInTx(ctx context.Context, tx pgx.Tx, in CreateGrantInput) (grantRow, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return grantRow{}, errors.New("crmauth: only a human shares records directly; agents stage through the approval gate")
	}
	// Scope-intersection: you can only share what you can see (H1
	// probe on the client-supplied record reference).
	if err := auth.EnsureLinkTarget(ctx, tx, in.RecordType, in.RecordID); err != nil {
		return grantRow{}, err
	}
	subjectTable := "app_user"
	if in.SubjectType == grantSubjectTeam {
		subjectTable = grantSubjectTeam
	}
	var subjectExists bool
	if err := tx.QueryRow(ctx,
		storekit.SQLf(`SELECT EXISTS (SELECT 1 FROM %s WHERE id = $1)`, subjectTable),
		in.SubjectID).Scan(&subjectExists); err != nil {
		return grantRow{}, err
	}
	if !subjectExists {
		return grantRow{}, apperrors.ErrNotFound
	}
	// Two rules bind a grant, and they judge different seats.
	//
	// Scope-intersection (ADR-0039) judges the grantor at every access level.
	// EnsureLinkTarget above admits a caller whose only claim is a share, so
	// authority is asked separately here. The upsert restates the whole grant,
	// its term and its width, so a `read` assertion needs it as a `write` does.
	//
	// The seat ceiling (AAD-AC-4) judges the recipient by the asserted access.
	// A `read` grant to a read seat is fine. Both rules bind a re-assert as they
	// bind a first share.
	if err := auth.EnsureCanGrant(ctx, tx, in.RecordType, in.RecordID); err != nil {
		return grantRow{}, err
	}
	if err := refuseWriteGrantToReadSeat(ctx, tx, in); err != nil {
		return grantRow{}, err
	}
	// The precondition read and the write it feeds run under one lock. FOR
	// UPDATE cannot order two callers creating this grant at once, because an
	// absent row locks nothing. The loser would then audit a first share over a
	// row it displaced.
	if err := storekit.LockWriteIdentity(ctx, tx, "record_grant", grantIdentity(in)); err != nil {
		return grantRow{}, err
	}
	prior, replaced, err := replacedGrant(ctx, tx, in)
	if err != nil {
		return grantRow{}, err
	}
	var before map[string]any
	if replaced {
		before = grantImage(prior)
	}
	// A grant is identified by its natural key, so a re-assert restates the
	// same row. `expires_at` takes the proposed value even when it is NULL: the
	// contract says a re-assert resets it, and a COALESCE would make an expiry
	// unclearable. `granted_by` moves to the caller, who is now accountable.
	out, err := upsertGrant(ctx, tx, in, actor.UserID)
	if err != nil {
		return grantRow{}, err
	}
	// `record_share` in both directions, downgrades included: the contract pins
	// the verb, and `record_unshare` belongs to revocation. Both images render
	// the persisted row through one shape. An image that omitted a field the
	// upsert can change would report that nothing moved when it did.
	if _, err := storekit.Audit(ctx, tx, "record_share",
		in.RecordType, in.RecordID, before, grantImage(out)); err != nil {
		return grantRow{}, fmt.Errorf("audit record_share: %w", err)
	}
	return out, nil
}

// upsertGrant writes one grant by its natural key, the one writer of a
// record_grant row's terms. The caller holds the write-identity lock.
func upsertGrant(ctx context.Context, tx pgx.Tx, in CreateGrantInput, grantedBy ids.UUID) (grantRow, error) {
	values := []any{in.RecordType, in.RecordID, in.SubjectType, in.SubjectID, in.Access, grantedBy, in.Reason, in.ExpiresAt}
	placeholders := make([]string, len(values))
	for i := range values {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	out, err := scanGrant(tx.QueryRow(ctx, `
		INSERT INTO record_grant (record_type, record_id, subject_type, subject_id,
		                          access, granted_by, reason, expires_at)
		VALUES (`+strings.Join(placeholders, ", ")+`)
		ON CONFLICT (record_type, record_id, subject_type, subject_id) DO UPDATE
		SET access     = EXCLUDED.access,
		    expires_at = EXCLUDED.expires_at,
		    reason     = EXCLUDED.reason,
		    granted_by = EXCLUDED.granted_by,
		    version    = record_grant.version + 1
		RETURNING `+grantColumns, values...))
	if err != nil {
		return grantRow{}, fmt.Errorf("upsert record_grant: %w", err)
	}
	return out, nil
}

// grantIdentity renders the natural key the contract identifies a grant by —
// the same four columns the unique constraint and the upsert's conflict target
// use, so the lock and the row it protects can never key on different things.
func grantIdentity(in CreateGrantInput) string {
	return in.RecordType + ":" + in.RecordID.String() + ":" + in.SubjectType + ":" + in.SubjectID.String()
}

// replacedGrant reads the grant this assertion is about to displace, keyed the
// way the contract identifies one. `replaced` is false for a tuple never
// granted before, which is the audit's own distinction between a first share
// and a re-statement — it is not derivable from the row, because an absent
// grant and a grant with every field empty are different facts.
//
// It reads FOR UPDATE, which holds an existing row against a concurrent
// re-assert; the caller takes the write-identity lock first, which is what
// covers the case where there is no row to hold yet.
func replacedGrant(ctx context.Context, tx pgx.Tx, in CreateGrantInput) (prior grantRow, replaced bool, err error) {
	prior, err = scanGrant(tx.QueryRow(ctx, "SELECT "+grantColumns+` FROM record_grant
		WHERE record_type = $1 AND record_id = $2 AND subject_type = $3 AND subject_id = $4
		FOR UPDATE`,
		in.RecordType, in.RecordID, in.SubjectType, in.SubjectID))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return grantRow{}, false, nil
	case err != nil:
		return grantRow{}, false, fmt.Errorf("read the displaced record_grant: %w", err)
	}
	return prior, true, nil
}

// grantImage renders one audit image of a grant. Before and after share it so
// the pair diffs to exactly what a re-assert moved, which means it has to carry
// every field the upsert can change — `granted_by` included, because a
// re-assert moves accountability to the caller and the audit row's own actor
// records only the after side of that.
func grantImage(g grantRow) map[string]any {
	return map[string]any{
		"subject_type": g.SubjectType, "subject_id": g.SubjectID, "access": g.Access,
		"reason": g.Reason, "expires_at": g.ExpiresAt, "granted_by": g.GrantedBy,
	}
}

// refuseWriteGrantToReadSeat holds the receiving half of the seat ceiling
// (AAD-AC-4): a read seat may be handed a record to READ, which is exactly
// the authority its licence already carries, but never one to write.
//
// The granting half is enforced a layer up — sharing is a POST, so a
// read-seat grantor never reaches here — and it is not the same rule. A
// full-seat admin sharing a deal for editing with a read-seat colleague is
// the case this closes: the grant would have widened that colleague's row
// scope onto a record the seat ceiling then refuses every write to, which
// is a grant that reads as authority and cannot be exercised.
//
// A `team` subject is deliberately out of scope: a team is not a seat, and
// refusing a whole team because one member reads is a wider rule than the
// AC states. The read seats inside it are still refused every write at
// their own admission, so no authority leaks — the grant is just less
// useful to them than to their colleagues.
//
// WHOEVER SHIPS A SEAT-CHANGE ENDPOINT OWES THE OTHER HALF. This guard runs at
// grant creation and nowhere else, so a member downgraded to a read seat keeps
// every write grant they were given while they held a full one. That is the
// rule and the stored data disagreeing, and the fix belongs in the seat
// write's own transaction: revoke the subject's write grants there, audited
// like any other grant change, so the data matches the rule at every instant.
// A downgrade quietly parking authority is how somebody gets it back on an
// upgrade nobody re-examined.
//
// Until then it is inert rather than wrong: platform/auth's write-authority
// predicate drops its record_grant arm for a read-seat principal, so the
// standing grant confers nothing at the place authority is actually read. That
// is the second guard, it holds whatever the row says, and it is not a reason
// to skip the revocation — data that states the opposite of the rule is a
// defect the next reader inherits.
func refuseWriteGrantToReadSeat(ctx context.Context, tx pgx.Tx, in CreateGrantInput) error {
	if in.Access != string(crmcontracts.RecordGrantAccessRecordGrantAccessWrite) || in.SubjectType == grantSubjectTeam {
		return nil
	}
	seat, err := seatOf(ctx, tx, in.SubjectID)
	if err != nil {
		return err
	}
	if !seat.CanMutate() {
		return fmt.Errorf("a write grant needs a full seat; the subject holds %q: %w",
			seat, apperrors.ErrSeatTierInsufficient)
	}
	return nil
}

// ShareAccessForTx is the widest access the seat ceiling lets a share give this
// member. It is write for a seat that may write, and read otherwise.
func ShareAccessForTx(ctx context.Context, tx pgx.Tx, member ids.UUID) (string, error) {
	seat, err := seatOf(ctx, tx, member)
	if err != nil {
		return "", err
	}
	if seat.CanMutate() {
		return string(crmcontracts.RecordGrantAccessRecordGrantAccessWrite), nil
	}
	return string(crmcontracts.RecordGrantAccessRecordGrantAccessRead), nil
}

func seatOf(ctx context.Context, tx pgx.Tx, member ids.UUID) (principal.SeatType, error) {
	var seat string
	if err := tx.QueryRow(ctx,
		`SELECT seat_type FROM app_user WHERE id = $1`, member).Scan(&seat); err != nil {
		return "", fmt.Errorf("read the subject's seat: %w", err)
	}
	return principal.SeatType(seat), nil
}

// mayRevoke answers who may take a share away, and it is deliberately not the
// same question as who may see the record.
//
// Revoking is administration OF the sharing, so it wants the same authority
// asserting one does (ADR-0039's scope-intersection rule, EnsureCanGrant):
// otherwise anyone the record was ever shared with — read-only — could delete a
// colleague's `write` grant on it, which is not an escalation but is a way to
// take work away from contacts who are doing it. The write probe is what stops
// that, and it keeps the read half's 404 first, so a caller who cannot see the
// record still learns nothing from the shape of the refusal.
//
// The one arm that is NOT about authority over the record is a subject
// declining their own share. That was possible before this rule and stays
// possible: the grant names them, taking it away costs nobody anything, and
// nothing else in the product lets a contact get out from under a share they did
// not ask for. A TEAM grant is not covered — its subject is the team, not the
// member reading it — so removing one is the record's business.
func mayRevoke(ctx context.Context, tx pgx.Tx, actor principal.Principal, grant grantRow) error {
	if grant.SubjectType == "user" && grant.SubjectID == actor.UserID {
		return nil
	}
	// Both gates sit INSIDE this branch, and that placement is the exception
	// working rather than a tidy-up. The object grant reads "may this role
	// change records of this kind" — a question declining your own share does
	// not ask, and one a read-only seat answers no to. Checking it before the
	// self arm would have left exactly the seat most likely to be handed a
	// share it did not want with no way to give it back.
	if err := auth.Require(ctx, grant.RecordType, principal.ActionUpdate); err != nil {
		return err
	}
	// Retractable, not live. A share standing on an archived record is stale
	// state nobody can tidy if this probe requires liveness — and retiring the
	// record is precisely when somebody reaches to clear the shares hanging off
	// it. auth.EnsureRetractable holds the rule.
	return auth.EnsureRetractable(ctx, tx, grant.RecordType, grant.RecordID)
}

// RevokeRecordGrant deletes one share. A nil ifVersion revokes whatever the
// grant now says; a stale one refuses, since a re-assert may have changed it.
func (s *Service) RevokeRecordGrant(ctx context.Context, id ids.UUID, ifVersion *int64) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return errors.New("crmauth: only a human revokes shares directly; agents stage through the approval gate")
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		// FOR UPDATE for the reason the assertion path takes it: this row is
		// both the thing being deleted and the audit's before image, and a
		// concurrent re-assert would otherwise let the trail record an access
		// level that was already displaced by the time the DELETE ran.
		grant, err := scanGrant(tx.QueryRow(ctx,
			"SELECT "+grantColumns+" FROM record_grant WHERE id = $1 FOR UPDATE", id))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := mayRevoke(ctx, tx, actor, grant); err != nil {
			return err
		}
		if ifVersion != nil && *ifVersion != grant.Version {
			return apperrors.ErrVersionSkew
		}
		if _, err := tx.Exec(ctx, `DELETE FROM record_grant WHERE id = $1`, id); err != nil {
			return err
		}
		// The same image the assertion path renders, so a record's timeline
		// carries one shape: a share and an unshare of the same grant differ in
		// which side is absent, not in which fields they thought worth naming.
		_, err = storekit.Audit(ctx, tx, "record_unshare",
			grant.RecordType, grant.RecordID, grantImage(grant), nil)
		return err
	})
}

func wireGrant(g grantRow) crmcontracts.RecordGrant {
	out := crmcontracts.RecordGrant{
		Id:          openapi_types.UUID(g.ID),
		RecordType:  crmcontracts.RecordGrantRecordType(g.RecordType),
		RecordId:    openapi_types.UUID(g.RecordID),
		SubjectType: crmcontracts.RecordGrantSubjectType(g.SubjectType),
		SubjectId:   openapi_types.UUID(g.SubjectID),
		Access:      crmcontracts.RecordGrantAccess(g.Access),
		GrantedBy:   openapi_types.UUID(g.GrantedBy),
		Reason:      g.Reason,
		ExpiresAt:   g.ExpiresAt,
		CreatedAt:   g.CreatedAt,
		Version:     g.Version,
	}
	return out
}
