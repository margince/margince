// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s *Service) framework(ctx context.Context, tx pgx.Tx) (crmcontracts.ReportingFramework, error) {
	out := crmcontracts.ReportingFramework{Definition: crmcontracts.ReportingFrameworkInput{Template: reportingSales, Qualification: []crmcontracts.ReportingQualification{}, CaptureContexts: []crmcontracts.ReportingCaptureContext{}, Reason: "Not configured"}}
	err := tx.QueryRow(ctx, "SELECT f.revision,f.version,r.definition,r.effective_at FROM reporting_framework f JOIN reporting_framework_revision r ON r.revision=f.revision").Scan(&out.Revision, &out.Version, &out.Definition, &out.EffectiveAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	return out, err
}

// GetFramework returns the current stage definitions used by metric evaluation.
func (s *Service) GetFramework(ctx context.Context) (crmcontracts.ReportingFramework, error) {
	if err := auth.Require(ctx, "reporting_framework", principal.ActionRead); err != nil {
		return crmcontracts.ReportingFramework{}, err
	}
	var out crmcontracts.ReportingFramework
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error { var err error; out, err = s.framework(ctx, tx); return err })
	return out, err
}

// PublishFramework appends a revision instead of changing historical definitions.
func (s *Service) PublishFramework(ctx context.Context, version int64, in crmcontracts.ReportingFrameworkInput) (crmcontracts.ReportingFramework, error) {
	if err := requireReadWrite(ctx, "reporting_framework", principal.ActionUpdate); err != nil {
		return crmcontracts.ReportingFramework{}, err
	}
	if err := validateFrameworkInput(in); err != nil {
		return crmcontracts.ReportingFramework{}, err
	}
	author, err := actorID(ctx)
	if err != nil {
		return crmcontracts.ReportingFramework{}, err
	}
	var out crmcontracts.ReportingFramework
	err = s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO reporting_framework(singleton,revision,version) VALUES (true,0,0) ON CONFLICT DO NOTHING"); err != nil {
			return err
		}
		var current int64
		var frameworkID ids.UUID
		if err := tx.QueryRow(ctx, "SELECT version,id FROM reporting_framework FOR UPDATE").Scan(&current, &frameworkID); err != nil {
			return err
		}
		if current != version {
			return apperrors.ErrVersionSkew
		}
		before, err := s.framework(ctx, tx)
		if err != nil {
			return err
		}
		if err := s.authority.ValidateFramework(ctx, tx, in); err != nil {
			return err
		}
		out = crmcontracts.ReportingFramework{Definition: in, Revision: current + 1, Version: current + 1, EffectiveAt: s.now()}
		raw, err := encode(in)
		if err != nil {
			return err
		}
		var b bindings
		values := []string{b.add(out.Revision), b.add(raw), b.add(out.EffectiveAt), b.add(author)}
		if _, err := tx.Exec(ctx, "INSERT INTO reporting_framework_revision(revision,definition,effective_at,created_by) VALUES ("+strings.Join(values, ",")+")", b.values...); err != nil {
			return err
		}
		b = bindings{}
		if _, err := tx.Exec(ctx, "UPDATE reporting_framework SET revision="+b.add(out.Revision)+",version="+b.add(out.Version), b.values...); err != nil {
			return err
		}
		return recordChange(ctx, tx, "reporting_framework", frameworkID, "update", &before, out)
	})
	return out, err
}

func validateFrameworkInput(in crmcontracts.ReportingFrameworkInput) error {
	if strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 1000 || len(in.CaptureContexts) > 20 {
		return invalid("supply a reason and at most twenty capture contexts")
	}
	if in.Template != reportingSales && in.Template != "sdr" {
		return invalid("choose sales or SDR defaults")
	}
	return nil
}
