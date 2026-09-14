// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The morning digest builder (CAP-DDL-6, ADR-0063): the nightly suite's
// last pass assembles, per connected user, what capture did in the last
// day and what awaits review — every number a count of persisted rows at
// build time, stored as the pre-assembled payload one indexed GET serves
// (CAP-WIRE-6). Reads cross module tables freely (reads are governed by
// RLS, not ownership); writes only capture_digest.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DigestPayload is the stored CAP-DDL-6 payload — the wire shape verbatim.
type DigestPayload struct {
	Date        string          `json:"date"`
	GeneratedAt time.Time       `json:"generated_at"`
	Capture     DigestCapture   `json:"capture"`
	Review      DigestReview    `json:"review"`
	Connectors  []DigestConnRow `json:"connectors"`
	// Projects is what moved on the bodies of work overnight
	// (digestprojects.go). Absent when the build had no source for it.
	Projects *DigestProjects `json:"projects,omitempty"`
}

// DigestCapture is what landed in the window.
type DigestCapture struct {
	MessagesSynced    int `json:"messages_synced"`
	ActivitiesCreated int `json:"activities_created"`
	ContactsCreated   int `json:"contacts_created"`
	CompaniesCreated  int `json:"companies_created"`
}

// DigestReview is what awaits the human.
type DigestReview struct {
	// Both are per-reader counts and both are OPTIONAL, which the wire has
	// always allowed. Absent means the build could not count for this reader —
	// the review seam is unwired — and that is a different statement from zero.
	// Zero says "nothing is waiting for you"; absent says "we did not count",
	// and a build that reported the first when it meant the second would be the
	// same dishonest number this section replaced, pointing the other way.
	DedupeOpen       *int           `json:"dedupe_open,omitempty"`
	ApprovalsPending *int           `json:"approvals_pending,omitempty"`
	Classify         DigestClassify `json:"classify"`
}

// DigestClassify is the window's label tally.
type DigestClassify struct {
	Commitments int `json:"commitments"`
	Meetings    int `json:"meetings"`
	Noise       int `json:"noise"`
}

// DigestConnRow is one connector's health line.
type DigestConnRow struct {
	Provider       string     `json:"provider"`
	Status         string     `json:"status"`
	LastSyncedAt   *time.Time `json:"last_synced_at,omitempty"`
	LastErrorClass *string    `json:"last_sync_error_class,omitempty"`
}

