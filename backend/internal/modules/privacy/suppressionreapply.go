// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Erased stays erased, whatever brought it back.
//
// The suppression list guards the door mail comes through; a restore puts rows
// back underneath it. A resurrected subject is invisible because the record
// looks exactly like one that was never erased, so the list is re-applied
// blind to how the row arrived.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/corefeatures"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ReapplySuppressionsReason is the reason the re-erasure records, so the
// second tombstone is distinguishable from the first: the trail should say a
// subject was erased twice and why, not appear to repeat itself.
const ReapplySuppressionsReason = "resurrected after erasure; suppression list reapplied"

// ReapplySuppressions re-erases every live subject whose address or channel
// identity is on the suppression list, and reports how many it found.
//
// Zero is the answer on a healthy installation, and that is the point: the
// pass is cheap to run and its result is evidence either way. A non-zero
// answer is not a failure of this pass — it is this pass catching one.
//
// The hashing happens in Go, through the same SuppressionForms and
// ChannelIdentityForms the eraser's hashes are matched with. In SQL, lower() and btrim()
// would stand in for Go's normalization, and the two agree only on ASCII. A
// second spelling of the hash silently forks the list, which is the mistake
// suppression.go names in its own header.
func (e *Eraser) ReapplySuppressions(ctx context.Context) (int, error) {
	// The same gate the erasure itself asks for, asked once at the top. The
	// scan below reads every contact in the installation to decide which ones
	// the list already ended, and a caller who may not delete a contact has no
	// business being handed that reading either.
	ctx = corefeatures.RetainedDataContext(ctx)
	if err := auth.Require(ctx, "contact", principal.ActionDelete); err != nil {
		return 0, err
	}
	suppressed, err := e.suppressedHashes(ctx)
	if err != nil {
		return 0, err
	}
	// An empty list is the restore case the journal answers; keep scanning.
	resurrected, err := e.contactsMatchingSuppression(ctx, suppressed)
	if err != nil {
		return 0, err
	}
	var taken int
	var refused error
	for _, contactID := range resurrected {
		// One transaction each, through the ordinary erasure: a pass that
		// reached into the cascade's steps would be a second eraser to keep in
		// step with the first, and the thing most likely to drift is exactly
		// the part that decides what "erased" means.
		//
		// A subject the eraser refuses does NOT stop the others. A legal hold
		// is the refusal that matters here and it is per subject: one held
		// contact halting the pass would leave every resurrection after them
		// standing, and the hold — which is somebody else's obligation over
		// one record — would silently become a reason the whole installation
		// kept data it was told to erase. So the pass takes what it may, and
		// the refusals travel back joined rather than swallowed: the count
		// says what was done and the error says what could not be.
		if err := e.EraseContact(replayingErasure(ctx), contactID, ReapplySuppressionsReason); err != nil {
			refused = errors.Join(refused,
				fmt.Errorf("privacy: re-erasing resurrected contact %s: %w", contactID, err))
			continue
		}
		taken++
	}
	return taken, refused
}

// suppressionEntry is one row of the list: the kind of identifier and its
// hash. The kind is part of the key, because the journal files each kind
// under its own prefix.
type suppressionEntry struct{ kind, hash string }

// suppressionState is what the list says about one entry. A lifted entry still
// erases a contact created before the lift: that contact is the erased one,
// brought back by a restore. The subject's own signup is created at the lift.
type suppressionState struct {
	erasedAt time.Time
	liftedAt *time.Time
}

// changedAt is when the entry last changed, to tell the newer of two answers.
func (s suppressionState) changedAt() time.Time {
	if s.liftedAt != nil {
		return *s.liftedAt
	}
	return s.erasedAt
}

// erases reports whether the entry erases a contact created at created.
func (s suppressionState) erases(created time.Time) bool {
	return s.liftedAt == nil || created.Before(*s.liftedAt)
}

