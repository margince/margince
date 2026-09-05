// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

// "View as buyer". A seller sees the room through the real public edge, as a
// PREVIEW participant: their own seat, read-only, invisible to buyers. A mock
// would drift from the release, paused and expired rules the moment one of
// them changed; a real session cannot.
//
// What keeps the preview harmless: the row is `preview` and `view` by CHECK,
// so no participant write can give it a voice in the room; it is outside the
// per-address uniqueness and excluded from every roster, count and the public
// link request, so a buyer never learns of it and the rep's own address never
// receives a mailed link; every public write refuses a preview session; and
// pausing or closing the room ends preview sessions while a real buyer's
// survive, since the rep has no reason to keep reading a room they froze.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// previewCredentialTTL is how long the rep has to open the tab. Minutes, not
// the week a buyer gets: the credential is handed to a browser in the same
// click, and anything longer is a link lying around.
const previewCredentialTTL = 10 * time.Minute

// previewSessionTTL bounds a preview tab left open.
const previewSessionTTL = time.Hour

// IssuedPreview is the one-time credential, returned once.
type IssuedPreview struct {
	Credential string
	ExpiresAt  time.Time
}

// PreviewRoom mints a credential for the caller's preview seat in the room,
// creating the seat on first use.
//
// Preview tabs the rep already has open are LEFT ALONE. Minting used to revoke
// them, which made the second "View as buyer" click kill the first tab: that
// tab then reported a dead link, and the rep read the failure as belonging to
// the link they had just made rather than to the one they had already opened.
// Superseding the unconsumed CREDENTIAL is enough to keep a minted-but-unopened
// link from lying around, and issueCredentialFor already does exactly that.
func (s *Store) PreviewRoom(ctx context.Context, roomID ids.DealRoomID) (IssuedPreview, error) {
	if err := previewAllowedForCaller(ctx); err != nil {
		return IssuedPreview{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return IssuedPreview{}, err
	}
	raw, digest, err := mintCredential()
	if err != nil {
		return IssuedPreview{}, err
	}
	var out IssuedPreview
	err = s.tx(ctx, func(tx pgx.Tx) error {
		room, err := readRoom(ctx, tx, roomID)
		if err != nil {
			return err
		}
		if err := ensureDealWritable(ctx, tx, room); err != nil {
			return err
		}
		if room.State == stateArchived {
			return notAdmitting(stateArchived)
		}
		seat, err := previewSeat(ctx, tx, roomID, by)
		if err != nil {
			return err
		}
		expiresAt, err := issueCredentialFor(ctx, tx, seat, digest, by, previewCredentialTTL)
		if err != nil {
			return err
		}
		// Audit-only: a preview is the seller looking at their own room, and
		// the catalog's invitation events are about buyers being admitted.
		if _, err := storekit.Audit(ctx, tx, "invite", participantObject, seat.UUID, nil,
			map[string]any{fieldRoomID: roomID.UUID, "preview": true}); err != nil {
			return fmt.Errorf("audit deal room preview: %w", err)
		}
		out = IssuedPreview{Credential: raw, ExpiresAt: expiresAt}
		return nil
	})
	return out, err
}

// previewAllowedForCaller is the half of the preview gate that depends on WHO
// is asking rather than on which room.
//
// Extracted so a room read can answer PreviewAvailable with the same rule the
// press will apply. Two spellings of "may this person preview" would agree
// until one of them changed, and the visible cost of that is a button offered
// and then refused — which is the state this exists to leave behind.
func previewAllowedForCaller(ctx context.Context) error {
	if err := auth.Require(ctx, roomObject, principal.ActionUpdate); err != nil {
		return err
	}
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	// RequireHuman admits the system principal; a preview is a person's act
	// on their own seat and a system caller has no seat to preview from.
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalHuman {
		return apperrors.ErrPermissionDenied
	}
	return nil
}

// StampPreviewAvailable answers, for a page of rooms, whether THIS caller could
// open each one's buyer preview — for a read that wants to say so before the
// press rather than after it.
//
// Every condition PreviewRoom applies: the caller's authority, the deal being
// writable and live, and the room not being archived. It mints nothing and
// writes nothing.
//
// SET-BASED, over all the rooms' deals at once, because the alternative is two
// queries a row on a page of up to two hundred. auth.StampWritable is the same
// answer every other capability boolean in the tree is stamped with
// (Deal.Writable, Lead.Writable), including its archived exclusion — a deal
// that is writable but archived is nobody's to present.
//
// An error PROPAGATES. Answering false on a failed probe would be worse than
// wrong: the probe runs on the caller's transaction, a failed statement leaves
// it aborted, and a swallowed error would let the read commit having verified
// nothing and tell a rep a database stall was a decision about their access.
func StampPreviewAvailable(ctx context.Context, tx pgx.Tx, rooms []crmcontracts.DealRoom) error {
	if len(rooms) == 0 {
		return nil
	}
	// The caller's own authority is asked once: it is the same answer for every
	// room on the page, and a denial here is an ordinary false rather than an
	// error — a colleague who may read rooms and not present them is the case
	// this field exists to describe.
	if err := previewAllowedForCaller(ctx); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			for i := range rooms {
				no := false
				rooms[i].PreviewAvailable = &no
			}
			return nil
		}
		return err
	}
	deals := make([]dealOfRoom, len(rooms))
	for i, room := range rooms {
		deals[i] = dealOfRoom{DealID: ids.UUID(room.DealId)}
	}
	writable, err := auth.StampWritable(ctx, tx, dealTable, deals,
		func(d dealOfRoom) ids.UUID { return d.DealID },
		func(d *dealOfRoom, may bool) { d.Writable = may })
	if err != nil {
		return err
	}
	for i := range rooms {
		// Archived is the room's OWN state, which StampWritable answers for the
		// deal rather than for the room.
		available := writable[ids.UUID(rooms[i].DealId)] && rooms[i].State != stateArchived
		rooms[i].PreviewAvailable = &available
	}
	return nil
}

