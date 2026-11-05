// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

func countryIs(code jurisdiction.Code) CountryReader {
	return func(context.Context, pgx.Tx) (jurisdiction.Code, error) { return code, nil }
}

func publicNoticeCtx(e *sarIdentifierEnv) context.Context {
	return principal.WithActor(e.ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:public_confirm"})
}

// seedNoticeHistory gives the export's subject an address, two consent
// answers, two communication bases, one mail, and four retention rules.
func seedNoticeHistory(t *testing.T, e *sarIdentifierEnv) {
	t.Helper()
	ctx := context.Background()
	tag := e.contact.String()[28:]
	activity := ids.NewV7()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE contact SET address_city = 'Berlin' WHERE id = $1`, []any{e.contact}},
		{`INSERT INTO consent_purpose (key, label, class) VALUES ($1, 'News', 'marketing'), ($2, 'Old', 'marketing')`,
			[]any{"pn_news_" + tag, "pn_old_" + tag}},
		{`INSERT INTO contact_consent (contact_id, purpose_id, state)
		  SELECT $1, id, CASE WHEN key LIKE 'pn_news_%' THEN 'granted' ELSE 'withdrawn' END
		    FROM consent_purpose WHERE key IN ($2, $3)`, []any{e.contact, "pn_news_" + tag, "pn_old_" + tag}},
		{`INSERT INTO communication_basis (contact_id, kind, captured_by) VALUES ($1, 'subject_initiated_correspondence', 'test')`, []any{e.contact}},
		{`INSERT INTO communication_basis (contact_id, kind, captured_by, revoked_at) VALUES ($1, 'legitimate_interests', 'test', now())`, []any{e.contact}},
		{`INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by, retention_class, retention_class_at)
		  VALUES ($1, 'email', 'Order', 'about the order', now(), 'manual', 'test', 'commercial_correspondence', now())`, []any{activity}},
		{`INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, []any{activity, e.contact}},
		{`DELETE FROM retention_policy`, nil},
		{`INSERT INTO retention_policy (object_type, category, retain_days, action) VALUES
		    ('activity', NULL, 90, 'erase'), ('activity', 'transcript', 30, 'erase'),
		    ('raw_capture', NULL, 90, 'erase'), ('deal', 'lost', 365, 'archive')`, nil},
	} {
		if _, err := e.owner.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding %q: %v", statement.sql, err)
		}
	}
}

func TestANoticeDisclosesWhatTheExportHoldsAndTheUsesOnRecord(t *testing.T) {
	e := setupSARIdentifiers(t)
	seedNoticeHistory(t, e)

	got, err := AssembleNoticeDisclosure(publicNoticeCtx(e), e.db, countryIs("zu"), NoticeSubject{ContactID: e.contact})
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	for _, kind := range []string{"name", "email", "phone", "postal_address", "social_profile", "correspondence", "preferences"} {
		if !slices.Contains(got.Categories, kind) {
			t.Errorf("categories %v leave out %q, which the export holds", got.Categories, kind)
		}
	}
	tag := e.contact.String()[28:]
	want := []NoticeUse{{"pn_news_" + tag, "consent"}, {businessCorrespondence, "subject_initiated_correspondence"}}
	if !slices.Equal(got.Uses, want) {
		t.Errorf("uses %v, want only the granted purpose and the live basis: %v", got.Uses, want)
	}
	if len(got.Retention) != 1 || got.Retention[0].Scope != "activity" {
		t.Fatalf("retention %+v, want only the mail rule: no transcript, original or deal is held", got.Retention)
	}
	floor := got.Retention[0].KeptByLaw
	if floor == nil || floor.Keep != (jurisdiction.Period{Years: 6}) || floor.Anchor != jurisdiction.AnchorCalendarYearEnd {
		t.Errorf("the 90-day erase names floor %+v, want the six years the floor keeps correspondence", floor)
	}
}

func TestARuleIsShownBareWhereNoFloorBinds(t *testing.T) {
	e := setupSARIdentifiers(t)
	seedNoticeHistory(t, e)

	got, err := AssembleNoticeDisclosure(publicNoticeCtx(e), e.db, countryIs("zv"), NoticeSubject{ContactID: e.contact})
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if len(got.Retention) != 1 || got.Retention[0].KeptByLaw != nil {
		t.Errorf("retention %+v, want the mail rule with no floor under a pack that declares none", got.Retention)
	}
}

func TestOnlyThePublicEdgeAssemblesANotice(t *testing.T) {
	e := setupSARIdentifiers(t)
	if _, err := AssembleNoticeDisclosure(e.ctx, e.db, countryIs("zv"), NoticeSubject{ContactID: e.contact}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a human seat assembled a notice disclosure: %v", err)
	}
}
