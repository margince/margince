// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The headline is the reader's language, not the installation's, and the
// language never reaches the fingerprint a dismissal is keyed on.
func TestAContactMomentIsWrittenInTheReadersLanguage(t *testing.T) {
	e := integration.Setup(t)
	contact := seedLinkedContact(t, e, "sprache@kunde.example")
	logTaskFor(t, e, contact, "Send the signed contract", new(clocktest.Now(t).Add(24*time.Hour)))
	svc := contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), func() time.Time { return clocktest.Now(t) }, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), func() time.Time { return clocktest.Now(t) })
	contactID := ids.From[ids.ContactKind](contact)

	english, err := svc.Assemble(e.Admin(), contactID)
	if err != nil || english.Moment == nil {
		t.Fatalf("assembling the contact in the base language: %v, moment %v", err, english.Moment)
	}
	admin, _ := principal.Actor(e.Admin())
	admin.Locale = "vi"
	vietnamese, err := svc.Assemble(principal.WithActor(e.Admin(), admin), contactID)
	if err != nil || vietnamese.Moment == nil {
		t.Fatalf("assembling the contact for a Vietnamese reader: %v", err)
	}
	if want := "Bạn cần thực hiện cho họ: Send the signed contract"; vietnamese.Moment.Headline != want {
		t.Errorf("headline = %q, want %q", vietnamese.Moment.Headline, want)
	}
	if english.Moment.EvidenceFingerprint != vietnamese.Moment.EvidenceFingerprint {
		t.Errorf("the fingerprint changed with the language: %q vs %q",
			english.Moment.EvidenceFingerprint, vietnamese.Moment.EvidenceFingerprint)
	}
}
