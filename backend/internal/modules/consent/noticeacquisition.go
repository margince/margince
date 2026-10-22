// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// NoticeAcquisition is the evidence a duty's deadline was computed from, so a
// reader can judge a deadline years in the past instead of trusting it.
type NoticeAcquisition struct {
	Kind string
	// OccurredAt is nil where the recording door did not know; the deadline
	// then ran from CapturedAt, as compose/noticecaseopen.go dates it.
	OccurredAt     *time.Time
	CapturedAt     time.Time
	CapturedBy     string
	CapturedByName *string
}

// acquisitionsFor reads the evidence behind a page of duties in one statement,
// keyed by acquisition id. A seat name resolves only for a live human seat, as
// SeatNames answers, so an archived one reads as a former member.
func acquisitionsFor(ctx context.Context, tx pgx.Tx, acquisitionIDs []ids.UUID) (map[ids.UUID]NoticeAcquisition, error) {
	out := make(map[ids.UUID]NoticeAcquisition, len(acquisitionIDs))
	if len(acquisitionIDs) == 0 {
		return out, nil
	}
	args := []any{acquisitionIDs}
	rows, err := tx.Query(ctx, storekit.SQLf(`
		SELECT e.id, e.kind, e.occurred_at, e.captured_at, e.captured_by, u.display_name
		  FROM contact_acquisition_evidence e
		  LEFT JOIN app_user u ON e.captured_by = 'human:' || u.id::text AND u.archived_at IS NULL
		 WHERE e.id = ANY($%d)`, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("consent: reading the acquisition behind these duties: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var evidence NoticeAcquisition
		if err := rows.Scan(&id, &evidence.Kind, &evidence.OccurredAt, &evidence.CapturedAt,
			&evidence.CapturedBy, &evidence.CapturedByName); err != nil {
			return nil, fmt.Errorf("consent: reading the acquisition behind a duty: %w", err)
		}
		out[id] = evidence
	}
	return out, rows.Err()
}

// attachAcquisitions puts each case's evidence on it. A case whose evidence is
// gone keeps a nil Acquisition, which the wire says rather than inventing one.
func attachAcquisitions(ctx context.Context, tx pgx.Tx, cases []NoticeCase) error {
	wanted := make([]ids.UUID, 0, len(cases))
	for _, c := range cases {
		wanted = append(wanted, c.AcquisitionID)
	}
	found, err := acquisitionsFor(ctx, tx, wanted)
	if err != nil {
		return err
	}
	for i := range cases {
		if evidence, ok := found[cases[i].AcquisitionID]; ok {
			cases[i].Acquisition = &evidence
		}
	}
	return nil
}

// Wire is the one place the evidence crosses into the contract. The privacy
// queue and the attention lane share it, so neither can spell it differently.
func (a *NoticeAcquisition) Wire() *crmcontracts.NoticeAcquisition {
	if a == nil {
		return nil
	}
	return &crmcontracts.NoticeAcquisition{
		Kind:           a.Kind,
		OccurredAt:     a.OccurredAt,
		CapturedAt:     a.CapturedAt,
		CapturedBy:     a.CapturedBy,
		CapturedByName: a.CapturedByName,
	}
}

// attachOneAcquisition is attachAcquisitions for the single-case reads and
// writes, so a case answers the same evidence whichever route returned it.
func attachOneAcquisition(ctx context.Context, tx pgx.Tx, c *NoticeCase) error {
	one := []NoticeCase{*c}
	if err := attachAcquisitions(ctx, tx, one); err != nil {
		return err
	}
	*c = one[0]
	return nil
}
