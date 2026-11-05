// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/config"
)

// Every source Load records is one the contract can name. The base file and
// the --config-file file are one API source, told apart by `file`.
func TestEverySourceHasAContractName(t *testing.T) {
	sources := []config.Source{
		config.SourceFlag, config.SourceEnv, config.SourceEnvFile, config.SourceConfigFile,
		config.SourceBaseFile, config.SourceDatabase, config.SourceDefault, config.SourceUnset,
	}
	for _, s := range sources {
		if got := settingOrigin(config.Origin{Source: s}); !got.Source.Valid() {
			t.Errorf("source %q answers %q, which the contract does not list", s, got.Source)
		}
	}
	base := settingOrigin(config.Origin{Source: config.SourceBaseFile, Name: "capture.auto_enrich", File: "/etc/margince/base.yaml"})
	if base.Source != crmcontracts.SettingOriginSourceConfigFile || base.File == nil || *base.File != "/etc/margince/base.yaml" {
		t.Fatalf("the base file answers %+v, want config_file naming the file", base)
	}
}

// Locks names only what a boot source sets, and says "nothing" as an empty
// list rather than null.
func TestLocksNameOnlyTheBootSetSettings(t *testing.T) {
	if got := Locks(unsetConfig(), StoredSettings()); got == nil || len(got) != 0 {
		t.Fatalf("an unbound configuration locks %v, want []", got)
	}
}