// suppressedHashes reads the standing instruction, every kind of it.
func (e *Eraser) suppressedHashes(ctx context.Context) (map[suppressionEntry]suppressionState, error) {
	hashes := map[suppressionEntry]suppressionState{}
	if err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT kind, value_hash, created_at, lifted_at FROM erasure_suppression`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var entry suppressionEntry
			var state suppressionState
			if err := rows.Scan(&entry.kind, &entry.hash, &state.erasedAt, &state.liftedAt); err != nil {
				return err
			}
			hashes[entry] = state
		}
		return rows.Err()
	}); err != nil {
		return nil, fmt.Errorf("privacy: reading the suppression list: %w", err)
	}
	return hashes, nil
}

// liveIdentifier is one identifier of a live contact, in every form the list
// can store it.
type liveIdentifier struct {
	contact ids.UUID
	created time.Time
	forms   []string
}

// contactsMatchingSuppression finds the live contacts an erasure should
// already have taken, by address or by channel identity.
//
// Every live identifier is hashed, rather than every suppressed hash being
// searched for. The list is the smaller side on a healthy installation, and
// the comparison has to happen in Go anyway.
func (e *Eraser) contactsMatchingSuppression(ctx context.Context, suppressed map[suppressionEntry]suppressionState) ([]ids.UUID, error) {
	// Each kind on the list, with the live contacts' identifiers of that kind
	// and how the eraser hashed them.
	liveIdentifierQueries := []struct {
		kind  string
		query string
		scan  func(rows pgx.Rows) (liveIdentifier, error)
	}{
		{
			kind: "email",
			query: `
				SELECT ce.contact_id, c.created_at, ce.email
				  FROM contact_email ce
				  JOIN contact c ON c.id = ce.contact_id AND c.archived_at IS NULL
				 ORDER BY ce.contact_id`,
			scan: func(rows pgx.Rows) (liveIdentifier, error) {
				var live liveIdentifier
				var email string
				err := rows.Scan(&live.contact, &live.created, &email)
				live.forms = storekit.SuppressionForms(email)
				return live, err
			},
		},
		{
			kind: "channel_identity",
			query: `
				SELECT ci.contact_id, c.created_at, ci.provider, ci.channel_user_id
				  FROM contact_channel_identity ci
				  JOIN contact c ON c.id = ci.contact_id AND c.archived_at IS NULL
				 ORDER BY ci.contact_id`,
			scan: func(rows pgx.Rows) (liveIdentifier, error) {
				var live liveIdentifier
				var provider, account string
				err := rows.Scan(&live.contact, &live.created, &provider, &account)
				live.forms = storekit.ChannelIdentityForms(provider, account)
				return live, err
			},
		},
	}
	// The database read finishes before any of it is asked about. Probing the
	// journal inside the cursor would hold one transaction open across a
	// network round trip per identifier.
	var matched []ids.UUID
	var ask []suppressionCandidate
	seen := map[ids.UUID]struct{}{}
	if err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		for _, source := range liveIdentifierQueries {
			rows, err := tx.Query(ctx, source.query)
			if err != nil {
				return err
			}
			for rows.Next() {
				live, err := source.scan(rows)
				if err != nil {
					rows.Close()
					return err
				}
				if _, already := seen[live.contact]; already {
					continue
				}
				erases, newest := listAnswer(suppressed, source.kind, live)
				if erases {
					seen[live.contact] = struct{}{}
					matched = append(matched, live.contact)
					continue
				}
				ask = append(ask, suppressionCandidate{liveIdentifier: live, kind: source.kind, listChanged: newest})
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("privacy: looking for resurrected subjects: %w", err)
	}

	return e.matchJournaled(ctx, ask, seen, matched)
}

// matchJournaled asks the exported journal about what the database's own list
// did not erase. That is the restore case: the rows came back, and the list
// that would have caught them rolled back with them. A journal entry counts
// only when it is newer than the list's own row. A journal exported before a
// lift so does not erase the subject's own signup.
func (e *Eraser) matchJournaled(
	ctx context.Context, ask []suppressionCandidate, seen map[ids.UUID]struct{}, matched []ids.UUID,
) ([]ids.UUID, error) {
	for _, c := range ask {
		if _, already := seen[c.contact]; already {
			continue
		}
		states, err := e.journalStatesInAnyForm(ctx, c.kind, c.forms)
		if err != nil {
			return nil, err
		}
		if !slices.ContainsFunc(states, func(state suppressionState) bool {
			return state.changedAt().After(c.listChanged) && state.erases(c.created)
		}) {
			continue
		}
		seen[c.contact] = struct{}{}
		matched = append(matched, c.contact)
	}
	return matched, nil
}

// suppressionCandidate is a live contact's identifier that the database's own
// list does not erase, still to be asked of the journal. listChanged is the
// newest change the list holds for it, zero when it holds none.
type suppressionCandidate struct {
	liveIdentifier
	kind        string
	listChanged time.Time
}

// listAnswer reports whether the list erases the identifier in any of its
// forms, keyed or unkeyed. It also answers the newest change it holds for them.
func listAnswer(suppressed map[suppressionEntry]suppressionState, kind string, live liveIdentifier) (bool, time.Time) {
	var newest time.Time
	for _, form := range live.forms {
		state, ok := suppressed[suppressionEntry{kind: kind, hash: form}]
		if !ok {
			continue
		}
		if state.erases(live.created) {
			return true, newest
		}
		if state.changedAt().After(newest) {
			newest = state.changedAt()
		}
	}
	return false, newest
}