// BuildDigests assembles one digest per user holding a live connection in
// the current workspace, for digestDate covering the window since the
// previous build day. Idempotent per (user, day): a re-run replaces the
// day's payload (the counts are as-of-now truths, not increments).
func (r *Registry) BuildDigests(ctx context.Context, digestDate time.Time) error {
	day := digestDate.Format(time.DateOnly)
	since := time.Date(digestDate.Year(), digestDate.Month(), digestDate.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	return r.db.Tx(ctx, func(tx pgx.Tx) error {
		users, err := connectedUsers(ctx, tx)
		if err != nil {
			return err
		}
		for _, userID := range users {
			payload, err := r.buildDigestPayload(ctx, tx, userID, day, since)
			if err != nil {
				return err
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("capture: encoding digest: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO capture_digest (user_id, digest_date, payload)
				VALUES ($1, $2, $3)
				ON CONFLICT (user_id, digest_date) DO UPDATE SET payload = EXCLUDED.payload`,
				userID, day, raw); err != nil {
				return fmt.Errorf("capture: storing digest: %w", err)
			}
		}
		return nil
	})
}

// connectedUsers lists seats with a live capture connection.
func connectedUsers(ctx context.Context, tx pgx.Tx) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT user_id FROM capture_connection
		WHERE status IN ('connected','error') AND archived_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("capture: listing digest users: %w", err)
	}
	defer rows.Close()
	var out []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *Registry) buildDigestPayload(ctx context.Context, tx pgx.Tx, userID ids.UUID, day string, since time.Time) (DigestPayload, error) {
	p := DigestPayload{Date: day, GeneratedAt: r.now().UTC()}
	if err := readDigestCounts(ctx, tx, userID, since, p.GeneratedAt, &p); err != nil {
		return DigestPayload{}, err
	}

	// The connector health strip is the USER's own connections (RC-8).
	rows, err := tx.Query(ctx, `
		SELECT c.provider, c.status, s.last_synced_at, s.last_error_class
		FROM capture_connection c
		LEFT JOIN capture_sync_state s ON s.connection_id = c.id
		WHERE c.user_id = $1 AND c.archived_at IS NULL
		ORDER BY c.provider`, userID)
	if err != nil {
		return DigestPayload{}, fmt.Errorf("capture: digest connector strip: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c DigestConnRow
		if err := rows.Scan(&c.Provider, &c.Status, &c.LastSyncedAt, &c.LastErrorClass); err != nil {
			return DigestPayload{}, err
		}
		p.Connectors = append(p.Connectors, c)
	}
	if p.Connectors == nil {
		p.Connectors = []DigestConnRow{}
	}
	if err := rows.Err(); err != nil {
		return DigestPayload{}, err
	}
	// Everything below is per READER rather than per workspace: it names
	// records, and which records a contact can see is theirs. One context binds
	// that reader once, with the live authority the resolver answers.
	if r.digestProjects == nil && r.digestReview == nil {
		return p, nil
	}
	readerCtx, err := r.digestReaderContext(ctx, userID)
	if err != nil {
		return DigestPayload{}, err
	}
	if err := r.refreshDigestReview(readerCtx, &p); err != nil {
		return DigestPayload{}, err
	}

	if r.digestProjects != nil {
		// The section names projects and the work on them, so a user with no
		// project grant gets none rather than an empty one.
		projects, err := r.digestProjects(readerCtx, tx, since, p.GeneratedAt)
		if err != nil {
			return DigestPayload{}, fmt.Errorf("capture: digest projects section: %w", err)
		}
		p.Projects = projects
	}
	return p, nil
}

// digestReaderContext binds the digest's reader as the acting principal,
// with the LIVE authority the resolver answers — the same source the
// connector context reads the granting human's from.
func (r *Registry) digestReaderContext(ctx context.Context, userID ids.UUID) (context.Context, error) {
	wsID, ok := principal.WorkspaceID(ctx)
	if !ok {
		return nil, errors.New("capture: digest build outside workspace context")
	}
	rbac, err := r.authority.EffectiveRBAC(ctx, wsID, userID)
	if err != nil {
		return nil, fmt.Errorf("capture: resolving the digest reader's authority: %w", err)
	}
	return principal.WithActor(ctx, principal.Principal{
		Type:        principal.PrincipalHuman,
		ID:          "human:" + userID.String(),
		UserID:      userID,
		TeamIDs:     rbac.TeamIDs,
		Permissions: rbac.Permissions,
	}), nil
}

// ReadDigest serves the calling user's digest: the requested day, or the
// latest when day is zero. No digest yet answers (nil, nil) — the
// transport's honest 404.
func (r *Registry) ReadDigest(ctx context.Context, userID ids.UUID, day *time.Time) (*DigestPayload, error) {
	var raw []byte
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var row pgx.Row
		if day != nil {
			row = tx.QueryRow(ctx,
				`SELECT payload FROM capture_digest WHERE user_id = $1 AND digest_date = $2`,
				userID, day.Format(time.DateOnly))
		} else {
			row = tx.QueryRow(ctx,
				`SELECT payload FROM capture_digest WHERE user_id = $1 ORDER BY digest_date DESC LIMIT 1`, userID)
		}
		err := row.Scan(&raw)
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("capture: reading digest: %w", err)
	}
	if raw == nil {
		// Absence IS the answer: no digest has been built yet — the
		// transport's honest 404, not an error.
		return nil, nil //nolint:nilnil // deliberate: state "none" precedes the first nightly build
	}
	var p DigestPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("capture: decoding stored digest: %w", err)
	}
	readerCtx, err := r.digestReaderContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := r.refreshDigestReview(readerCtx, &p); err != nil {
		return nil, err
	}
	digestDay, err := time.Parse(time.DateOnly, p.Date)
	if err != nil {
		return nil, fmt.Errorf("capture: reading digest day: %w", err)
	}
	if err := r.db.Tx(readerCtx, func(tx pgx.Tx) error {
		since := digestDay.AddDate(0, 0, -1)
		if err := readDigestCounts(readerCtx, tx, userID, since, p.GeneratedAt, &p); err != nil {
			return err
		}
		if r.digestProjects != nil {
			projects, err := r.digestProjects(readerCtx, tx, since, p.GeneratedAt)
			if err != nil {
				return err
			}
			p.Projects = projects
		} else {
			p.Projects = nil
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &p, nil
}

// Pending decisions follow live authority and state, including on cached days.
func (r *Registry) refreshDigestReview(ctx context.Context, p *DigestPayload) error {
	p.Review.DedupeOpen, p.Review.ApprovalsPending = nil, nil
	if r.digestReview == nil {
		return nil
	}
	review, err := r.digestReview(ctx)
	if err != nil {
		return fmt.Errorf("capture: digest review counts: %w", err)
	}
	p.Review.DedupeOpen, p.Review.ApprovalsPending = &review.DedupeOpen, &review.ApprovalsPending
	return nil
}
