// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package programsettings_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ps "github.com/margince/margince/backend/internal/compose/programsettings"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// An installation with no auth section signs in by email and password.
func TestPasswordLoginIsOnUnlessTheFileTurnsItOff(t *testing.T) {
	t.Parallel()
	if !config.Val(configtest.MustLoad(t, "version: 1\n", ps.FileSettings()...), ps.PasswordLogin) {
		t.Fatal("a file with no auth section turned password sign-in off")
	}
	// The file cannot check that something else opens a door; cmd/api refuses
	// the combination that leaves no way in.
	off := configtest.MustLoad(t, "version: 1\nauth: { password: { enabled: false } }\n", ps.FileSettings()...)
	if config.Val(off, ps.PasswordLogin) {
		t.Fatal("auth.password.enabled=false read as enabled; the switch would be inert")
	}
}

// A typo must never silently disable authentication.
func TestATypoUnderAuthStopsTheBoot(t *testing.T) {
	t.Parallel()
	_, err := configtest.Load(t, "version: 1\nauth:\n  passwrd:\n    enabled: false\n", nil, nil, ps.FileSettings()...)
	if err == nil || !strings.Contains(err.Error(), "passwrd") {
		t.Fatalf("an unknown key loaded silently: %v", err)
	}
}

// Capture stores content close to special categories, so it is off unless the
// file says so.
func TestAIPayloadCaptureIsOffUnlessTheFileTurnsItOn(t *testing.T) {
	t.Parallel()
	if config.Val(configtest.MustLoad(t, "version: 1\n", ps.FileSettings()...), ps.CapturePayloads) {
		t.Fatal("ai.capture_payloads must default to false")
	}
	if !config.Val(configtest.MustLoad(t, "version: 1\nai:\n  capture_payloads: true\n", ps.FileSettings()...), ps.CapturePayloads) {
		t.Fatal("ai.capture_payloads: true read as off")
	}
}

func TestTheFxCandidatesAreReadAsWrittenAndHeldToISO4217(t *testing.T) {
	t.Parallel()
	cfg := configtest.MustLoad(t, "version: 1\nrates:\n  fx_source: https://example.test/fx\n  fx_currencies: [USD, GBP, CHF]\n", ps.FileSettings()...)
	if got := config.Val(cfg, ps.RatesFxSource); got != "https://example.test/fx" {
		t.Errorf("rates.fx_source = %q", got)
	}
	if got := strings.Join(config.Val(cfg, ps.RatesFxCurrencies), ","); got != "USD,GBP,CHF" {
		t.Errorf("rates.fx_currencies = %q", got)
	}
	// Unset is empty: the worker supplies USD, GBP and CHF, so loading must
	// not invent a value.
	if got := config.Val(configtest.MustLoad(t, "version: 1\n", ps.FileSettings()...), ps.RatesFxCurrencies); len(got) != 0 {
		t.Errorf("rates.fx_currencies defaulted to %v", got)
	}
	for name, list := range map[string]string{"non-iso code": "[USD, US]", "non-letter code": `[USD, "12$"]`, "duplicate": "[USD, usd]"} {
		if _, err := configtest.Load(t, "rates:\n  fx_currencies: "+list+"\n", nil, nil, ps.FileSettings()...); err == nil {
			t.Errorf("%s: a malformed fx_currencies set loaded; a typo must fail at boot", name)
		}
	}
}

// --config-file wins key by key over the base. A list it names replaces the
// base's whole, and a key it leaves out keeps the base's value.
func TestAConfigFileReplacesAListAndKeepsTheBasesOtherKeys(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	base, over := filepath.Join(dir, "margince.yaml"), filepath.Join(dir, "posture.yaml")
	for path, body := range map[string]string{
		base: "version: 1\nrates:\n  fx_source: https://base.test/fx\n  fx_currencies: [USD, GBP, CHF]\n",
		over: "rates:\n  fx_currencies: [SEK]\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	reg, err := config.NewRegistry(ps.FileSettings())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(reg, config.Sources{Args: []string{"--config", base, "--config-file", over}, BootstrapRoots: deployconfig.Roots()})
	if err != nil {
		t.Fatal(err)
	}
	if got := config.Val(cfg, ps.RatesFxCurrencies); len(got) != 1 || got[0] != "SEK" {
		t.Errorf("fx_currencies = %v; a list the config file names replaces the base's entirely", got)
	}
	if got := config.Val(cfg, ps.RatesFxSource); got != "https://base.test/fx" {
		t.Errorf("fx_source = %q; a key the config file leaves out keeps the base's value", got)
	}
}

// An unset gate must never expose /mcp.
func TestTheMCPConnectorIsOffUnlessTheFileTurnsItOn(t *testing.T) {
	t.Parallel()
	if config.Val(configtest.MustLoad(t, "version: 1\nworkspace:\n  name: T\n", ps.FileSettings()...), ps.MCPConnector) {
		t.Fatal("the connector gate must default off")
	}
	if !config.Val(configtest.MustLoad(t, "version: 1\nmcp:\n  connector_enabled: true\n", ps.FileSettings()...), ps.MCPConnector) {
		t.Fatal("mcp.connector_enabled: true read as off")
	}
}

// The retired reporting switch acts on nothing, and a file carrying it still
// loads.
func TestRetiredReportingConfigurationStillLoads(t *testing.T) {
	t.Parallel()
	if _, err := configtest.Load(t, "version: 1\nanalytics:\n  performance_enabled: true\n", nil, nil, ps.FileSettings()...); err != nil {
		t.Fatalf("a file carrying analytics.performance_enabled must still load: %v", err)
	}
}

func TestListsAreOnUnlessAFileSwitchesThemOff(t *testing.T) {
	t.Parallel()
	if !config.Val(configtest.MustLoad(t, "version: 1\n", ps.FileSettings()...), ps.ListsEnabled) {
		t.Fatal("lists.enabled unset switched lists off, want on")
	}
	if config.Val(configtest.MustLoad(t, "version: 1\nlists:\n  enabled: false\n", ps.FileSettings()...), ps.ListsEnabled) {
		t.Fatal("lists.enabled: false must switch lists off")
	}
}

// A capability that erases tenant data or fakes a real send is stated, never
// assumed.
func TestTheOperationsSwitchesAreOffUnlessTheFileArmsThem(t *testing.T) {
	t.Parallel()
	off := configtest.MustLoad(t, "version: 1\nworkspace:\n  name: T\n", ps.FileSettings()...)
	if config.Val(off, ps.AllowDataReset) || config.Val(off, ps.AllowTestMailbox) {
		t.Fatal("an operations switch defaulted on")
	}
	on := configtest.MustLoad(t, "version: 1\noperations:\n  allow_data_reset: true\n  allow_test_mailbox: true\n", ps.FileSettings()...)
	if !config.Val(on, ps.AllowDataReset) || !config.Val(on, ps.AllowTestMailbox) {
		t.Fatal("operations switches set true read as off")
	}
}
