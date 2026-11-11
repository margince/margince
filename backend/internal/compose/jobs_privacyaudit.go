// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"time"

	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/riverqueue/river"
)

type privacyRetentionCursor struct {
	PoliciesDone bool                          `json:"privacy_policies_done"`
	Audit        privacy.HistoricalAuditCursor `json:"privacy_audit_cursor"`
}

func (w *privacyRetentionWorker) runPass(ctx, passCtx context.Context, job *river.Job[PrivacyRetentionArgs]) error {
	var cursor privacyRetentionCursor
	if len(job.Metadata) > 0 {
		if err := json.Unmarshal(job.Metadata, &cursor); err != nil {
			return jobs.FaultContext(ctx, err)
		}
	}
	service := w.retention(InstallationDB(w.pool))
	if !cursor.PoliciesDone {
		if err := service.EvaluateInstallation(passCtx); err != nil {
			return jobs.FaultContext(ctx, err)
		}
		if err := river.MetadataSet(ctx, "privacy_policies_done", true); err != nil {
			return jobs.FaultContext(ctx, err)
		}
		return river.JobSnooze(time.Second)
	}
	next, done, err := service.RetryHistoricalAuditPage(passCtx, cursor.Audit)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if done {
		return nil
	}
	// A crash before metadata is saved replays the same idempotent page.
	if err := river.MetadataSet(ctx, "privacy_audit_cursor", next); err != nil {
		return jobs.FaultContext(ctx, err)
	}
	return river.JobSnooze(time.Second)
}
