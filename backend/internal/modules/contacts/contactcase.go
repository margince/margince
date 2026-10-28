// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A contact case is every live contact reachable from one contact over open
// duplicate pairs whose both ends the reader can see. It is read off
// dedupe_candidate and never stored, so dismissing, re-opening or merging a
// pair re-shapes it at once. The lowest member id is its anchor.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// contactCaseMaxMembers is how many contacts one review may merge. A larger
// component still decides pair by pair.
const contactCaseMaxMembers = 25

type caseMember struct {
	ID        ids.UUID
	Name      string
	Email     *string
	Title     *string
	Company   *string
	OwnerID   *ids.UUID
	OwnerName *string
	// OwnerLive is a live human seat owning the member; OwnerSeat is its seat.
	OwnerLive bool
	OwnerSeat string
	Version   int64
	CreatedAt time.Time
}

type caseEdge struct {
	ID          ids.UUID
	Left, Right ids.UUID
	Confidence  float64
	Disposition string
	Evidence    json.RawMessage
}

type contactCase struct {
	Members  []caseMember
	Edges    []caseEdge
	TooLarge bool
}

func (c contactCase) anchor() ids.UUID { return c.Members[0].ID }

func (c contactCase) member(id ids.UUID) (caseMember, bool) {
	i := slices.IndexFunc(c.Members, func(m caseMember) bool { return m.ID == id })
	if i < 0 {
		return caseMember{}, false
	}
	return c.Members[i], true
}

// confidence is the strongest open pair in the case.
func (c contactCase) confidence() float64 {
	best := 0.0
	for _, e := range c.Edges {
		if e.Disposition == dispositionOpen && e.Confidence > best {
			best = e.Confidence
		}
	}
	return best
}

