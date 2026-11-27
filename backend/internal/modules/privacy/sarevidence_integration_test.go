// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The notice a lead twin of the subject was sent travels with the export. The
// writer lives in the consent module, which this package cannot import, so the
// row is seeded the way the notice lane writes it.
func TestTheAccessExportCarriesTheNoticeALeadTwinWasSent(t *testing.T) {
	e := setupSARIdentifiers(t)
	lead := ids.NewV7()
	if _, err := e.owner.Exec(e.ctx,
		`INSERT INTO lead (id, full_name, email, source, captured_by) VALUES ($1, 'Sara Subject', $2, 'manual', 'human:test')`,
		lead, ident(e.contact, liveEmail)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(e.ctx,
		`INSERT INTO lead_privacy_notice (lead_id, template_key, template_version, locale, state, queued_at)
		 VALUES ($1, 'privacy_notice', 3, 'en', 'queued', now())`, lead); err != nil {
		t.Fatal(err)
	}

	pkg, err := AssembleSAR(e.ctx, e.db, e.contact)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}
	if len(pkg.LeadPrivacyNotices) != 1 || pkg.LeadPrivacyNotices[0]["template_key"] != "privacy_notice" {
		t.Fatalf("lead notices = %v, want the one privacy_notice notice", pkg.LeadPrivacyNotices)
	}
}
