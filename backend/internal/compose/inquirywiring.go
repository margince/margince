// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/compose/inquiries"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/webforms"
	"github.com/margince/margince/backend/internal/platform/database"
)

type inquiryWork = inquiries.Work

func newInquiryRouting(db *database.DB) inquiries.Routing {
	return inquiries.NewRouting(db, newWebFormFence(db), NewProviderFor(db))
}

func newInquiryAssignment(db *database.DB) inquiries.Assignment {
	return inquiries.NewAssignment(db, newContactsStore(db), newInquiryRouting(db))
}

func newInquirySettings(db *database.DB, forms *webforms.Store) inquiries.Settings {
	ex := automation.Executors{Provider: NewProviderFor(db), Authority: identity.NewService(db.Pool())}
	return inquiries.NewSettings(db, forms, newInquiryRouting(db), ex, func() *automation.WorkflowEngine { return NewWorkflowEngine(db) })
}

func newInquiryWork(db *database.DB) *inquiries.Work {
	return inquiries.NewWork(db, newInquiryRouting(db))
}

func newInquiryFiringFence(db *database.DB) *inquiries.FiringFence {
	return inquiries.NewFiringFence(db, newWebFormFence(db))
}