// dealOfRoom carries one room's deal id through auth.StampWritable, which
// stamps a flag onto rows it is given rather than answering a bare set. The
// rooms themselves cannot be passed: the flag it would stamp is the DEAL's
// writability, and a room carries no such field to put it in.
type dealOfRoom struct {
	DealID   ids.UUID
	Writable bool
}

// previewSeat finds the caller's preview participant in the room, or creates
// it: the rep's own name and address, so a buyer-side audit row that names
// the participant names the rep.
func previewSeat(ctx context.Context, tx pgx.Tx, roomID ids.DealRoomID, by string) (ids.DealRoomParticipantID, error) {
	user := invitingUser(ctx)
	if user == nil {
		return ids.DealRoomParticipantID{}, apperrors.ErrPermissionDenied
	}
	var seat ids.DealRoomParticipantID
	err := tx.QueryRow(ctx,
		`SELECT id FROM deal_room_participant
		  WHERE room_id = $1 AND invited_by = $2 AND preview AND revoked_at IS NULL`,
		roomID, *user).Scan(&seat)
	if err == nil {
		return seat, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ids.DealRoomParticipantID{}, fmt.Errorf("find deal room preview seat: %w", err)
	}
	var name, email string
	if err := tx.QueryRow(ctx,
		`SELECT display_name, lower(email) FROM app_user WHERE id = $1 AND archived_at IS NULL`,
		*user).Scan(&name, &email); err != nil {
		return ids.DealRoomParticipantID{}, fmt.Errorf("read the previewing user: %w", err)
	}
	seat = ids.New[ids.DealRoomParticipantKind]()
	if _, err := tx.Exec(ctx,
		`INSERT INTO deal_room_participant
		     (id, room_id, full_name, email, capability, invited_by, preview, source, captured_by)
		 VALUES ($1, $2, $3, $4, $5, $6, true, $7, $8)`,
		seat, roomID, name, email, capabilityView, *user, sourceCredential, by); err != nil {
		return ids.DealRoomParticipantID{}, fmt.Errorf("insert deal room preview seat: %w", err)
	}
	return seat, nil
}

// endPreviewSessions is what pausing and closing do to preview sessions: the
// rep froze the room, and a preview tab still reading it would only tell
// them what they already know. A real buyer's session is untouched.
func endPreviewSessions(ctx context.Context, tx pgx.Tx, roomID ids.DealRoomID) error {
	if _, err := tx.Exec(ctx,
		`UPDATE deal_room_session s SET revoked_at = now()
		   FROM deal_room_participant p
		  WHERE p.id = s.participant_id AND p.room_id = $1 AND p.preview AND s.revoked_at IS NULL`,
		roomID); err != nil {
		return fmt.Errorf("end deal room preview sessions: %w", err)
	}
	// And the credential not yet opened: a preview minted a minute before the
	// pause must not become a session a minute after it.
	if _, err := tx.Exec(ctx,
		`UPDATE deal_room_invitation i SET superseded_at = now()
		   FROM deal_room_participant p
		  WHERE p.id = i.participant_id AND p.room_id = $1 AND p.preview
		    AND i.consumed_at IS NULL AND i.superseded_at IS NULL`,
		roomID); err != nil {
		return fmt.Errorf("retire deal room preview credentials: %w", err)
	}
	return nil
}

// errPreviewSession refuses a write from a preview session. Read everything,
// change nothing: a rep's question to their own room would otherwise be
// recorded as the buyer's.
var errPreviewSession = &fieldError{
	field: fieldCapability,
	code:  "preview_session",
	msg:   "a preview cannot write; sign in as a buyer to comment",
}
