// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/privacyreview"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// The migrations the audit redaction tasks need applied before they run.
const (
	auditRedactionMigration = "core/1794074311"
	retainedEventsMigration = "core/1794074312"
	legacyErasureMigration  = "core/1794074313"
)

func auditRedactionDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1794066141_erased_audit_payloads", Version: 2,
		Title:    systemtext.Literal("datatask.title.core.1794066141_erased_audit_payloads"),
		Requires: []string{auditRedactionMigration, retainedEventsMigration}, Steps: privacyreview.AuditSteps(privacy.RedactHistoricalAuditPageTx),
	}
}

func linkedAuditRedactionDataTask(pool *pgxpool.Pool) datatask.Task {
	steps := privacyreview.AuditSteps(privacy.RedactLinkedHistoricalAuditPageTx)
	identities := privacyreview.AuditSteps(privacy.RedactLinkedHistoricalIdentitiesPageTx)[0]
	identities.Name = "request_identity_images"
	steps = append(steps, identities, privacyreview.UnlinkedApprovalReviewStep(notices.NewStore(InstallationDB(pool))))
	return datatask.Task{ID: "core/1794076293_linked_erased_audit_payloads", Version: 1, Title: systemtext.Literal("datatask.title.core.1794076293_linked_erased_audit_payloads"), Requires: []string{auditRedactionMigration, retainedEventsMigration, legacyErasureMigration}, Steps: steps}
}

func markedAuditRedactionDataTask() datatask.Task {
	weekly := privacyreview.AuditSteps(privacy.RedactMarkedWeeklyAuditPageTx)[0]
	weekly.Name = "marked_weekly_images"
	weekly.Limit = privacy.MarkerAuditPageSize
	requests := privacyreview.AuditSteps(privacy.RedactRetiredRequestAuditPageTx)[0]
	requests.Name = "retired_request_images"
	requests.Limit = privacy.MarkerAuditPageSize
	return datatask.Task{ID: "core/1794076293_marked_erased_audit_payloads", Version: 1, Title: systemtext.Literal("datatask.title.core.1794076293_marked_erased_audit_payloads"), Requires: []string{auditRedactionMigration, retainedEventsMigration, legacyErasureMigration}, Steps: []datatask.Step{weekly, requests}}
}
