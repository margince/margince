// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

import (
	"reflect"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAccountMomentLanguagePreservesDismissalAndEvidence(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Example Company", &e.Rep1))
	logCompanyTask(t, e, company.UUID, "Original promise", company360Clock.Add(24*time.Hour))
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)
	baseline, err := svc.Assemble(reader, company)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Moment == nil {
		t.Fatal("missing baseline moment")
	}
	fingerprint := baseline.Moment.EvidenceFingerprint
	if err := svc.DismissSuggestion(reader, company, fingerprint, nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ language, headline, quiet string }{
		{"de", "Sie schulden ihnen: Original promise", "Keine offenen Zusagen gegenüber diesem Unternehmen"},
		{"vi", "Bạn cần thực hiện cho họ: Original promise", "Không có cam kết nào chưa thực hiện với công ty này"},
	} {
		integration.SetBaseLanguage(e.Admin(), t, e.Pool, tc.language)
		hidden, err := svc.Assemble(reader, company)
		if err != nil {
			t.Fatal(err)
		}
		if hidden.Moment == nil || hidden.Moment.Headline != tc.quiet {
			t.Fatalf("%s reopened dismissed promise: %+v", tc.language, hidden.Moment)
		}
		if err := svc.RestoreSuggestion(reader, company, fingerprint); err != nil {
			t.Fatal(err)
		}
		restored, err := svc.Assemble(reader, company)
		if err != nil {
			t.Fatal(err)
		}
		if restored.Moment == nil || restored.Moment.Headline != tc.headline {
			t.Fatalf("%s moment: %+v", tc.language, restored.Moment)
		}
		if restored.Moment.EvidenceFingerprint != fingerprint || !reflect.DeepEqual(restored.Moment.Evidence, baseline.Moment.Evidence) {
			t.Fatal("language changed promise evidence")
		}
		if err := svc.DismissSuggestion(reader, company, fingerprint, nil); err != nil {
			t.Fatal(err)
		}
	}
}
