// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/companyscan"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/promptlang"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestScanFailuresKeepTheirWrittenLanguage(t *testing.T) {
	for _, tc := range []struct{ language, worker, model string }{
		{"en", "No worker", "The model's answer"},
		{"de", "Es ist kein Worker", "Die Antwort des Modells"},
		{"vi", "Không có tiến trình", "Câu trả lời của mô hình"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			e := integration.Setup(t)
			at := clocktest.Now(t)
			now := func() time.Time { return at }
			integration.SetBaseLanguage(e.Admin(), t, e.Pool, tc.language)
			company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Original account", &e.Rep1))
			view := company360Service(e)
			noWorker := companyscan.NewService(e.Pool, view, view, nil, nil, nil, now, nil)
			got, err := noWorker.Ensure(e.Admin(), company, false)
			if err != nil {
				t.Fatal(err)
			}
			if got.State != crmcontracts.CompanyScanStateDegraded || got.DegradeReason == nil || !strings.HasPrefix(*got.DegradeReason, tc.worker) {
				t.Fatalf("no-worker result: %+v", got)
			}
			seedInboundAsk(t, e, company.UUID, "workspace")
			lane := &failingLane{}
			var queued []companyscan.Queued
			svc := scanClocked(e, lane, &queued, now)
			rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)
			got = readOnce(rep, t, svc, company, &queued)
			if got.State != crmcontracts.CompanyScanStateDegraded || got.DegradeReason == nil || !strings.HasPrefix(*got.DegradeReason, tc.model) || lane.calls == 0 {
				t.Fatalf("unusable-model result: %+v", got)
			}
			for _, request := range lane.requests {
				if !strings.Contains(request.System, promptlang.Rule(tc.language)) {
					t.Fatal("model did not receive base language")
				}
			}
			original := *got.DegradeReason
			nextLanguage := "en"
			if tc.language == "en" {
				nextLanguage = "de"
			}
			integration.SetBaseLanguage(e.Admin(), t, e.Pool, nextLanguage)
			replay, err := svc.Get(rep, company)
			if err != nil {
				t.Fatal(err)
			}
			if replay.Stale == nil || !*replay.Stale || replay.DegradeReason == nil || *replay.DegradeReason != original {
				t.Fatalf("read rewrote stored explanation: %+v", replay)
			}
		})
	}
}
