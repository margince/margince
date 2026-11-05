// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

// Each limit admits its whole range and nothing past either end, and the
// default an untouched installation reads is inside it. A default outside its
// own range would be a value no admin could ever save back.
func TestEachWebsiteReadingLimitAdmitsItsRangeAndNothingPastIt(t *testing.T) {
	for _, tc := range []struct {
		entry                *config.Setting[int]
		lowest, def, highest int
	}{
		{AutoEnrichDailyCap, 1, DefaultAutoEnrichDailyCap, MaxAutoEnrichDailyCap},
		{SiteReadMaxPages, 1, DefaultSiteReadMaxPages, MaxSiteReadMaxPages},
		{SiteReadMaxMiB, 1, DefaultSiteReadMaxMiB, MaxSiteReadMaxMiB},
		{SiteReadWallSeconds, MinSiteReadWallSeconds, DefaultSiteReadWallSeconds, MaxSiteReadWallSeconds},
		{ConfidentialityPassCap, 1, DefaultConfidentialityPassCap, MaxConfidentialityPassCap},
	} {
		t.Run(tc.entry.Key(), func(t *testing.T) {
			for _, ok := range []int{tc.lowest, tc.def, tc.highest} {
				if err := tc.entry.ValidateJSON(json.RawMessage(strconv.Itoa(ok))); err != nil {
					t.Errorf("%d refused: %v", ok, err)
				}
			}
			for _, bad := range []int{tc.lowest - 1, tc.highest + 1} {
				if err := tc.entry.ValidateJSON(json.RawMessage(strconv.Itoa(bad))); err == nil {
					t.Errorf("%d admitted, want a refusal", bad)
				}
			}
			declared, ok := tc.entry.DefaultText()
			if !ok || declared != strconv.Itoa(tc.def) {
				t.Errorf("the entry declares default %q (set: %v), want %d", declared, ok, tc.def)
			}
		})
	}
}