// revision changes whenever a member, an owner, a version, a pair or the
// policy changes; a decision refuses a stale one.
func (c contactCase) revision(policy bool) string {
	var b strings.Builder
	for _, m := range c.Members {
		owner := ""
		if m.OwnerID != nil {
			owner = m.OwnerID.String()
		}
		b.WriteString("m:" + m.ID.String() + ":" + strconv.FormatInt(m.Version, 10) + ":" + owner + "\n")
	}
	for _, e := range c.Edges {
		b.WriteString("p:" + e.ID.String() + ":" + e.Disposition + "\n")
	}
	b.WriteString("policy:" + strconv.FormatBool(policy) + "\n")
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// liveOwners are the distinct live owners of the given members, in id order.
func (c contactCase) liveOwners(memberIDs []ids.UUID) []ids.UUID {
	var owners []ids.UUID
	for _, id := range memberIDs {
		if m, ok := c.member(id); ok && m.OwnerLive && !slices.Contains(owners, *m.OwnerID) {
			owners = append(owners, *m.OwnerID)
		}
	}
	slices.SortFunc(owners, compareUUID)
	return owners
}

func compareUUID(a, b ids.UUID) int { return slices.Compare(a[:], b[:]) }

// openContactPairs is the edge rule a case is walked over. A pair counts when
// it is open and both ends pass the queue's own live visibility clause.
func openContactPairs(ctx context.Context, arg func(any) int) (string, error) {
	visible, err := dedupeVisibilityClause(ctx, arg, true)
	if err != nil {
		return "", err
	}
	return `dedupe_candidate.entity_type = 'contact' AND dedupe_candidate.disposition = 'open'
	    AND dedupe_candidate.archived_at IS NULL AND ` + visible, nil
}

// walkContactCase reads the case holding start. A contact in no open pair the
// reader can see answers an empty case.
func walkContactCase(ctx context.Context, tx pgx.Tx, start ids.UUID) (contactCase, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	startPos := arg(start)
	pairs, err := openContactPairs(ctx, arg)
	if err != nil {
		return contactCase{}, err
	}
	bound := arg(contactCaseMaxMembers + 1)
	// UNION drops a contact already walked, so a cycle of pairs ends. The LIMIT
	// stops one past the bound, which marks the case as too large.
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		WITH RECURSIVE walked(id) AS (
		  SELECT $%d::uuid
		  UNION
		  SELECT CASE WHEN dedupe_candidate.left_contact_id = walked.id
		              THEN dedupe_candidate.right_contact_id ELSE dedupe_candidate.left_contact_id END
		    FROM walked JOIN dedupe_candidate
		      ON walked.id IN (dedupe_candidate.left_contact_id, dedupe_candidate.right_contact_id)
		   WHERE %s)
		SELECT id FROM walked LIMIT $%d`, startPos, pairs, bound), args...)
	if err != nil {
		return contactCase{}, fmt.Errorf("contacts: walking the duplicate case: %w", err)
	}
	memberIDs, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return contactCase{}, fmt.Errorf("contacts: walking the duplicate case: %w", err)
	}
	if len(memberIDs) < 2 {
		return contactCase{}, nil
	}
	c := contactCase{TooLarge: len(memberIDs) > contactCaseMaxMembers}
	slices.SortFunc(memberIDs, compareUUID)
	if c.TooLarge {
		memberIDs = memberIDs[:contactCaseMaxMembers]
	}
	if c.Members, err = readCaseMembers(ctx, tx, memberIDs); err != nil {
		return contactCase{}, err
	}
	if c.Edges, err = readCaseEdges(ctx, tx, memberIDs); err != nil {
		return contactCase{}, err
	}
	return c, nil
}

// liveCaseOwnerSQL is whether the joined owner seat u still counts as an owner
// of a case. It asks what identity asks before carrying a former owner's
// access, so the owners a case names are the ones a merge can carry.
var liveCaseOwnerSQL = "coalesce(u.removed_at IS NULL AND " + workforce.LiveMemberSQL("u") + ", false)"

// readCaseMembers reads what a reviewer compares, in id order. The employer is
// the list's own, so a company hidden from the reader stays hidden here.
func readCaseMembers(ctx context.Context, tx pgx.Tx, memberIDs []ids.UUID) ([]caseMember, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	email, err := orderByReachableEmail(ctx, arg)
	if err != nil {
		return nil, err
	}
	employer, err := orderByCurrentEmployer(ctx, arg)
	if err != nil {
		return nil, err
	}
	members := arg(memberIDs)
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT contact.id, contact.full_name, %s, contact.title, %s,
		       contact.owner_id, u.display_name,
		       %s, coalesce(u.seat_type, ''),
		       contact.version, contact.created_at
		  FROM contact LEFT JOIN app_user u ON u.id = contact.owner_id
		 WHERE contact.id = ANY($%d) AND contact.archived_at IS NULL
		 ORDER BY contact.id`, email, employer, liveCaseOwnerSQL, members), args...)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the case members: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (caseMember, error) {
		var m caseMember
		err := row.Scan(&m.ID, &m.Name, &m.Email, &m.Title, &m.Company, &m.OwnerID, &m.OwnerName,
			&m.OwnerLive, &m.OwnerSeat, &m.Version, &m.CreatedAt)
		return m, err
	})
}

// readCaseEdges reads the pairs between members that a reviewer weighs: open
// ones, and dismissed ones so nobody merges what a colleague called different
// contacts. The walk's visibility clause bounds both ends again.
func readCaseEdges(ctx context.Context, tx pgx.Tx, memberIDs []ids.UUID) ([]caseEdge, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	members := arg(memberIDs)
	visible, err := dedupeVisibilityClause(ctx, arg, true)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT id, left_contact_id, right_contact_id, confidence, evidence, disposition
		  FROM dedupe_candidate
		 WHERE entity_type = 'contact' AND archived_at IS NULL
		   AND disposition IN ('open', 'not_a_duplicate')
		   AND left_contact_id = ANY($%[1]d) AND right_contact_id = ANY($%[1]d)
		   AND %[2]s
		 ORDER BY id`, members, visible), args...)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the case pairs: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (caseEdge, error) {
		var e caseEdge
		err := row.Scan(&e.ID, &e.Left, &e.Right, &e.Confidence, &e.Evidence, &e.Disposition)
		return e, err
	})
}

// actorOwnsAMember says whether the caller is the live full-seat owner of one
// of the given members, the standing a delegated merge rests on.
func (c contactCase) actorOwnsAMember(actor principal.Principal, memberIDs []ids.UUID) bool {
	for _, id := range memberIDs {
		m, ok := c.member(id)
		if ok && m.OwnerLive && *m.OwnerID == actor.UserID && m.OwnerSeat == string(principal.SeatFull) {
			return true
		}
	}
	return false
}
