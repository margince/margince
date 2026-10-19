// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCloseDateReceiptRetainsItsWrittenLanguageAndCorrection(t *testing.T) {
	for _, tc := range []struct{ language, phrase string }{{"en", "could not read its correspondence"}, {"de", "Korrespondenz konnte nicht gelesen"}, {"vi", "không đọc được trao đổi"}} {
		t.Run(tc.language, func(t *testing.T) {
			env := setupCloseDate(t)
			settingStore := settings.New(env.Pool, settings.NewRegistry(identity.BaseLanguage))
			if err := settings.Set(env.Admin(), settingStore, identity.BaseLanguage, tc.language); err != nil {
				t.Fatal(err)
			}
			id := env.seedSweepDeal(t, "Quiet deal", env.late, new("commit"), new(30), 90)
			if err := env.sweep(); err != nil {
				t.Fatal(err)
			}
			if err := settings.Set(env.Admin(), settingStore, identity.BaseLanguage, "en"); err != nil {
				t.Fatal(err)
			}
			store := deals.NewStore(env.DB(), DealsInstallation())
			receipts, err := store.RecentCorrectionsOwnedBy(env.As(env.Rep1, []ids.UUID{env.Team1}, integration.AdminPerms), time.Time{}, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(receipts) != 1 || receipts[0].DealID.UUID != id || !strings.Contains(receipts[0].Basis, tc.phrase) {
				t.Fatalf("receipt lost its deal or language: %+v", receipts)
			}
			corrected := env.readSwept(t, id)
			if corrected.forecastCat == nil || *corrected.forecastCat != "best_case" || env.pendingCorrections(t, id) != 0 {
				t.Fatalf("language changed correction behavior: %+v", corrected)
			}
		})
	}
}
