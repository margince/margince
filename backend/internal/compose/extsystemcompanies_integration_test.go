// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The company and mail-content reads over real Postgres, from a subscription delivery.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

func (s *systemReadEnv) company(t *testing.T, name, visibility string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	s.WsExec(t, `INSERT INTO company (id, display_name, source, captured_by, visibility, owner_id, source_system, source_id)
		VALUES ($1, $2, 'system', 'system', $3, $4, 'erp', $5)`, id, name, visibility, s.Rep1, "c-"+name)
	return id
}

func TestTheCompanyReadHoldsOnlyWhatTheWholeWorkspaceMaySee(t *testing.T) {
	s := setupSystemRead(t)
	composeReader(t, extension.SystemReadCompanies)
	shown := s.company(t, "Shown GmbH", "workspace")
	hidden := s.company(t, "Hidden GmbH", "owner")
	var got extension.CompanySummary
	var page extension.Page[extension.CompanySummary]
	var hiddenErr error
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		var err error
		if got, err = rt.Companies().Get(ctx, shown.String()); err != nil {
			return err
		}
		_, hiddenErr = rt.Companies().Get(ctx, hidden.String())
		page, err = rt.Companies().List(ctx, extension.CompanyPageQuery{})
		return err
	})
	if got.Name != "Shown GmbH" || got.SourceSystem != "erp" || got.SourceID != "c-Shown GmbH" {
		t.Errorf("Get = %+v, want the company with its source", got)
	}
	if !errors.Is(hiddenErr, extension.ErrNotFound) {
		t.Errorf("an owner-only company: err = %v, want ErrNotFound", hiddenErr)
	}
	for _, c := range page.Items {
		if c.ID == hidden.String() {
			t.Errorf("List holds the private company %s", c.ID)
		}
	}
}

func TestTheMailContentReadGivesTheWordsOfWorkspaceMailOnly(t *testing.T) {
	s := setupSystemRead(t)
	composeReader(t, extension.SystemReadMailContent)
	open := ids.NewV7()
	private := ids.NewV7()
	redacted := ids.NewV7()
	for _, row := range []struct {
		id       ids.UUID
		audience string
		redact   string
	}{{open, "workspace", "{}"}, {private, "participants", "{}"}, {redacted, "workspace", "{body}"}} {
		s.WsExec(t, `INSERT INTO activity (id, kind, subject, body, source, captured_by, audience, audience_reason, redacted_fields, capture_label, capture_labeled_at, bulk_mail_attested, source_id)
			VALUES ($1, 'email', 'Quote', 'Hello', 'system', 'system', $2, CASE WHEN $2 = 'workspace' THEN NULL ELSE 'manual' END, $3::text[], 'commitment', now(), true, 'm-1')`,
			row.id, row.audience, row.redact)
	}
	var got, withheld extension.MailContent
	var privateErr error
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		var err error
		if got, err = rt.MailContent().Get(ctx, open.String()); err != nil {
			return err
		}
		_, privateErr = rt.MailContent().Get(ctx, private.String())
		withheld, err = rt.MailContent().Get(ctx, redacted.String())
		return err
	})
	if got.Subject != "Quote" || got.Body != "Hello" || got.CaptureLabel != "commitment" || !got.BulkMail || got.SourceID != "m-1" || got.ContentState != extension.MailContentAvailable {
		t.Errorf("open mail = %+v, want its words, label, bulk flag and source id", got)
	}
	if !errors.Is(privateErr, extension.ErrNotFound) {
		t.Errorf("a participants-only mail: err = %v, want ErrNotFound", privateErr)
	}
	if withheld.ContentState != extension.MailContentWithheld || withheld.Subject != "" || withheld.Body != "" {
		t.Errorf("redacted mail = %+v, want withheld with no words", withheld)
	}
}
