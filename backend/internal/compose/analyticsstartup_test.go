// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnalyticsStartupOmitsDataWithoutBothReportGrants(t *testing.T) {
	for _, granted := range []string{"", "report_definition", "reporting_framework"} {
		t.Run(granted, func(t *testing.T) {
			actor := principal.Principal{Type: principal.PrincipalHuman, Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{granted: {Read: true}}}}
			ctx := principal.WithActor(t.Context(), actor)
			// No readers are wired: an ungranted caller must not reach any of them.
			got, err := (&Server{}).analyticsStartup(ctx)
			if err != nil || got != nil {
				t.Fatalf("startup = %+v, %v; want omitted", got, err)
			}
		})
	}
}
