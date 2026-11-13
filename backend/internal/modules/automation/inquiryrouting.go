// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// InquiryRouting keeps lead writes and recipient access checks with their owners.
type InquiryRouting interface {
	TaskSubmissions(context.Context, []ids.ActivityID) (map[ids.UUID]string, error)
	ExistingTask(context.Context, context.Context, string) (crmcontracts.Activity, bool, error)
	Within(context.Context, ids.UUID, func(context.Context) error) error
	Validate(context.Context, crmcontracts.WebsiteInquiryPolicy) error
	Choose(context.Context, context.Context, crmcontracts.WebsiteInquiryPolicy, datasource.EntityRef, ids.UUID) (ids.UUID, string, error)
	Preview(context.Context, context.Context, crmcontracts.WebsiteInquiryPolicy, datasource.EntityRef, ids.UUID) (ids.UUID, string, error)
	CheckRecipient(context.Context, ids.UUID, datasource.EntityRef, ids.UUID) error
	ReuseArrivalTask(context.Context, context.Context, string, ids.UUID, ids.UUID, time.Time) (bool, error)
}

// WithInquiryRouting lets inquiry policies be checked and changed under their
// form's fence.
func (s *AutomationStore) WithInquiryRouting(routing InquiryRouting) *AutomationStore {
	s.inquiries = routing
	return s
}

// WithInquiryRouting hands routing to the handlers' automation store.
func (h Handlers) WithInquiryRouting(routing InquiryRouting) Handlers {
	h.automations = h.automations.WithInquiryRouting(routing)
	return h
}
