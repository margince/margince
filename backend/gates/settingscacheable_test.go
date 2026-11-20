// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"flag"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var updateCacheableSettings = flag.Bool("update-cacheable-settings", false,
	"rewrite gates/testdata/cacheable_settings.txt from the settings catalog")

const cacheableSettingsFile = "gates/testdata/cacheable_settings.txt"

// controlWords name the settings a reader must see change at once. A cadence,
// a format or a threshold never has one of these in its key.
var controlWords = regexp.MustCompile(`mfa|sso|oidc|oauth|secret|token|credential|provider_key|consent|retain|retention|privacy|private|sharing|agent|^ai\.|approval|auto_apply|autopilot|overwrite|merge|policy`)

// A cached setting can be CacheLifetime late in this process or another, so a
// setting is cached only when it declares it. The cached set is read off the
// catalog into a file, so each addition shows in a diff. None may carry the
// name of a control.
func TestTheCachedSettingsAreTheDeclaredLowRiskOnes(t *testing.T) {
	t.Parallel()
	var cached []string
	for _, spec := range compose.SettingsCatalogForTest() {
		if !spec.Cacheable {
			continue
		}
		if controlWords.MatchString(spec.Key) {
			t.Errorf("%s declares itself cacheable but its name marks a control; a change to it must apply at once", spec.Key)
		}
		cached = append(cached, spec.Key)
	}
	slices.Sort(cached)
	if len(cached) == 0 {
		t.Fatal("no setting is cacheable; the catalog read found nothing")
	}
	want := strings.Join(cached, "\n") + "\n"
	if *updateCacheableSettings {
		if err := os.WriteFile(cacheableSettingsFile, []byte(want), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := gatekit.ReadFile(cacheableSettingsFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("%s is out of date with the settings that declare Cacheable. Review the change, then run:\n  go test ./gates -run CachedSettings -update-cacheable-settings", cacheableSettingsFile)
	}
}
